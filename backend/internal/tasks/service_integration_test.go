//go:build integration

// Интеграционные тесты модуля tasks на живом Postgres (запуск — SPEC §15).
package tasks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
)

var testNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))

// recBus запоминает опубликованные события (тесты однопоточные — мьютекс не нужен).
type recBus struct{ events []eventbus.Event }

func (b *recBus) Publish(e eventbus.Event) { b.events = append(b.events, e) }

// testBus — шина последнего newTestService: тестам про события она нужна, остальным — нет.
var testBus *recBus

func newTestService(t *testing.T) (*Service, *clock.Fixed) {
	t.Helper()
	clk := clock.NewFixed(testNow)
	testBus = &recBus{}
	return NewService(dbtest.New(t), clk, testBus), clk
}

func assertCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) {
		t.Fatalf("want *httpx.Error %d %q, got %v", status, code, err)
	}
	if he.Status != status || he.Code != code {
		t.Errorf("got %d %q, want %d %q", he.Status, he.Code, status, code)
	}
}

// must — для коротких цепочек в тестах: must(s.Create(...)). t в него не передать (Go не
// разворачивает несколько результатов вместе с другими аргументами), поэтому panic — тест всё равно упадёт.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// order — id дел колонки по порядку (все, и выполненные).
func order(t *testing.T, s *Service, column int64) []int64 {
	t.Helper()
	all, err := s.Tasks(context.Background(), FilterAll)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, x := range all {
		if x.ColumnID == column {
			ids = append(ids, x.ID)
		}
	}
	return ids
}

func TestTypes(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	study := must(s.CreateType(ctx, TypeInput{Name: " Учёба ", Color: new("#16A34A")}))
	life := must(s.CreateType(ctx, TypeInput{Name: "Жизнь"}))
	if study.Name != "Учёба" || *study.Color != "#16a34a" || study.Position != 0 || life.Position != 1 {
		t.Errorf("created: %+v %+v", study, life)
	}
	_, err := s.CreateType(ctx, TypeInput{Name: "учёба"})
	assertCode(t, err, 409, "duplicate_type")

	upd := must(s.UpdateType(ctx, study.ID, TypePatch{Color: httpx.Optional[string]{Set: true}}))
	if upd.Color != nil || upd.Name != "Учёба" {
		t.Errorf("clear color: %+v", upd)
	}
	_, err = s.UpdateType(ctx, life.ID, TypePatch{Name: new("УЧЁБА")})
	assertCode(t, err, 409, "duplicate_type")
	_, err = s.UpdateType(ctx, 999, TypePatch{})
	assertCode(t, err, 404, "not_found")

	if err := s.ReorderTypes(ctx, []int64{life.ID, study.ID}); err != nil {
		t.Fatal(err)
	}
	if ts := must(s.Types(ctx)); ts[0].ID != life.ID || ts[1].ID != study.ID {
		t.Errorf("order: %+v", ts)
	}
	assertCode(t, s.ReorderTypes(ctx, []int64{life.ID}), 400, "invalid_order")

	// Удаление типа: дела остаются без типа.
	col := must(s.CreateColumn(ctx, "Входящие"))
	task := must(s.Create(ctx, TaskInput{Title: "Курсовая", Blocks: 6, Importance: 8, TypeID: &study.ID, ColumnID: col.ID}))
	if err := s.DeleteType(ctx, study.ID); err != nil {
		t.Fatal(err)
	}
	if got := must(s.Tasks(ctx, FilterOpen)); len(got) != 1 || got[0].ID != task.ID || got[0].TypeID != nil {
		t.Errorf("task after type delete: %+v", got)
	}
	assertCode(t, s.DeleteType(ctx, study.ID), 404, "not_found")
}

