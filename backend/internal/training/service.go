package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
	"lexis/internal/training/store"
	"lexis/internal/words"
)

// SettingNewWordsLimit — ключ в settings (JSON-число). UI для него появится в 4.7.
const SettingNewWordsLimit = "new_words_limit"

// Service — сессии тренировки. Состояние сессии живёт только в БД (training_sessions + reviews),
// в памяти ничего нет: рестарт контейнера посреди сессии ничего не ломает.
// Саму очередь держит клиент; сервер отдаёт её на старте и подсказывает requeue.
type Service struct {
	pool    *pgxpool.Pool
	clock   clock.Clock
	bus     eventbus.Publisher
	newRand func() *rand.Rand // на каждый вызов свой генератор: *rand.Rand не потокобезопасен
}

func NewService(pool *pgxpool.Pool, clk clock.Clock, bus eventbus.Publisher) *Service {
	return &Service{pool: pool, clock: clk, bus: bus, newRand: func() *rand.Rand {
		return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}}
}

type StartParams struct {
	GroupID  int64    `json:"groupId"`
	Strategy Strategy `json:"strategy"`
	Free     bool     `json:"free"`
}

type StartResult struct {
	SessionID *int64 `json:"sessionId"` // nil — карточек нет, сессия не создана
	Cards     []Card `json:"cards"`
}

// Start собирает очередь и создаёт сессию. Если карточек нет — сессию не создаём:
// группа без due-карточек и так считается «повторённой» (§5), а пустая сессия
// только засорила бы статистику.
func (s *Service) Start(ctx context.Context, p StartParams) (StartResult, error) {
	if p.GroupID <= 0 {
		return StartResult{}, httpx.BadRequest("invalid_group", "groupId must be a positive id")
	}
	if !p.Strategy.Valid() {
		return StartResult{}, httpx.BadRequest("invalid_strategy", "strategy must be en_ru, ru_en or context")
	}

	q := store.New(s.pool)
	exists, err := q.GroupExists(ctx, p.GroupID)
	if err != nil {
		return StartResult{}, fmt.Errorf("training start: %w", err)
	}
	if !exists {
		return StartResult{}, httpx.NotFound("group not found")
	}

	now := s.clock.Now()
	ws, err := s.sessionWords(ctx, q, p, now)
	if err != nil {
		return StartResult{}, fmt.Errorf("training start: %w", err)
	}
	cards := Build(ws, BuildOptions{
		Strategy: p.Strategy,
		Now:      now,
		NewLimit: newWordsLimit(ctx, q),
		Free:     p.Free,
		Rand:     s.newRand(),
	})
	if len(cards) == 0 {
		return StartResult{Cards: cards}, nil
	}

	id, err := q.InsertSession(ctx, store.InsertSessionParams{
		GroupID: &p.GroupID, Strategy: string(p.Strategy), Free: p.Free, Now: now,
	})
	if err != nil {
		return StartResult{}, fmt.Errorf("training start: insert session: %w", err)
	}
	return StartResult{SessionID: &id, Cards: cards}, nil
}

func (s *Service) sessionWords(ctx context.Context, q *store.Queries, p StartParams, now time.Time) ([]QueueWord, error) {
	_, dayEnd := clock.DayBounds(now)
	rows, err := q.ListSessionWords(ctx, store.ListSessionWordsParams{GroupID: p.GroupID, Free: p.Free, DayEnd: dayEnd})
	if err != nil {
		return nil, fmt.Errorf("list session words: %w", err)
	}
	ws := make([]QueueWord, len(rows))
	byID := make(map[int64]*QueueWord, len(rows))
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ws[i] = QueueWord{ID: r.ID, Term: r.Term, Forms: r.Forms, Translation: r.Translation, Level: r.Level, NextReviewAt: r.NextReviewAt}
		byID[r.ID] = &ws[i]
		ids[i] = r.ID
	}
	// Примеры нужны только стратегии context — для остальных лишний запрос не делаем.
	if p.Strategy != StrategyContext || len(rows) == 0 {
		return ws, nil
	}
	exs, err := q.ListExamplesByWordIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list examples: %w", err)
	}
	for _, e := range exs {
		ex, code := words.ParseExample(e.Sentence)
		if code != "" { // в БД только валидная разметка; если нет — показываем как есть
			ex = words.Example{Text: e.Sentence, Sentence: e.Sentence}
		}
		w := byID[e.WordID]
		w.Examples = append(w.Examples, ex)
	}
	return ws, nil
}

