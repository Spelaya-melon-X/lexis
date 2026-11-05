//go:build integration

// Интеграционные тесты achievements на живом Postgres (запуск — SPEC §15).
package achievements

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
)

var (
	msk     = time.FixedZone("MSK", 3*60*60)
	testNow = time.Date(2026, 11, 10, 12, 0, 0, 0, msk)
)

func day(d int) *time.Time {
	t := time.Date(2026, 11, d, 9, 0, 0, 0, msk)
	return &t
}

func newTestEnv(t *testing.T) (*Service, dbtest.Fixtures, *clock.Fixed) {
	t.Helper()
	pool := dbtest.New(t)
	clk := clock.NewFixed(testNow)
	return NewService(pool, clk), dbtest.Fixtures{Pool: pool, Now: testNow}, clk
}

func mustCheck(t *testing.T, svc *Service) []Code {
	t.Helper()
	got, err := svc.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return got
}

func TestService_Check(t *testing.T) {
	svc, fx, _ := newTestEnv(t)

	if got := mustCheck(t, svc); len(got) != 0 {
		t.Fatalf("empty DB unlocked %v", got)
	}

	// 3 дня подряд → first_session + streak_3; незавершённая сессия не считается.
	for _, d := range []int{8, 9, 10} {
		fx.Session(t, dbtest.Session{FinishedAt: day(d)})
	}
	fx.Session(t, dbtest.Session{})
	if got := mustCheck(t, svc); !slices.Equal(got, []Code{FirstSession, Streak3}) {
		t.Errorf("unlocked = %v", got)
	}
	// Повторная проверка ничего не разблокирует заново.
	if got := mustCheck(t, svc); len(got) != 0 {
		t.Errorf("second check unlocked %v", got)
	}

	// 10 слов на уровне ≥ 4 → learned_10.
	g := fx.Group(t, "g")
	for i := range 9 {
		fx.Word(t, fmt.Sprintf("m%d", i), 4, testNow)
	}
	fx.Word(t, "almost", 3, testNow, g)
	if got := mustCheck(t, svc); len(got) != 0 {
		t.Errorf("9 learned unlocked %v", got)
	}
	fx.Exec(t, `UPDATE words SET level = 5 WHERE term = 'almost'`)
	// Теперь и единственное слово группы g в архиве → first_archived_group.
	if got := mustCheck(t, svc); !slices.Equal(got, []Code{Learned10, FirstArchivedGroup}) {
		t.Errorf("unlocked = %v", got)
	}
}

func TestService_Check_ArchivedGroupNeedsAllWords(t *testing.T) {
	svc, fx, _ := newTestEnv(t)
	g := fx.Group(t, "g")
	fx.Group(t, "empty") // пустая группа «архивной» не считается
	fx.Word(t, "a", 5, testNow, g)
	fx.Word(t, "b", 4, testNow, g)
	if got := mustCheck(t, svc); slices.Contains(got, FirstArchivedGroup) {
		t.Errorf("group with a non-archived word unlocked %v", got)
	}
}

func TestService_List(t *testing.T) {
	svc, fx, clk := newTestEnv(t)
	ctx := context.Background()
	for i := range 12 {
		fx.Word(t, fmt.Sprintf("w%d", i), 4, testNow)
	}

	list, err := svc.List(ctx) // List сам проверяет и разблокирует (без гонки с подписчиком)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 15 || list[0].Code != FirstSession {
		t.Fatalf("list = %+v", list)
	}
	byCode := map[Code]Achievement{}
	for _, a := range list {
		byCode[a.Code] = a
	}
	if a := byCode[Learned10]; a.UnlockedAt == nil || !a.UnlockedAt.Equal(testNow) || a.Progress != 10 {
		t.Errorf("learned_10 = %+v", a)
	}
	if a := byCode[Learned50]; a.UnlockedAt != nil || a.Progress != 12 || a.Target != 50 {
		t.Errorf("learned_50 = %+v", a)
	}

	// Метрика упала (слова вернули из архива) — полученное остаётся, прогресс полный.
	fx.Exec(t, `UPDATE words SET level = 2`)
	clk.Advance(time.Hour)
	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, a := range list {
		if a.Code == Learned10 && (a.UnlockedAt == nil || !a.UnlockedAt.Equal(testNow) || a.Progress != a.Target) {
			t.Errorf("learned_10 after drop = %+v", a)
		}
	}
}

