package importexport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/importexport/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db"
	"lexis/internal/platform/httpx"
	"lexis/internal/words"
)

// WordStore — то, что импорту/экспорту нужно от модуля words (реализует *words.Service).
// Импорт списка слов идёт через те же SaveCards, что и ввод: те же правила §4 и слияние дублей.
type WordStore interface {
	List(ctx context.Context, f words.ListFilter) ([]words.Word, error)
	SaveCards(ctx context.Context, cards []words.Card) (created, merged int, err error)
}

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	words WordStore
}

func NewService(pool *pgxpool.Pool, clk clock.Clock, ws WordStore) *Service {
	return &Service{pool: pool, clock: clk, words: ws}
}

// ImportResult — ответ POST /api/data/import. Kind говорит, что произошло:
// "words" — слова добавлены/слиты (как bulk), "backup" — данные заменены бэкапом.
type ImportResult struct {
	Kind     string            `json:"kind"`
	Created  int               `json:"created,omitempty"`
	Merged   int               `json:"merged,omitempty"`
	Errors   []words.LineError `json:"errors,omitempty"`
	Restored *RestoreCounts    `json:"restored,omitempty"`
}

type RestoreCounts struct {
	Groups       int `json:"groups"`
	Words        int `json:"words"`
	Sessions     int `json:"sessions"`
	Reviews      int `json:"reviews"`
	Achievements int `json:"achievements"`
}

// Import определяет формат по содержимому (см. DetectFormat) и импортирует.
func (s *Service) Import(ctx context.Context, data []byte) (ImportResult, error) {
	var rows []words.Row
	var err error
	switch DetectFormat(data) {
	case FormatBackup:
		b, err := DecodeBackup(data)
		if err != nil {
			return ImportResult{}, err
		}
		counts, err := s.Restore(ctx, b)
		if err != nil {
			return ImportResult{}, err
		}
		return ImportResult{Kind: "backup", Restored: &counts}, nil
	case FormatJSONList:
		rows, err = ParseWordList(data)
	default:
		rows, err = ParseCSV(data)
	}
	if err != nil {
		return ImportResult{}, err
	}

	parsed := words.ParseRows(rows)
	created, merged, err := s.words.SaveCards(ctx, parsed.Cards)
	if err != nil {
		return ImportResult{}, fmt.Errorf("import: %w", err)
	}
	return ImportResult{Kind: "words", Created: created, Merged: merged, Errors: parsed.Errors}, nil
}

// ExportCSV пишет все слова в формате импорта.
func (s *Service) ExportCSV(ctx context.Context, w io.Writer) error {
	ws, err := s.words.List(ctx, words.ListFilter{})
	if err != nil {
		return fmt.Errorf("export csv: %w", err)
	}
	if err := WriteCSV(w, ws); err != nil {
		return fmt.Errorf("export csv: write: %w", err)
	}
	return nil
}