// newWordsLimit читает лимит из settings; нет ключа или мусор — DefaultNewLimit.
// Ошибка чтения не роняет старт сессии: лимит — не повод отказать в тренировке.
func newWordsLimit(ctx context.Context, q *store.Queries) int {
	raw, err := q.GetSetting(ctx, SettingNewWordsLimit)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultNewLimit
	}
	var n int
	if err == nil {
		err = json.Unmarshal(raw, &n)
	}
	if err != nil || n < 0 {
		slog.WarnContext(ctx, "training: bad new_words_limit setting, using default", "raw", string(raw), "err", err)
		return DefaultNewLimit
	}
	return n
}

type AnswerParams struct {
	WordID int64  `json:"wordId"`
	Answer Answer `json:"answer"`
}

type AnswerResult struct {
	LevelBefore    int16 `json:"levelBefore"`
	LevelAfter     int16 `json:"levelAfter"`
	FirstInSession bool  `json:"firstInSession"`
	// RequeueAt — только для «Не знаю»: вернуть карточку через столько позиций (3–5).
	RequeueAt *int `json:"requeueAt,omitempty"`
}

// Answer записывает ответ. Уровень меняет только первый ответ на слово в обычной сессии (§6);
// повторы и free-сессии пишутся в reviews с level_before = level_after.
func (s *Service) Answer(ctx context.Context, sessionID int64, p AnswerParams) (AnswerResult, error) {
	if !p.Answer.Valid() {
		return AnswerResult{}, httpx.BadRequest("invalid_answer", "answer must be good, hard or again")
	}
	if p.WordID <= 0 {
		return AnswerResult{}, httpx.BadRequest("invalid_word", "wordId must be a positive id")
	}

	now := s.clock.Now()
	var res AnswerResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// Порядок блокировок везде одинаковый — сессия, потом слово, — поэтому без дедлоков.
		sess, err := openSession(ctx, q, sessionID)
		if err != nil {
			return err
		}
		if sess.GroupID != nil { // группу могли удалить посреди сессии — тогда не проверяем
			in, err := q.WordInGroup(ctx, store.WordInGroupParams{WordID: p.WordID, GroupID: *sess.GroupID})
			if err != nil {
				return fmt.Errorf("word in group: %w", err)
			}
			if !in {
				return httpx.BadRequest("word_not_in_session", "word is not in the session group")
			}
		}

		w, err := q.GetWordSRSForUpdate(ctx, p.WordID)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("word not found")
		}
		if err != nil {
			return fmt.Errorf("get word: %w", err)
		}
		answered, err := q.HasReview(ctx, store.HasReviewParams{SessionID: sessionID, WordID: p.WordID})
		if err != nil {
			return fmt.Errorf("has review: %w", err)
		}

		res = AnswerResult{LevelBefore: w.Level, LevelAfter: w.Level, FirstInSession: !answered}
		if res.FirstInSession && !sess.Free {
			st := Apply(State{
				Level: w.Level, Streak: w.Streak, NextReviewAt: w.NextReviewAt,
				LastReviewedAt: w.LastReviewedAt, LastGoodAt: w.LastGoodAt,
			}, p.Answer, now)
			if err := q.UpdateWordSRS(ctx, store.UpdateWordSRSParams{
				ID: w.ID, Level: st.Level, Streak: st.Streak, NextReviewAt: st.NextReviewAt,
				LastReviewedAt: st.LastReviewedAt, LastGoodAt: st.LastGoodAt,
			}); err != nil {
				return fmt.Errorf("update word srs: %w", err)
			}
			res.LevelAfter = st.Level
		}

		if err := q.InsertReview(ctx, store.InsertReviewParams{
			SessionID: sessionID, WordID: p.WordID, Answer: string(p.Answer),
			FirstInSession: res.FirstInSession, LevelBefore: res.LevelBefore, LevelAfter: res.LevelAfter, Now: now,
		}); err != nil {
			return fmt.Errorf("insert review: %w", err)
		}
		return nil
	})
	if err != nil {
		return AnswerResult{}, fmt.Errorf("training answer: %w", err)
	}

	if res.LevelAfter != res.LevelBefore {
		s.bus.Publish(eventbus.WordLevelChanged{WordID: p.WordID, FromLevel: res.LevelBefore, ToLevel: res.LevelAfter, At: now})
	}
	// «Не знаю» возвращается в очередь при каждом ответе, не только первом — пока не будет Знаю/Затрудняюсь.
	if p.Answer == AnswerAgain {
		n := RequeueOffset(s.newRand())
		res.RequeueAt = &n
	}
	return res, nil
}

