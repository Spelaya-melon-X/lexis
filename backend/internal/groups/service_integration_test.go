//go:build integration

// Интеграционные тесты сервиса groups на живом Postgres (запуск — SPEC §15).
package groups

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
	"lexis/internal/words"
)

// Среда, 4 ноября 2026: неделя началась 2-го (пн), месяц — 1-го.
var testNow = at(4, 12, 0)

type nopBus struct{}

func (nopBus) Publish(eventbus.Event) {}

type testEnv struct {
	svc   *Service
	words *words.Service
	pool  *pgxpool.Pool
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	pool := dbtest.New(t)
	clk := clock.NewFixed(testNow)
	ws := words.NewService(pool, clk, nopBus{})
	return testEnv{svc: NewService(pool, clk, ws), words: ws, pool: pool}
}

func (e testEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e testEnv) scalar(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return n
}

func (e testEnv) group(t *testing.T, name string) int64 {
	t.Helper()
	return e.scalar(t, `INSERT INTO groups (name, created_at) VALUES ($1, $2) RETURNING id`, name, testNow)
}

func (e testEnv) word(t *testing.T, term string, level int16, next time.Time, groupIDs ...int64) int64 {
	t.Helper()
	id := e.scalar(t, `INSERT INTO words (term, translation, level, next_review_at, created_at)
	                   VALUES ($1, $2, $3, $4, $5) RETURNING id`, term, "tr_"+term, level, next, testNow.AddDate(0, 0, -30))
	for _, g := range groupIDs {
		e.exec(t, `INSERT INTO word_groups (word_id, group_id) VALUES ($1, $2)`, id, g)
	}
	return id
}

// session — сессия группы; finished nil — не завершена.
func (e testEnv) session(t *testing.T, groupID int64, finished *time.Time, total, good int, free bool) int64 {
	t.Helper()
	started := testNow.AddDate(0, 0, -40)
	if finished != nil {
		started = finished.Add(-5 * time.Minute)
	}
	return e.scalar(t, `INSERT INTO training_sessions (group_id, strategy, free, started_at, finished_at, total, good)
	                    VALUES ($1, 'en_ru', $2, $3, $4, $5, $6) RETURNING id`, groupID, free, started, finished, total, good)
}

func tptr(t time.Time) *time.Time { return &t }