// Сквозной путь через настоящую шину: Publish → подписчик → Check → запись в БД.
func TestModule_UnlocksOnEvent(t *testing.T) {
	_, fx, clk := newTestEnv(t)
	bus := eventbus.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	New(fx.Pool, clk, bus)

	fx.Session(t, dbtest.Session{FinishedAt: day(10)})
	bus.Publish(eventbus.WordsAdded{})      // не наше событие — игнорируется
	bus.Publish(eventbus.SessionFinished{}) // проверка запускается
	bus.Close()                             // после Close все события гарантированно обработаны

	if n := fx.Int(t, `SELECT count(*) FROM achievements WHERE code = $1`, string(FirstSession)); n != 1 {
		t.Errorf("first_session not unlocked by event")
	}
}

func TestModule_HTTP(t *testing.T) {
	svc, _, _ := newTestEnv(t)
	r := chi.NewRouter()
	r.Route("/api/achievements", (&Module{svc: svc}).Routes)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/achievements", nil))
	var body struct {
		Achievements []struct {
			Code       string     `json:"code"`
			Progress   int        `json:"progress"`
			Target     int        `json:"target"`
			UnlockedAt *time.Time `json:"unlockedAt"`
		} `json:"achievements"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil || rr.Code != 200 || len(body.Achievements) != 15 {
		t.Fatalf("GET: %d %+v %v", rr.Code, body, err)
	}
	if a := body.Achievements[1]; a.Code != "streak_3" || a.Target != 3 || a.UnlockedAt != nil {
		t.Errorf("streak_3 = %+v", a)
	}
}

// Ачивки «Игры» (§18.3, B5): дела-листья с done_at и часы чистой работы в фокусе.
func TestService_Check_Game(t *testing.T) {
	svc, fx, _ := newTestEnv(t)
	fx.Exec(t, `INSERT INTO task_columns (name, position, created_at) VALUES ('c', 0, $1)`, testNow)
	for i := range 10 {
		done := any(testNow)
		if i == 9 {
			done = nil // девятое открытое — пока 9 выполненных
		}
		fx.Exec(t, `INSERT INTO tasks (title, blocks, importance, column_id, position, done_at, created_at, updated_at)
			VALUES ($1, 1, 5, 1, $2, $3, $4, $4)`, fmt.Sprintf("t%d", i), i, done, testNow)
	}
	// Сессии: 6 ч работы с часовым перерывом (= 5 ч), 5 ч ровно, и отменённая — не в счёт.
	start := testNow.Add(-24 * time.Hour)
	fx.Exec(t, `INSERT INTO focus_sessions (task_id, task_title, day, planned_blocks, started_at, work_started_at, finished_at, outcome) VALUES
		(1, 't0', '2026-11-04', 2, $1, $1, $1::timestamptz + interval '6 hours', 'done'),
		(2, 't1', '2026-11-04', 2, $1, $1, $1::timestamptz + interval '5 hours' - interval '1 second', 'stopped'),
		(3, 't2', '2026-11-04', 2, $1, $1, $1::timestamptz + interval '9 hours', 'abandoned')`, start)
	fx.Exec(t, `INSERT INTO focus_breaks (session_id, started_at, ended_at) VALUES (1, $1, $1::timestamptz + interval '1 hour')`, start)

	// 9 дел, 9:59:59 работы — ещё ничего.
	if got := mustCheck(t, svc); len(got) != 0 {
		t.Fatalf("unlocked too early: %v", got)
	}
	fx.Exec(t, `UPDATE tasks SET done_at = $1 WHERE title = 't9'`, testNow)
	fx.Exec(t, `UPDATE focus_sessions SET finished_at = finished_at + interval '1 second' WHERE id = 2`)
	if got := mustCheck(t, svc); !slices.Equal(got, []Code{GameTasks10, GameHours10}) {
		t.Errorf("unlocked = %v", got)
	}
}