type LevelChange struct {
	WordID      int64  `json:"wordId"`
	Term        string `json:"term"`
	LevelBefore int16  `json:"levelBefore"`
	LevelAfter  int16  `json:"levelAfter"`
}

type FinishResult struct {
	SessionID    int64         `json:"sessionId"`
	Free         bool          `json:"free"`
	Total        int           `json:"total"` // слов с ответом (по первым ответам)
	Good         int           `json:"good"`
	Hard         int           `json:"hard"`
	Again        int           `json:"again"`
	DurationSec  int           `json:"durationSec"`
	LevelChanges []LevelChange `json:"levelChanges"`
}

// Finish закрывает сессию и считает итоги по первым ответам.
func (s *Service) Finish(ctx context.Context, sessionID int64) (FinishResult, error) {
	now := s.clock.Now()
	var res FinishResult
	var groupID int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		sess, err := openSession(ctx, q, sessionID)
		if err != nil {
			return err
		}
		if sess.GroupID != nil {
			groupID = *sess.GroupID
		}

		totals, err := q.SessionTotals(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("session totals: %w", err)
		}
		changes, err := q.ListLevelChanges(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("level changes: %w", err)
		}

		res = FinishResult{
			SessionID:    sessionID,
			Free:         sess.Free,
			Total:        int(totals.Total),
			Good:         int(totals.Good),
			Hard:         int(totals.Hard),
			Again:        int(totals.Again),
			DurationSec:  max(int(now.Sub(sess.StartedAt).Seconds()), 0),
			LevelChanges: make([]LevelChange, len(changes)),
		}
		for i, c := range changes {
			res.LevelChanges[i] = LevelChange{WordID: c.WordID, Term: c.Term, LevelBefore: c.LevelBefore, LevelAfter: c.LevelAfter}
		}

		if err := q.FinishSession(ctx, store.FinishSessionParams{
			ID: sessionID, Now: now, Total: totals.Total, Good: totals.Good, Hard: totals.Hard,
			Again: totals.Again, DurationSec: int32(res.DurationSec),
		}); err != nil {
			return fmt.Errorf("finish session: %w", err)
		}
		return nil
	})
	if err != nil {
		return FinishResult{}, fmt.Errorf("training finish: %w", err)
	}

	s.bus.Publish(eventbus.SessionFinished{
		SessionID: sessionID, GroupID: groupID, Free: res.Free,
		Good: res.Good, Hard: res.Hard, Again: res.Again, DurationSec: res.DurationSec, FinishedAt: now,
	})
	return res, nil
}

// openSession блокирует сессию и проверяет, что она существует и ещё не завершена.
func openSession(ctx context.Context, q *store.Queries, id int64) (store.TrainingSession, error) {
	sess, err := q.GetSessionForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sess, httpx.NotFound("session not found")
	}
	if err != nil {
		return sess, fmt.Errorf("get session: %w", err)
	}
	if sess.FinishedAt != nil {
		return sess, httpx.Conflict("session_finished", "session is already finished")
	}
	return sess, nil
}
