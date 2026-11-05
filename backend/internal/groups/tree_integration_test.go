//go:build integration

// Дерево групп (§18.2) на живом Postgres: агрегаты по поддереву, создание, перенос, удаление.
package groups

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/httpx"
	"lexis/internal/words"
)

// child — группа внутри parent напрямую в БД (минуя проверки сервиса).
func (e testEnv) child(t *testing.T, name string, parent int64) int64 {
	t.Helper()
	return e.scalar(t, `INSERT INTO groups (name, parent_id, created_at) VALUES ($1, $2, $3) RETURNING id`, name, parent, testNow)
}

func (e testEnv) byID(t *testing.T) map[int64]Group {
	t.Helper()
	list, err := e.svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	m := make(map[int64]Group, len(list))
	for _, g := range list {
		m[g.ID] = g
	}
	return m
}

func TestTree_SubtreeAggregates(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	//	travel ── airport ── gates
	//	       └─ hotel
	travel := env.group(t, "travel")
	airport := env.child(t, "airport", travel)
	gates := env.child(t, "gates", airport)
	hotel := env.child(t, "hotel", travel)

	env.word(t, "trip", 2, at(10, 0, 0), travel)            // своё слово родителя, не due
	env.word(t, "plane", 2, at(3, 0, 0), airport, hotel)    // в двух подгруппах — считается один раз
	env.word(t, "gate", 1, at(1, 0, 0), gates)              // новое — due
	env.word(t, "archived", 5, at(1, 0, 0), hotel)          // архив — не due
	env.session(t, airport, tptr(at(4, 9, 0)), 1, 1, false) // сегодня по airport

	g := env.byID(t)
	checks := []struct {
		id              int64
		words, own, due int
		repeated        bool
		levels          Levels
		parent          *int64
	}{
		{travel, 4, 1, 2, false, Levels{1, 2, 0, 0, 1}, nil}, // сессия подгруппы родителя не засчитывает
		{airport, 2, 1, 2, true, Levels{1, 1, 0, 0, 0}, &travel},
		{gates, 1, 1, 1, true, Levels{1, 0, 0, 0, 0}, &airport}, // предок тренировался сегодня
		{hotel, 2, 2, 1, false, Levels{0, 1, 0, 0, 1}, &travel},
	}
	for _, c := range checks {
		got := g[c.id]
		if got.WordCount != c.words || got.OwnWordCount != c.own || got.Due != c.due || got.RepeatedToday != c.repeated || got.Levels != c.levels {
			t.Errorf("%s: got words=%d own=%d due=%d repeated=%v levels=%v, want %d %d %d %v %v",
				got.Name, got.WordCount, got.OwnWordCount, got.Due, got.RepeatedToday, got.Levels, c.words, c.own, c.due, c.repeated, c.levels)
		}
		if (got.ParentID == nil) != (c.parent == nil) || (c.parent != nil && *got.ParentID != *c.parent) {
			t.Errorf("%s: parent = %v, want %v", got.Name, got.ParentID, c.parent)
		}
	}

	// Аналитика и слова родителя — по поддереву.
	d, err := env.svc.Get(ctx, travel, RangeWeek)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(d.Words) != 4 || d.Analytics.SessionsThisWeek != 1 || d.Analytics.Productivity[6].Sessions != 1 {
		t.Errorf("Get(travel): words=%d week=%d today=%+v", len(d.Words), d.Analytics.SessionsThisWeek, d.Analytics.Productivity[6])
	}
	// Тренировка родителя засчитывает всё поддерево.
	env.session(t, travel, tptr(at(4, 10, 0)), 4, 2, true)
	for id, gr := range env.byID(t) {
		if !gr.RepeatedToday {
			t.Errorf("after training the root, group %d (%s) is not repeated", id, gr.Name)
		}
	}
}

func TestTree_CreateMove(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	travel, err := env.svc.Create(ctx, CreateParams{Name: "#Travel"})
	if err != nil || travel.Name != "travel" || travel.ParentID != nil {
		t.Fatalf("Create root: %+v %v", travel, err)
	}
	airport, err := env.svc.Create(ctx, CreateParams{Name: "airport", ParentID: &travel.ID, Color: new("#ABCDEF")})
	if err != nil || *airport.ParentID != travel.ID || *airport.Color != "#abcdef" {
		t.Fatalf("Create child: %+v %v", airport, err)
	}
	gates, err := env.svc.Create(ctx, CreateParams{Name: "gates", ParentID: &airport.ID})
	if err != nil {
		t.Fatalf("Create grandchild: %v", err)
	}
	_, err = env.svc.Create(ctx, CreateParams{Name: "door", ParentID: &gates.ID})
	assertHTTPError(t, err, 400, "group_too_deep")
	_, err = env.svc.Create(ctx, CreateParams{Name: "x", ParentID: new(int64(999999))})
	assertHTTPError(t, err, 400, "invalid_parent")
	_, err = env.svc.Create(ctx, CreateParams{Name: "x", ParentID: new(int64(0))})
	assertHTTPError(t, err, 400, "invalid_parent")
	_, err = env.svc.Create(ctx, CreateParams{Name: "a/b"})
	assertHTTPError(t, err, 400, "invalid_group_name")
	_, err = env.svc.Create(ctx, CreateParams{Name: "AIRPORT"})
	assertHTTPError(t, err, 409, "duplicate_group")

	verbs, _ := env.svc.Create(ctx, CreateParams{Name: "verbs"})
	move := func(id int64, parent *int64) error {
		_, err := env.svc.Update(ctx, id, UpdateParams{ParentID: optional(parent)})
		return err
	}
	assertHTTPError(t, move(travel.ID, &gates.ID), 400, "group_cycle")
	assertHTTPError(t, move(airport.ID, &airport.ID), 400, "group_cycle")
	assertHTTPError(t, move(travel.ID, &verbs.ID), 400, "group_too_deep") // поддерево высоты 3 под корневую
	if err := move(airport.ID, &verbs.ID); err != nil {                   // высота 2 под корневую — ок
		t.Fatalf("move airport → verbs: %v", err)
	}
	if err := move(airport.ID, nil); err != nil { // null — в корень
		t.Fatalf("move airport → root: %v", err)
	}
	g := env.byID(t)
	if g[airport.ID].ParentID != nil || *g[gates.ID].ParentID != airport.ID {
		t.Errorf("after moves: airport=%v gates=%v", g[airport.ID].ParentID, g[gates.ID].ParentID)
	}
	// Переименование не трогает родителя.
	if gr, err := env.svc.Update(ctx, gates.ID, UpdateParams{Name: new("gate")}); err != nil || *gr.ParentID != airport.ID {
		t.Errorf("rename: %+v %v", gr, err)
	}
}