func TestService_List(t *testing.T) {
	env := newTestEnv(t)
	a := env.group(t, "a")
	b := env.group(t, "b") // пустая
	c := env.group(t, "c")
	d := env.group(t, "d")

	env.word(t, "new", 1, at(1, 0, 0), a)
	env.word(t, "overdue", 2, at(3, 9, 0), a, c) // в двух группах
	env.word(t, "future", 3, at(10, 0, 0), a, d)
	env.word(t, "archived", 5, at(1, 0, 0), a)
	env.word(t, "today", 4, at(4, 23, 0), c) // сегодня позже now — тоже due

	env.session(t, a, tptr(at(3, 22, 0)), 3, 3, false) // вчера — не в счёт
	env.session(t, a, nil, 0, 0, false)                // начата, не завершена — не в счёт
	env.session(t, c, tptr(at(4, 8, 0)), 1, 1, true)   // free сегодня — засчитывает (§5)

	got, err := env.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []Group{
		{ID: a, Name: "a", WordCount: 4, Due: 2, RepeatedToday: false, Levels: Levels{1, 1, 1, 0, 1}},
		{ID: b, Name: "b", WordCount: 0, Due: 0, RepeatedToday: true},
		{ID: c, Name: "c", WordCount: 2, Due: 2, RepeatedToday: true, Levels: Levels{0, 1, 0, 1, 0}},
		{ID: d, Name: "d", WordCount: 1, Due: 0, RepeatedToday: true, Levels: Levels{0, 0, 1, 0, 0}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d groups: %+v", len(got), got)
	}
	for i := range want {
		g := got[i]
		g.CreatedAt, g.Color = time.Time{}, nil
		if !reflect.DeepEqual(g, want[i]) {
			t.Errorf("group %d:\n got  %+v\n want %+v", i, g, want[i])
		}
	}
}

func TestService_Get(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a := env.group(t, "a")
	other := env.group(t, "other")
	env.word(t, "new", 1, at(1, 0, 0), a)
	env.word(t, "known", 3, at(10, 0, 0), a)
	env.word(t, "master", 4, at(10, 0, 0), a)
	env.word(t, "archived", 5, at(1, 0, 0), a)
	env.word(t, "foreign", 2, at(1, 0, 0), other)

	octTZ := func(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, msk) }
	env.session(t, a, tptr(octTZ(28)), 4, 4, false)
	env.session(t, a, tptr(octTZ(31)), 1, 1, false)
	env.session(t, a, tptr(at(2, 10, 0)), 4, 2, false)
	env.session(t, a, tptr(at(3, 10, 0)), 2, 2, true)
	env.session(t, a, tptr(at(4, 9, 0)), 10, 5, false)
	env.session(t, a, nil, 0, 0, false)
	env.session(t, other, tptr(at(4, 9, 0)), 5, 0, false)

	d, err := env.svc.Get(ctx, a, RangeWeek)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Group.ID != a || d.Group.WordCount != 4 || d.Group.Levels != (Levels{1, 0, 1, 1, 1}) {
		t.Errorf("group = %+v", d.Group)
	}
	an := d.Analytics
	if an.MasteredPercent != 50 || an.SessionsThisWeek != 3 || an.SessionsThisMonth != 3 || an.Range != RangeWeek {
		t.Errorf("analytics = %+v", an)
	}
	wantShare := map[string]float64{"2026-10-31": 1, "2026-11-02": 0.5, "2026-11-03": 1, "2026-11-04": 0.5}
	if len(an.Productivity) != 7 || an.Productivity[0].Day != "2026-10-29" {
		t.Fatalf("productivity window = %+v", an.Productivity)
	}
	for _, p := range an.Productivity {
		want, ok := wantShare[p.Day]
		switch {
		case ok && (p.GoodShare == nil || *p.GoodShare != want || p.Sessions != 1):
			t.Errorf("%s: got %d %v, want 1 %v", p.Day, p.Sessions, deref(p.GoodShare), want)
		case !ok && (p.GoodShare != nil || p.Sessions != 0):
			t.Errorf("%s: want empty, got %+v", p.Day, p)
		}
	}
	if len(d.Words) != 4 {
		t.Errorf("words = %d, want 4", len(d.Words))
	}

	d, err = env.svc.Get(ctx, a, RangeMonth)
	if err != nil {
		t.Fatalf("Get month: %v", err)
	}
	sessions := 0
	for _, p := range d.Analytics.Productivity {
		sessions += p.Sessions
	}
	if len(d.Analytics.Productivity) != 30 || sessions != 5 { // 28 октября попадает в 30-дневное окно
		t.Errorf("month: %d points, %d sessions", len(d.Analytics.Productivity), sessions)
	}

	_, err = env.svc.Get(ctx, 999999, RangeWeek)
	assertHTTPError(t, err, 404, "not_found")
}