func TestTasks_CreateUpdate(t *testing.T) {
	s, clk := newTestService(t)
	ctx := context.Background()
	col := must(s.CreateColumn(ctx, "Входящие"))
	typ := must(s.CreateType(ctx, TypeInput{Name: "Учёба"}))

	a := must(s.Create(ctx, TaskInput{Title: "Отчёт", Blocks: 2, Importance: 5, Deadline: new("2026-11-07"), TypeID: &typ.ID, ColumnID: col.ID}))
	b := must(s.Create(ctx, TaskInput{Title: "Шкаф", Blocks: 4, Importance: 2, ColumnID: col.ID}))
	if *a.Deadline != "2026-11-07" || a.Position != 0 || b.Position != 1 || a.Done || !a.CreatedAt.Equal(testNow) {
		t.Errorf("created: %+v %+v", a, b)
	}
	_, err := s.Create(ctx, TaskInput{Title: "x", Blocks: 1, Importance: 1, ColumnID: 999})
	assertCode(t, err, 400, "invalid_column")
	_, err = s.Create(ctx, TaskInput{Title: "x", Blocks: 1, Importance: 1, ColumnID: col.ID, TypeID: new(int64(999))})
	assertCode(t, err, 400, "invalid_type")

	clk.Advance(time.Hour)
	done := must(s.Update(ctx, a.ID, Patch{Done: new(true), Deadline: httpx.Optional[string]{Set: true}}))
	if !done.Done || !done.DoneAt.Equal(testNow.Add(time.Hour)) || done.Deadline != nil || *done.TypeID != typ.ID || !done.UpdatedAt.Equal(testNow.Add(time.Hour)) {
		t.Errorf("done: %+v", done)
	}
	// TaskCompleted — один раз: повторное done и правка выполненного дела событие не дублируют.
	must(s.Update(ctx, a.ID, Patch{Done: new(true)}))
	must(s.Update(ctx, a.ID, Patch{Title: new("Отчёт готов")}))
	if want := []eventbus.Event{eventbus.TaskCompleted{TaskID: a.ID, At: testNow.Add(time.Hour).UTC()}}; !reflect.DeepEqual(utcEvents(testBus.events), want) {
		t.Errorf("events = %+v, want %+v", testBus.events, want)
	}
	_, err = s.Update(ctx, a.ID, Patch{TypeID: httpx.Optional[int64]{Set: true, Value: new(int64(999))}})
	assertCode(t, err, 400, "invalid_type")
	_, err = s.Update(ctx, 999, Patch{})
	assertCode(t, err, 404, "not_found")

	// Фильтр: открытые / выполненные / все.
	for f, want := range map[Filter][]int64{FilterOpen: {b.ID}, FilterDone: {a.ID}, FilterAll: {a.ID, b.ID}} {
		got := must(s.Tasks(ctx, f))
		ids := make([]int64, len(got))
		for i, x := range got {
			ids[i] = x.ID
		}
		if !reflect.DeepEqual(ids, want) {
			t.Errorf("filter %s: %v, want %v", f, ids, want)
		}
	}

	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	assertCode(t, s.Delete(ctx, a.ID), 404, "not_found")
}

func TestTasks_Move(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	todo := must(s.CreateColumn(ctx, "Todo"))
	doing := must(s.CreateColumn(ctx, "Doing"))
	mk := func(title string, col int64) int64 {
		return must(s.Create(ctx, TaskInput{Title: title, Blocks: 1, Importance: 5, ColumnID: col})).ID
	}
	a, b, c := mk("a", todo.ID), mk("b", todo.ID), mk("c", todo.ID)
	x := mk("x", doing.ID)

	if err := s.Move(ctx, c, MoveInput{ColumnID: todo.ID, BeforeID: &a}); err != nil {
		t.Fatal(err)
	}
	if got := order(t, s, todo.ID); !reflect.DeepEqual(got, []int64{c, a, b}) {
		t.Errorf("within column: %v", got)
	}
	if err := s.Move(ctx, a, MoveInput{ColumnID: doing.ID, BeforeID: &x}); err != nil {
		t.Fatal(err)
	}
	if got := order(t, s, doing.ID); !reflect.DeepEqual(got, []int64{a, x}) {
		t.Errorf("to other column: %v", got)
	}
	if got := order(t, s, todo.ID); !reflect.DeepEqual(got, []int64{c, b}) {
		t.Errorf("source column: %v", got)
	}
	if err := s.Move(ctx, b, MoveInput{ColumnID: doing.ID}); err != nil { // null — в конец
		t.Fatal(err)
	}
	if got := order(t, s, doing.ID); !reflect.DeepEqual(got, []int64{a, x, b}) {
		t.Errorf("to end: %v", got)
	}

	assertCode(t, s.Move(ctx, c, MoveInput{ColumnID: doing.ID, BeforeID: &c}), 400, "invalid_task")
	assertCode(t, s.Move(ctx, c, MoveInput{ColumnID: todo.ID, BeforeID: &x}), 400, "invalid_task") // x в другой колонке
	assertCode(t, s.Move(ctx, c, MoveInput{ColumnID: 999}), 400, "invalid_column")
	assertCode(t, s.Move(ctx, 999, MoveInput{ColumnID: todo.ID}), 404, "not_found")
}

