//go:build integration

// Интеграционные тесты сервиса training на живом Postgres (запуск — SPEC §15).
package training

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
)

type fakeBus struct {
	mu     sync.Mutex
	events []eventbus.Event
}

func (b *fakeBus) Publish(e eventbus.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

func (b *fakeBus) all() []eventbus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.events)
}

type testEnv struct {
	svc   *Service
	pool  *pgxpool.Pool
	clock *clock.Fixed
	bus   *fakeBus
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	pool := dbtest.New(t)
	env := testEnv{pool: pool, clock: clock.NewFixed(qNow), bus: &fakeBus{}}
	env.svc = NewService(pool, env.clock, env.bus)
	env.svc.newRand = func() *rand.Rand { return rng(1) } // детерминированно
	return env
}

func (e testEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e testEnv) group(t *testing.T, name string) int64 {
	t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO groups (name, created_at) VALUES ($1, $2) RETURNING id`, name, qNow).Scan(&id); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	return id
}

// word вставляет слово напрямую — чтобы точно задать уровень и даты.
func (e testEnv) word(t *testing.T, groupID int64, term string, level int16, next time.Time, examples ...string) int64 {
	t.Helper()
	ctx := context.Background()
	forms := []string{}
	if strings.Contains(term, "_") {
		forms = strings.Split(term, "_")
	}
	var id int64
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO words (term, forms, translation, level, next_review_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		term, forms, "tr_"+term, level, next, qNow.AddDate(0, 0, -30)).Scan(&id); err != nil {
		t.Fatalf("insert word: %v", err)
	}
	e.exec(t, `INSERT INTO word_groups (word_id, group_id) VALUES ($1, $2)`, id, groupID)
	for i, ex := range examples {
		e.exec(t, `INSERT INTO word_examples (word_id, position, sentence) VALUES ($1, $2, $3)`, id, i, ex)
	}
	return id
}

type srsRow struct {
	Level        int16
	Streak       int16
	NextReviewAt time.Time
	LastGoodAt   *time.Time
}

func (e testEnv) srs(t *testing.T, wordID int64) srsRow {
	t.Helper()
	var r srsRow
	if err := e.pool.QueryRow(context.Background(),
		`SELECT level, streak, next_review_at, last_good_at FROM words WHERE id = $1`, wordID).
		Scan(&r.Level, &r.Streak, &r.NextReviewAt, &r.LastGoodAt); err != nil {
		t.Fatalf("srs: %v", err)
	}
	return r
}

func (e testEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (e testEnv) mustStart(t *testing.T, p StartParams) StartResult {
	t.Helper()
	res, err := e.svc.Start(context.Background(), p)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return res
}

func (e testEnv) mustAnswer(t *testing.T, sessionID, wordID int64, a Answer) AnswerResult {
	t.Helper()
	res, err := e.svc.Answer(context.Background(), sessionID, AnswerParams{WordID: wordID, Answer: a})
	if err != nil {
		t.Fatalf("Answer(%d, %s): %v", wordID, a, err)
	}
	return res
}

func TestService_Start(t *testing.T) {
	env := newTestEnv(t)
	g := env.group(t, "g")
	other := env.group(t, "other")
	overdue := env.word(t, g, "overdue", 2, at(8, 9, 0))
	today := env.word(t, g, "today", 3, at(10, 23, 0)) // позже now, но сегодня — due
	fresh := env.word(t, g, "fresh", 1, at(1, 9, 0))
	env.word(t, g, "tomorrow", 2, at(11, 0, 0))
	archived := env.word(t, g, "archived", 5, at(1, 0, 0))
	env.word(t, other, "foreign", 2, at(8, 0, 0))

	res := env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyEnRu})
	if res.SessionID == nil {
		t.Fatal("session must be created")
	}
	if got := ids(res.Cards); !reflect.DeepEqual(got, []int64{overdue, today, fresh}) {
		t.Errorf("cards = %v, want [overdue today fresh] = %v", got, []int64{overdue, today, fresh})
	}
	var strategy string
	var free bool
	var started time.Time
	if err := env.pool.QueryRow(context.Background(),
		`SELECT strategy, free, started_at FROM training_sessions WHERE id = $1`, *res.SessionID).Scan(&strategy, &free, &started); err != nil {
		t.Fatalf("session row: %v", err)
	}
	if strategy != "en_ru" || free || !started.Equal(qNow) {
		t.Errorf("session row: %s %v %v", strategy, free, started)
	}

	t.Run("free includes archive and future words", func(t *testing.T) {
		res := env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyRuEn, Free: true})
		got := ids(res.Cards)
		if len(got) != 5 || !slices.Contains(got, archived) {
			t.Errorf("free cards = %v", got)
		}
	})

	t.Run("no cards → no session", func(t *testing.T) {
		empty := env.group(t, "empty")
		env.word(t, empty, "later", 3, at(20, 0, 0))
		before := env.count(t, `SELECT count(*) FROM training_sessions`)
		res := env.mustStart(t, StartParams{GroupID: empty, Strategy: StrategyEnRu})
		if res.SessionID != nil || res.Cards == nil || len(res.Cards) != 0 {
			t.Errorf("want {nil, []}, got %+v", res)
		}
		if after := env.count(t, `SELECT count(*) FROM training_sessions`); after != before {
			t.Errorf("session created for empty queue")
		}
	})

	t.Run("unknown group", func(t *testing.T) {
		_, err := env.svc.Start(context.Background(), StartParams{GroupID: 999999, Strategy: StrategyEnRu})
		assertHTTPError(t, err, 404, "not_found")
	})
}

func TestService_Start_NewWordsLimit(t *testing.T) {
	env := newTestEnv(t)
	g := env.group(t, "g")
	for i := range 12 {
		env.word(t, g, fmt.Sprintf("new%d", i), 1, at(1, 0, 0))
	}

	tests := []struct {
		name    string
		setting string // "" — ключа нет
		want    int
	}{
		{"default", "", DefaultNewLimit},
		{"from settings", "3", 3},
		{"zero", "0", 0},
		{"garbage falls back to default", `"abc"`, DefaultNewLimit},
		{"negative falls back to default", "-2", DefaultNewLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env.exec(t, `DELETE FROM settings`)
			if tt.setting != "" {
				env.exec(t, `INSERT INTO settings (key, value) VALUES ($1, $2::jsonb)`, SettingNewWordsLimit, tt.setting)
			}
			res := env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyEnRu})
			if len(res.Cards) != tt.want {
				t.Errorf("cards = %d, want %d", len(res.Cards), tt.want)
			}
		})
	}
}

func TestService_Start_Context(t *testing.T) {
	env := newTestEnv(t)
	g := env.group(t, "g")
	verb := env.word(t, g, "go_went_gone", 2, at(9, 0, 0), "I [go]", "I [went]", "I have [gone]")
	plain := env.word(t, g, "apple", 2, at(9, 0, 0), "An [apple]", "Red [apple]")
	bare := env.word(t, g, "pear", 2, at(9, 0, 0))

	res := env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyContext})
	byWord := map[int64][]Card{}
	for _, c := range res.Cards {
		byWord[c.WordID] = append(byWord[c.WordID], c)
	}
	if n := len(byWord[verb]); n != 3 {
		t.Errorf("verb subcards = %d, want 3", n)
	}
	for _, c := range byWord[verb] {
		if c.Example == nil || len(c.Example.Highlights) != 1 || !reflect.DeepEqual(c.Forms, []string{"go", "went", "gone"}) {
			t.Errorf("verb subcard = %+v", c)
		}
	}
	if n := len(byWord[plain]); n != 1 || byWord[plain][0].Example == nil {
		t.Errorf("plain word cards = %+v", byWord[plain])
	}
	if n := len(byWord[bare]); n != 1 || byWord[bare][0].Example != nil {
		t.Errorf("word without examples = %+v", byWord[bare])
	}
}

func TestService_Answer(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	g := env.group(t, "g")
	// streak 2, прошлое «Знаю» позавчера → третье «Знаю» поднимает на «Знаю» (3).
	learning := env.word(t, g, "learning", 2, at(9, 0, 0))
	env.exec(t, `UPDATE words SET streak = 2, last_good_at = $2 WHERE id = $1`, learning, at(8, 12, 0))
	fresh := env.word(t, g, "fresh", 1, at(1, 0, 0))
	sid := *env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyEnRu}).SessionID

	res := env.mustAnswer(t, sid, learning, AnswerGood)
	if res.LevelBefore != 2 || res.LevelAfter != 3 || !res.FirstInSession || res.RequeueAt != nil {
		t.Errorf("first good = %+v", res)
	}
	if got := env.srs(t, learning); got.Level != 3 || got.Streak != 0 || !got.NextReviewAt.Equal(qNow.AddDate(0, 0, 3)) ||
		got.LastGoodAt == nil || !got.LastGoodAt.Equal(qNow) {
		t.Errorf("srs after promotion = %+v", got)
	}

	// Повторный ответ на то же слово в сессии — только закрепление.
	res = env.mustAnswer(t, sid, learning, AnswerAgain)
	if res.LevelBefore != 3 || res.LevelAfter != 3 || res.FirstInSession || res.RequeueAt == nil || *res.RequeueAt < 3 || *res.RequeueAt > 5 {
		t.Errorf("repeat again = %+v", res)
	}
	if got := env.srs(t, learning); got.Level != 3 || got.Streak != 0 {
		t.Errorf("repeat must not change srs: %+v", got)
	}

	// Новое слово + «Не знаю» → «Изучаю», и вернуть в очередь.
	res = env.mustAnswer(t, sid, fresh, AnswerAgain)
	if res.LevelBefore != 1 || res.LevelAfter != 2 || !res.FirstInSession || res.RequeueAt == nil {
		t.Errorf("new word again = %+v", res)
	}

	if n := env.count(t, `SELECT count(*) FROM reviews WHERE session_id = $1`, sid); n != 3 {
		t.Errorf("reviews = %d, want 3", n)
	}
	if n := env.count(t, `SELECT count(*) FROM reviews WHERE session_id = $1 AND NOT first_in_session AND level_before = level_after`, sid); n != 1 {
		t.Errorf("repeat review rows = %d, want 1", n)
	}
	want := []eventbus.Event{
		eventbus.WordLevelChanged{WordID: learning, FromLevel: 2, ToLevel: 3, At: qNow},
		eventbus.WordLevelChanged{WordID: fresh, FromLevel: 1, ToLevel: 2, At: qNow},
	}
	if got := env.bus.all(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v\nwant %+v", got, want)
	}

	t.Run("errors", func(t *testing.T) {
		foreign := env.word(t, env.group(t, "other"), "foreign", 2, at(9, 0, 0))
		tests := []struct {
			name    string
			session int64
			word    int64
			status  int
			code    string
		}{
			{"unknown session", 999999, learning, 404, "not_found"},
			{"word from another group", sid, foreign, 400, "word_not_in_session"},
			{"unknown word", sid, 999999, 400, "word_not_in_session"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := env.svc.Answer(ctx, tt.session, AnswerParams{WordID: tt.word, Answer: AnswerGood})
				assertHTTPError(t, err, tt.status, tt.code)
			})
		}
	})
}

func TestService_Answer_Free(t *testing.T) {
	env := newTestEnv(t)
	g := env.group(t, "g")
	w := env.word(t, g, "archived", 5, at(1, 0, 0))
	before := env.srs(t, w)
	sid := *env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyEnRu, Free: true}).SessionID

	res := env.mustAnswer(t, sid, w, AnswerAgain)
	if res.LevelBefore != 5 || res.LevelAfter != 5 || !res.FirstInSession || res.RequeueAt == nil {
		t.Errorf("free answer = %+v", res)
	}
	if after := env.srs(t, w); !reflect.DeepEqual(after, before) {
		t.Errorf("free session changed srs: %+v → %+v", before, after)
	}
	if got := env.bus.all(); len(got) != 0 {
		t.Errorf("free session must not publish WordLevelChanged: %+v", got)
	}
}

func TestService_Finish(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	g := env.group(t, "g")
	a := env.word(t, g, "a", 2, at(9, 0, 0))
	b := env.word(t, g, "b", 3, at(9, 0, 0))
	c := env.word(t, g, "c", 3, at(9, 0, 0))
	sid := *env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyEnRu}).SessionID

	env.clock.Advance(30 * time.Second)
	env.mustAnswer(t, sid, a, AnswerGood)  // 2, streak 0→1 — уровень тот же
	env.mustAnswer(t, sid, b, AnswerAgain) // 3 → 2
	env.mustAnswer(t, sid, c, AnswerHard)  // 3, без изменений
	env.mustAnswer(t, sid, b, AnswerGood)  // повтор — не считается
	env.clock.Advance(60 * time.Second)

	res, err := env.svc.Finish(ctx, sid)
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	want := FinishResult{
		SessionID: sid, Total: 3, Good: 1, Hard: 1, Again: 1, DurationSec: 90,
		LevelChanges: []LevelChange{{WordID: b, Term: "b", LevelBefore: 3, LevelAfter: 2}},
	}
	if !reflect.DeepEqual(res, want) {
		t.Errorf("result = %+v\nwant %+v", res, want)
	}

	var total, good, dur int
	var finished *time.Time
	if err := env.pool.QueryRow(ctx, `SELECT total, good, duration_sec, finished_at FROM training_sessions WHERE id = $1`, sid).
		Scan(&total, &good, &dur, &finished); err != nil {
		t.Fatalf("session row: %v", err)
	}
	if total != 3 || good != 1 || dur != 90 || finished == nil || !finished.Equal(env.clock.Now()) {
		t.Errorf("session row: total %d good %d dur %d finished %v", total, good, dur, finished)
	}

	events := env.bus.all()
	wantEv := eventbus.SessionFinished{SessionID: sid, GroupID: g, Good: 1, Hard: 1, Again: 1, DurationSec: 90, FinishedAt: env.clock.Now()}
	if len(events) == 0 || !reflect.DeepEqual(events[len(events)-1], wantEv) {
		t.Errorf("last event = %+v, want %+v", events, wantEv)
	}

	_, err = env.svc.Finish(ctx, sid)
	assertHTTPError(t, err, 409, "session_finished")
	_, err = env.svc.Answer(ctx, sid, AnswerParams{WordID: a, Answer: AnswerGood})
	assertHTTPError(t, err, 409, "session_finished")
	_, err = env.svc.Finish(ctx, 999999)
	assertHTTPError(t, err, 404, "not_found")
}

func TestService_GroupDeletedMidSession(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	g := env.group(t, "g")
	w := env.word(t, g, "a", 2, at(9, 0, 0))
	sid := *env.mustStart(t, StartParams{GroupID: g, Strategy: StrategyEnRu}).SessionID
	env.exec(t, `DELETE FROM groups WHERE id = $1`, g) // group_id в сессии → NULL

	env.mustAnswer(t, sid, w, AnswerGood)
	if _, err := env.svc.Finish(ctx, sid); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	ev := env.bus.all()
	if fin, ok := ev[len(ev)-1].(eventbus.SessionFinished); !ok || fin.GroupID != 0 {
		t.Errorf("want SessionFinished with GroupID 0, got %+v", ev)
	}
}

// TestModule_HTTP — сквозная проверка роутов и JSON поверх живой БД.
func TestModule_HTTP(t *testing.T) {
	env := newTestEnv(t)
	g := env.group(t, "g")
	w := env.word(t, g, "go_went_gone", 2, at(9, 0, 0), "I [went] home")

	r := chi.NewRouter()
	r.Route("/api/training", (&Module{svc: env.svc}).Routes)
	do := func(path, body string, out any) int {
		t.Helper()
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		if out != nil && rr.Code == 200 {
			if err := json.NewDecoder(rr.Body).Decode(out); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
		return rr.Code
	}

	var start struct {
		SessionID *int64 `json:"sessionId"`
		Cards     []struct {
			WordID  int64    `json:"wordId"`
			Forms   []string `json:"forms"`
			Example *struct {
				Sentence   string `json:"sentence"`
				Highlights []struct{ Start, End int }
			} `json:"example"`
		} `json:"cards"`
	}
	if code := do("/api/training/start", fmt.Sprintf(`{"groupId":%d,"strategy":"context"}`, g), &start); code != 200 {
		t.Fatalf("start: %d", code)
	}
	if start.SessionID == nil || len(start.Cards) != 1 || start.Cards[0].Example == nil ||
		start.Cards[0].Example.Sentence != "I went home" || len(start.Cards[0].Forms) != 3 {
		t.Fatalf("start body = %+v", start)
	}
	base := fmt.Sprintf("/api/training/%d", *start.SessionID)

	var ans map[string]any
	if code := do(base+"/answer", fmt.Sprintf(`{"wordId":%d,"answer":"again"}`, w), &ans); code != 200 || ans["requeueAt"] == nil {
		t.Errorf("answer: %d %+v", code, ans)
	}
	var fin FinishResult
	if code := do(base+"/finish", "", &fin); code != 200 || fin.Total != 1 || fin.Again != 1 || fin.LevelChanges == nil {
		t.Errorf("finish: %d %+v", code, fin)
	}
	if code := do(base+"/finish", "", nil); code != 409 {
		t.Errorf("second finish: %d", code)
	}
}