func TestService_Update(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	food := env.group(t, "food")
	env.group(t, "verbs")
	env.word(t, "apple", 2, at(1, 0, 0), food)

	str := func(s string) *string { return &s }

	g, err := env.svc.Update(ctx, food, UpdateParams{Name: str(" #Еда "), Color: OptionalString{Set: true, Value: str("#FF00AA")}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.Name != "еда" || g.Color == nil || *g.Color != "#ff00aa" || g.WordCount != 1 {
		t.Errorf("renamed = %+v", g)
	}

	// Цвет не передан — не меняется; имя — тоже.
	g, err = env.svc.Update(ctx, food, UpdateParams{})
	if err != nil || g.Name != "еда" || g.Color == nil {
		t.Errorf("noop update = %+v, %v", g, err)
	}
	// null очищает цвет.
	g, err = env.svc.Update(ctx, food, UpdateParams{Color: OptionalString{Set: true}})
	if err != nil || g.Color != nil || g.Name != "еда" {
		t.Errorf("clear color = %+v, %v", g, err)
	}
	// Своё же имя — не конфликт.
	if _, err := env.svc.Update(ctx, food, UpdateParams{Name: str("еда")}); err != nil {
		t.Errorf("rename to same: %v", err)
	}

	_, err = env.svc.Update(ctx, food, UpdateParams{Name: str("Verbs")})
	assertHTTPError(t, err, 409, "duplicate_group")
	_, err = env.svc.Update(ctx, 999999, UpdateParams{Name: str("x")})
	assertHTTPError(t, err, 404, "not_found")

	// Ввод с новым тегом попадает в переименованную группу, а не создаёт дубль.
	if _, err := env.words.Bulk(ctx, "pear || груша || || #ЕДА"); err != nil {
		t.Fatalf("Bulk: %v", err)
	}
	if n := env.scalar(t, `SELECT count(*) FROM groups`); n != 2 {
		t.Errorf("groups = %d, want 2", n)
	}
	if n := env.scalar(t, `SELECT count(*) FROM word_groups WHERE group_id = $1`, food); n != 2 {
		t.Errorf("words in renamed group = %d, want 2", n)
	}
}

func TestService_Delete(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	g := env.group(t, "g")
	w := env.word(t, "a", 2, at(1, 0, 0), g)
	s := env.session(t, g, tptr(at(4, 9, 0)), 1, 1, false)

	if err := env.svc.Delete(ctx, g); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n := env.scalar(t, `SELECT count(*) FROM words WHERE id = $1`, w); n != 1 {
		t.Error("word must survive group deletion")
	}
	if n := env.scalar(t, `SELECT count(*) FROM word_groups`); n != 0 {
		t.Errorf("word_groups left: %d", n)
	}
	if n := env.scalar(t, `SELECT count(*) FROM training_sessions WHERE id = $1 AND group_id IS NULL`, s); n != 1 {
		t.Error("session history must survive with group_id = NULL")
	}
	assertHTTPError(t, env.svc.Delete(ctx, g), 404, "not_found")
}

// TestModule_HTTP — сквозная проверка роутов и JSON поверх живой БД.
func TestModule_HTTP(t *testing.T) {
	env := newTestEnv(t)
	g := env.group(t, "g")
	env.word(t, "a", 2, at(1, 0, 0), g)

	r := chi.NewRouter()
	r.Route("/api/groups", (&Module{svc: env.svc}).Routes)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rr
	}

	rr := do(http.MethodGet, "/api/groups", "")
	var list struct{ Groups []Group }
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil || rr.Code != 200 || len(list.Groups) != 1 || list.Groups[0].Due != 1 {
		t.Fatalf("list: %d %+v %v", rr.Code, list, err)
	}

	path := fmt.Sprintf("/api/groups/%d", g)
	rr = do(http.MethodGet, path+"?range=week", "")
	var det struct {
		Group     Group
		Analytics struct {
			Levels       []int
			Productivity []DayPoint
		}
		Words []words.Word
	}
	if err := json.NewDecoder(rr.Body).Decode(&det); err != nil || rr.Code != 200 ||
		len(det.Analytics.Productivity) != 7 || len(det.Words) != 1 || det.Group.Levels != (Levels{0, 1, 0, 0, 0}) {
		t.Errorf("get: %d %+v %v", rr.Code, det, err)
	}
	if rr = do(http.MethodGet, path, ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"range":"month"`) {
		t.Errorf("default range: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodPatch, path, `{"color":"#123abc"}`); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"color":"#123abc"`) {
		t.Errorf("patch: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodPatch, path, `{"color":null}`); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"color":null`) {
		t.Errorf("patch null: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, path, ""); rr.Code != 204 {
		t.Errorf("delete: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodGet, path, ""); rr.Code != 404 {
		t.Errorf("get deleted: %d", rr.Code)
	}
}