// ExportBackup собирает полный снимок. Читаем в одной REPEATABLE READ транзакции:
// иначе ответ, записанный между запросами, мог бы сослаться на слово, которого нет в снимке.
func (s *Service) ExportBackup(ctx context.Context) (Backup, error) {
	b := Backup{Version: BackupVersion, ExportedAt: s.clock.Now(), Settings: map[string]json.RawMessage{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		q := store.New(tx)

		groups, err := q.ListAllGroups(ctx)
		if err != nil {
			return fmt.Errorf("groups: %w", err)
		}
		b.Groups = make([]BackupGroup, len(groups))
		for i, g := range groups {
			b.Groups[i] = BackupGroup{ID: g.ID, Name: g.Name, Color: g.Color, CreatedAt: g.CreatedAt}
		}

		ws, err := q.ListAllWords(ctx)
		if err != nil {
			return fmt.Errorf("words: %w", err)
		}
		b.Words = make([]BackupWord, len(ws))
		byWord := make(map[int64]*BackupWord, len(ws))
		for i, w := range ws {
			b.Words[i] = BackupWord{
				ID: w.ID, Term: w.Term, Forms: w.Forms, Translation: w.Translation, Level: w.Level, Streak: w.Streak,
				NextReviewAt: w.NextReviewAt, LastReviewedAt: w.LastReviewedAt, LastGoodAt: w.LastGoodAt, CreatedAt: w.CreatedAt,
				Examples: []string{}, GroupIDs: []int64{},
			}
			byWord[w.ID] = &b.Words[i]
		}
		exs, err := q.ListAllExamples(ctx)
		if err != nil {
			return fmt.Errorf("examples: %w", err)
		}
		for _, e := range exs {
			byWord[e.WordID].Examples = append(byWord[e.WordID].Examples, e.Sentence)
		}
		wgs, err := q.ListAllWordGroups(ctx)
		if err != nil {
			return fmt.Errorf("word groups: %w", err)
		}
		for _, wg := range wgs {
			byWord[wg.WordID].GroupIDs = append(byWord[wg.WordID].GroupIDs, wg.GroupID)
		}

		sessions, err := q.ListAllSessions(ctx)
		if err != nil {
			return fmt.Errorf("sessions: %w", err)
		}
		b.Sessions = make([]BackupSession, len(sessions))
		bySession := make(map[int64]*BackupSession, len(sessions))
		for i, ss := range sessions {
			b.Sessions[i] = BackupSession{
				ID: ss.ID, GroupID: ss.GroupID, Strategy: ss.Strategy, Free: ss.Free, StartedAt: ss.StartedAt,
				FinishedAt: ss.FinishedAt, Total: ss.Total, Good: ss.Good, Hard: ss.Hard, Again: ss.Again,
				DurationSec: ss.DurationSec, Reviews: []BackupReview{},
			}
			bySession[ss.ID] = &b.Sessions[i]
		}
		reviews, err := q.ListAllReviews(ctx)
		if err != nil {
			return fmt.Errorf("reviews: %w", err)
		}
		for _, r := range reviews {
			bs := bySession[r.SessionID]
			bs.Reviews = append(bs.Reviews, BackupReview{
				WordID: r.WordID, Answer: r.Answer, FirstInSession: r.FirstInSession,
				LevelBefore: r.LevelBefore, LevelAfter: r.LevelAfter, AnsweredAt: r.AnsweredAt,
			})
		}

		achs, err := q.ListAllAchievements(ctx)
		if err != nil {
			return fmt.Errorf("achievements: %w", err)
		}
		b.Achievements = make([]BackupAchievement, len(achs))
		for i, a := range achs {
			b.Achievements[i] = BackupAchievement{Code: a.Code, UnlockedAt: a.UnlockedAt}
		}

		settings, err := q.ListAllSettings(ctx)
		if err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		for _, st := range settings {
			b.Settings[st.Key] = json.RawMessage(st.Value)
		}
		return nil
	})
	if err != nil {
		return Backup{}, fmt.Errorf("export backup: %w", err)
	}
	return b, nil
}