func TestColumns(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	todo := must(s.CreateColumn(ctx, "Todo"))
	doing := must(s.CreateColumn(ctx, "Doing"))
	empty := must(s.CreateColumn(ctx, "Пусто"))

	if c := must(s.RenameColumn(ctx, todo.ID, "  Входящие ")); c.Name != "Входящие" {
		t.Errorf("rename: %+v", c)
	}
	_, err := s.RenameColumn(ctx, 999, "x")
	assertCode(t, err, 404, "not_found")
	if err := s.ReorderColumns(ctx, []int64{doing.ID, todo.ID, empty.ID}); err != nil {
		t.Fatal(err)
	}
	if cs := must(s.Columns(ctx)); cs[0].ID != doing.ID || cs[2].ID != empty.ID {
		t.Errorf("order: %+v", cs)
	}

	mk := func(title string, col int64) int64 {
		return must(s.Create(ctx, TaskInput{Title: title, Blocks: 1, Importance: 5, ColumnID: col})).ID
	}
	x := mk("x", doing.ID)
	a, b := mk("a", todo.ID), mk("b", todo.ID)
	must(s.Update(ctx, b, Patch{Done: new(true)})) // выполненные тоже переезжают

	if err := s.DeleteColumn(ctx, empty.ID, nil); err != nil {
		t.Fatalf("delete empty column: %v", err)
	}
	assertCode(t, s.DeleteColumn(ctx, todo.ID, nil), 409, "column_not_empty")
	assertCode(t, s.DeleteColumn(ctx, todo.ID, &todo.ID), 400, "invalid_column")
	assertCode(t, s.DeleteColumn(ctx, todo.ID, new(int64(999))), 400, "invalid_column")
	if err := s.DeleteColumn(ctx, todo.ID, &doing.ID); err != nil {
		t.Fatal(err)
	}
	if got := order(t, s, doing.ID); !reflect.DeepEqual(got, []int64{x, a, b}) {
		t.Errorf("moved tasks: %v, want [x a b]", got)
	}
	assertCode(t, s.DeleteColumn(ctx, todo.ID, nil), 404, "not_found")
}

func TestModule_HTTP(t *testing.T) {
	s, _ := newTestService(t)
	r := chi.NewRouter()
	r.Route("/api/tasks", (&Module{svc: s}).Routes)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rr
	}

	if rr := do(http.MethodPost, "/api/tasks/columns", `{"name":"Входящие"}`); rr.Code != 201 || !strings.Contains(rr.Body.String(), `"name":"Входящие"`) {
		t.Fatalf("create column: %d %s", rr.Code, rr.Body)
	}
	if rr := do(http.MethodPost, "/api/tasks/types", `{"name":"Учёба","color":"#16a34a"}`); rr.Code != 201 {
		t.Fatalf("create type: %d %s", rr.Code, rr.Body)
	}
	rr := do(http.MethodPost, "/api/tasks", `{"title":"Отчёт","blocks":2,"importance":7,"deadline":"2026-11-07","typeId":1,"columnId":1}`)
	if rr.Code != 201 || !strings.Contains(rr.Body.String(), `"deadline":"2026-11-07"`) {
		t.Fatalf("create task: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodPatch, "/api/tasks/1", `{"done":true,"typeId":null}`); rr.Code != 200 ||
		!strings.Contains(rr.Body.String(), `"done":true`) || !strings.Contains(rr.Body.String(), `"typeId":null`) {
		t.Errorf("patch: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodGet, "/api/tasks", ""); rr.Code != 200 || rr.Body.String() != "{\"tasks\":[]}\n" {
		t.Errorf("open tasks: %d %q", rr.Code, rr.Body)
	}
	if rr = do(http.MethodGet, "/api/tasks?done=all", ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"title":"Отчёт"`) {
		t.Errorf("all tasks: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodPost, "/api/tasks/1/move", `{"columnId":1,"beforeId":null}`); rr.Code != 204 {
		t.Errorf("move: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, "/api/tasks/columns/1", ""); rr.Code != 409 {
		t.Errorf("delete non-empty column: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodGet, "/api/tasks/types", ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"types":[{`) {
		t.Errorf("types: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, fmt.Sprintf("/api/tasks/%d", 1), ""); rr.Code != 204 {
		t.Errorf("delete: %d %s", rr.Code, rr.Body)
	}
}

// utcEvents приводит время событий к UTC: DeepEqual сравнивает и зону, а pgx отдаёт время не в MSK.
func utcEvents(es []eventbus.Event) []eventbus.Event {
	out := make([]eventbus.Event, len(es))
	for i, e := range es {
		switch v := e.(type) {
		case eventbus.TaskCompleted:
			v.At = v.At.UTC()
			out[i] = v
		case eventbus.FocusFinished:
			v.At = v.At.UTC()
			out[i] = v
		default:
			out[i] = e
		}
	}
	return out
}