// optional — поле PATCH «передано»: значение или null.
func optional(v *int64) httpx.Optional[int64] { return httpx.Optional[int64]{Set: true, Value: v} }

func TestTree_Delete(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	travel := env.group(t, "travel")
	airport := env.child(t, "airport", travel)
	gates := env.child(t, "gates", airport)
	w := env.word(t, "plane", 2, at(3, 0, 0), airport, gates)
	s := env.session(t, airport, tptr(at(3, 9, 0)), 1, 1, false)

	// lift: gates поднимается к travel, слово и история остаются.
	if err := env.svc.Delete(ctx, airport, ChildrenLift); err != nil {
		t.Fatalf("Delete lift: %v", err)
	}
	g := env.byID(t)
	if len(g) != 2 || *g[gates].ParentID != travel || g[travel].WordCount != 1 {
		t.Errorf("after lift: %+v", g)
	}
	if n := env.scalar(t, `SELECT count(*) FROM training_sessions WHERE id = $1 AND group_id IS NULL`, s); n != 1 {
		t.Errorf("session group_id must be NULL after delete, got %d rows", n)
	}

	// delete: travel вместе с gates; слова остаются.
	if err := env.svc.Delete(ctx, travel, ChildrenDelete); err != nil {
		t.Fatalf("Delete subtree: %v", err)
	}
	if n := env.scalar(t, `SELECT count(*) FROM groups`); n != 0 {
		t.Errorf("groups left: %d", n)
	}
	if n := env.scalar(t, `SELECT count(*) FROM words WHERE id = $1`, w); n != 1 {
		t.Errorf("word deleted with groups")
	}
	assertHTTPError(t, env.svc.Delete(ctx, travel, ChildrenLift), 404, "not_found")
	assertHTTPError(t, env.svc.Delete(ctx, 1, "nope"), 400, "invalid_children")
}

func TestTree_HTTP(t *testing.T) {
	env := newTestEnv(t)
	r := chi.NewRouter()
	r.Route("/api/groups", (&Module{svc: env.svc}).Routes)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rr
	}

	rr := do(http.MethodPost, "/api/groups", `{"name":"travel"}`)
	if rr.Code != 201 || !strings.Contains(rr.Body.String(), `"parentId":null`) {
		t.Fatalf("create: %d %s", rr.Code, rr.Body)
	}
	travel := env.scalar(t, `SELECT id FROM groups WHERE name = 'travel'`)
	rr = do(http.MethodPost, "/api/groups", fmt.Sprintf(`{"name":"airport","parentId":%d}`, travel))
	if rr.Code != 201 || !strings.Contains(rr.Body.String(), fmt.Sprintf(`"parentId":%d`, travel)) {
		t.Fatalf("create child: %d %s", rr.Code, rr.Body)
	}
	airport := env.scalar(t, `SELECT id FROM groups WHERE name = 'airport'`)
	env.word(t, "plane", 2, at(3, 0, 0), airport)

	// Слова поддерева: ?subtree=true через groups.Get и words-фильтр.
	ws, err := env.words.List(context.Background(), words.ListFilter{GroupID: &travel, Subtree: true})
	if err != nil || len(ws) != 1 {
		t.Errorf("words subtree: %d %v", len(ws), err)
	}
	if ws, _ := env.words.List(context.Background(), words.ListFilter{GroupID: &travel}); len(ws) != 0 {
		t.Errorf("words own only: %d", len(ws))
	}

	if rr = do(http.MethodPatch, fmt.Sprintf("/api/groups/%d", airport), `{"parentId":null}`); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"parentId":null`) {
		t.Errorf("move to root: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodPatch, fmt.Sprintf("/api/groups/%d", travel), fmt.Sprintf(`{"parentId":%d}`, travel)); rr.Code != 400 || !strings.Contains(rr.Body.String(), "group_cycle") {
		t.Errorf("cycle: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, fmt.Sprintf("/api/groups/%d?children=all", travel), ""); rr.Code != 400 {
		t.Errorf("bad children: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, fmt.Sprintf("/api/groups/%d?children=delete", travel), ""); rr.Code != 204 {
		t.Errorf("delete: %d %s", rr.Code, rr.Body)
	}
}