// Restore заменяет все данные (включая settings) содержимым бэкапа — одной транзакцией:
// упало посередине — в базе остаётся то, что было. Бэкап должен пройти validateBackup.
func (s *Service) Restore(ctx context.Context, b Backup) (RestoreCounts, error) {
	if err := validateBackup(b); err != nil {
		return RestoreCounts{}, err
	}
	var c RestoreCounts
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.TruncateData(ctx); err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
		if err := q.DeleteSettings(ctx); err != nil {
			return fmt.Errorf("delete settings: %w", err)
		}

		groupID := make(map[int64]int64, len(b.Groups)) // id в файле → новый id
		for _, g := range b.Groups {
			id, err := q.RestoreGroup(ctx, store.RestoreGroupParams{Name: g.Name, Color: g.Color, CreatedAt: g.CreatedAt})
			if err != nil {
				return fmt.Errorf("group %q: %w", g.Name, err)
			}
			groupID[g.ID] = id
		}

		wordID := make(map[int64]int64, len(b.Words))
		var exs []store.RestoreExamplesParams
		var wgs []store.RestoreWordGroupsParams
		for _, w := range b.Words {
			forms := w.Forms
			if forms == nil {
				forms = []string{} // NOT NULL — pgx пишет nil как NULL
			}
			id, err := q.RestoreWord(ctx, store.RestoreWordParams{
				Term: w.Term, Forms: forms, Translation: w.Translation, Level: w.Level, Streak: w.Streak,
				NextReviewAt: w.NextReviewAt, LastReviewedAt: w.LastReviewedAt, LastGoodAt: w.LastGoodAt, CreatedAt: w.CreatedAt,
			})
			if err != nil {
				return fmt.Errorf("word %q: %w", w.Term, err)
			}
			wordID[w.ID] = id
			for i, e := range w.Examples {
				exs = append(exs, store.RestoreExamplesParams{WordID: id, Position: int16(i), Sentence: e})
			}
			for _, gid := range w.GroupIDs {
				wgs = append(wgs, store.RestoreWordGroupsParams{WordID: id, GroupID: groupID[gid]})
			}
		}
		// COPY, а не INSERT по строке: ответов за год — десятки тысяч.
		if _, err := q.RestoreExamples(ctx, exs); err != nil {
			return fmt.Errorf("examples: %w", err)
		}
		if _, err := q.RestoreWordGroups(ctx, wgs); err != nil {
			return fmt.Errorf("word groups: %w", err)
		}

		var reviews []store.RestoreReviewsParams
		for _, ss := range b.Sessions {
			var gid *int64
			if ss.GroupID != nil {
				id := groupID[*ss.GroupID]
				gid = &id
			}
			sid, err := q.RestoreSession(ctx, store.RestoreSessionParams{
				GroupID: gid, Strategy: ss.Strategy, Free: ss.Free, StartedAt: ss.StartedAt, FinishedAt: ss.FinishedAt,
				Total: ss.Total, Good: ss.Good, Hard: ss.Hard, Again: ss.Again, DurationSec: ss.DurationSec,
			})
			if err != nil {
				return fmt.Errorf("session %d: %w", ss.ID, err)
			}
			for _, r := range ss.Reviews {
				reviews = append(reviews, store.RestoreReviewsParams{
					SessionID: sid, WordID: wordID[r.WordID], Answer: r.Answer, FirstInSession: r.FirstInSession,
					LevelBefore: r.LevelBefore, LevelAfter: r.LevelAfter, AnsweredAt: r.AnsweredAt,
				})
			}
		}
		if _, err := q.RestoreReviews(ctx, reviews); err != nil {
			return fmt.Errorf("reviews: %w", err)
		}

		for _, a := range b.Achievements {
			if err := q.RestoreAchievement(ctx, store.RestoreAchievementParams{Code: a.Code, UnlockedAt: a.UnlockedAt}); err != nil {
				return fmt.Errorf("achievement %s: %w", a.Code, err)
			}
		}
		for k, v := range b.Settings {
			if err := q.RestoreSetting(ctx, store.RestoreSettingParams{Key: k, Value: v}); err != nil {
				return fmt.Errorf("setting %s: %w", k, err)
			}
		}

		c = RestoreCounts{Groups: len(b.Groups), Words: len(b.Words), Sessions: len(b.Sessions), Reviews: len(reviews), Achievements: len(b.Achievements)}
		return nil
	})
	if err != nil {
		// Нарушен CHECK/UNIQUE (уровень 7, дубль слова в файле) — проблема файла, а не сервера.
		if db.IsIntegrityViolation(err) {
			return RestoreCounts{}, &httpx.Error{Status: http.StatusBadRequest, Code: "invalid_backup", Message: "backup violates data constraints", Err: err}
		}
		return RestoreCounts{}, fmt.Errorf("restore: %w", err)
	}
	return c, nil
}

// Reset удаляет все данные, кроме настроек (§12).
func (s *Service) Reset(ctx context.Context) error {
	if err := store.New(s.pool).TruncateData(ctx); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	return nil
}
