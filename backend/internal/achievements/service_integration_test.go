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
	if len(list) != 9 || list[0].Code != FirstSession {
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
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil || rr.Code != 200 || len(body.Achievements) != 9 {
		t.Fatalf("GET: %d %+v %v", rr.Code, body, err)
	}
	if a := body.Achievements[1]; a.Code != "streak_3" || a.Target != 3 || a.UnlockedAt != nil {
		t.Errorf("streak_3 = %+v", a)
	}
}
