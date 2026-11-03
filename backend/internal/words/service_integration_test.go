//go:build integration

// Интеграционные тесты сервиса words на живом Postgres (запуск — SPEC §15).
package words

import (
	"context"
	"encoding/json"
	"fmt"
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

var testNow = time.Date(2026, 11, 2, 10, 0, 0, 0, time.FixedZone("MSK", 3*60*60))

// fakeBus запоминает опубликованные события.
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
	env := testEnv{pool: pool, clock: clock.NewFixed(testNow), bus: &fakeBus{}}
	env.svc = NewService(pool, env.clock, env.bus)
	return env
}

func (e testEnv) mustBulk(t *testing.T, text string) BulkResult {
	t.Helper()
	res, err := e.svc.Bulk(context.Background(), text)
	if err != nil {
		t.Fatalf("Bulk: %v", err)
	}
	return res
}

func (e testEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e testEnv) wordID(t *testing.T, term string) int64 {
	t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM words WHERE lower(term) = lower($1)`, term).Scan(&id); err != nil {
		t.Fatalf("word id %q: %v", term, err)
	}
	return id
}

func (e testEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (e testEnv) positions(t *testing.T, wordID int64) []int16 {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `SELECT position FROM word_examples WHERE word_id = $1 ORDER BY position`, wordID)
	if err != nil {
		t.Fatalf("positions: %v", err)
	}
	defer rows.Close()
	var out []int16
	for rows.Next() {
		var p int16
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan position: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func findWord(t *testing.T, words []Word, term string) Word {
	t.Helper()
	for _, w := range words {
		if strings.EqualFold(w.Term, term) {
			return w
		}
	}
	t.Fatalf("word %q not found in %+v", term, words)
	return Word{}
}

func exampleTexts(w Word) []string {
	var out []string
	for _, ex := range w.Examples {
		out = append(out, ex.Text)
	}
	return out
}

func groupNames(w Word) []string {
	var out []string
	for _, g := range w.Groups {
		out = append(out, g.Name)
	}
	return out
}

func TestService_Bulk(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	res := env.mustBulk(t, "apple || яблоко || I eat an [apple] <> An apple a day || #food #nouns\n"+
		"go_went_gone || идти || I [go] home <> I [went] home || verbs\n"+
		"broken")
	if res.Created != 2 || res.Merged != 0 || len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Fatalf("first bulk = %+v", res)
	}

	words, err := env.svc.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	apple := findWord(t, words, "apple")
	if apple.Level != 1 || apple.Streak != 0 || !apple.NextReviewAt.Equal(testNow) || !apple.CreatedAt.Equal(testNow) {
		t.Errorf("new word SRS fields: %+v", apple)
	}
	if got := exampleTexts(apple); !reflect.DeepEqual(got, []string{"I eat an [apple]", "An apple a day"}) {
		t.Errorf("apple examples = %q", got)
	}
	if got := apple.Examples[0]; got.Sentence != "I eat an apple" || !reflect.DeepEqual(got.Highlights, []Span{{9, 14}}) {
		t.Errorf("example restored from markup = %+v", got)
	}
	if got := groupNames(apple); !reflect.DeepEqual(got, []string{"food", "nouns"}) {
		t.Errorf("apple groups = %q", got)
	}
	if got := findWord(t, words, "go_went_gone").Forms; !reflect.DeepEqual(got, []string{"go", "went", "gone"}) {
		t.Errorf("forms = %q", got)
	}

	// Второй пакет: дубликат в другом регистре сливается, новый — создаётся.
	env.clock.Advance(time.Hour)
	res = env.mustBulk(t, "APPLE || ЯБЛОКО || An apple a day <> Apples are [red] || #Food #fruits\npear || груша")
	if res.Created != 1 || res.Merged != 1 || len(res.Errors) != 0 {
		t.Fatalf("second bulk = %+v", res)
	}

	words, err = env.svc.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	apple = findWord(t, words, "apple")
	if apple.Term != "apple" || apple.Translation != "яблоко" || !apple.CreatedAt.Equal(testNow) {
		t.Errorf("merged word must keep original term/translation/created_at: %+v", apple)
	}
	if got := exampleTexts(apple); !reflect.DeepEqual(got, []string{"I eat an [apple]", "An apple a day", "Apples are [red]"}) {
		t.Errorf("merged examples = %q", got)
	}
	if got := env.positions(t, apple.ID); !reflect.DeepEqual(got, []int16{0, 1, 2}) {
		t.Errorf("positions = %v, want continuous 0..2", got)
	}
	if got := groupNames(apple); !reflect.DeepEqual(got, []string{"food", "fruits", "nouns"}) {
		t.Errorf("merged groups = %q", got)
	}
	if n := env.count(t, `SELECT count(*) FROM groups`); n != 4 { // food nouns verbs fruits
		t.Errorf("groups count = %d, want 4", n)
	}

	want := []eventbus.Event{
		eventbus.WordsAdded{Created: 2, Merged: 0, At: testNow},
		eventbus.WordsAdded{Created: 1, Merged: 1, At: testNow.Add(time.Hour)},
	}
	if got := env.bus.all(); !reflect.DeepEqual(got, want) {
		t.Errorf("events = %+v, want %+v", got, want)
	}
}

func TestService_Bulk_OnlyErrors(t *testing.T) {
	env := newTestEnv(t)
	res := env.mustBulk(t, "broken\n// comment\n\nalso broken || ")
	if res.Created != 0 || res.Merged != 0 || len(res.Errors) != 2 {
		t.Errorf("result = %+v", res)
	}
	if n := env.count(t, `SELECT count(*) FROM words`); n != 0 {
		t.Errorf("words count = %d", n)
	}
	if got := env.bus.all(); len(got) != 0 {
		t.Errorf("no event expected, got %+v", got)
	}
}

func TestService_List_Filters(t *testing.T) {
	env := newTestEnv(t)
	env.mustBulk(t, "a || 1 || || g1\nb || 2 || || g1 g2\nc || 3")
	env.exec(t, `UPDATE words SET level = 3 WHERE term = 'c'`)
	var g1, g2 int64
	if err := env.pool.QueryRow(context.Background(),
		`SELECT (SELECT id FROM groups WHERE name = 'g1'), (SELECT id FROM groups WHERE name = 'g2')`).Scan(&g1, &g2); err != nil {
		t.Fatalf("group ids: %v", err)
	}
	lvl := func(l int16) *int16 { return &l }
	grp := func(id int64) *int64 { return &id }

	tests := []struct {
		name   string
		filter ListFilter
		want   []string
	}{
		{"no filter", ListFilter{}, []string{"a", "b", "c"}},
		{"group g1", ListFilter{GroupID: grp(g1)}, []string{"a", "b"}},
		{"group g2", ListFilter{GroupID: grp(g2)}, []string{"b"}},
		{"level 3", ListFilter{Level: lvl(3)}, []string{"c"}},
		{"level 1 and g1", ListFilter{Level: lvl(1), GroupID: grp(g1)}, []string{"a", "b"}},
		{"level 3 and g1", ListFilter{Level: lvl(3), GroupID: grp(g1)}, nil},
		{"unknown group", ListFilter{GroupID: grp(999999)}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words, err := env.svc.List(context.Background(), tt.filter)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if words == nil {
				t.Fatal("List must return non-nil slice")
			}
			var got []string
			for _, w := range words {
				got = append(got, w.Term)
			}
			slices.Sort(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.mustBulk(t, "run || бежать || I [run] || sport\nwalk || гулять")
	id := env.wordID(t, "run")
	env.exec(t, `UPDATE words SET level = 3, streak = 2 WHERE id = $1`, id)

	str := func(s string) *string { return &s }
	strs := func(s ...string) *[]string { return &s }

	w, err := env.svc.Update(ctx, id, UpdateParams{
		Term:     str("run _ ran _ run"),
		Examples: strs("I [ ran ] fast", "I [run]"),
		Groups:   strs("#Verbs", "sport"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if w.Term != "run_ran_run" || !reflect.DeepEqual(w.Forms, []string{"run", "ran", "run"}) || w.Translation != "бежать" {
		t.Errorf("text fields: %+v", w)
	}
	if w.Level != 3 || w.Streak != 2 {
		t.Errorf("SRS must be untouched: level %d streak %d", w.Level, w.Streak)
	}
	if got := exampleTexts(w); !reflect.DeepEqual(got, []string{"I [ran] fast", "I [run]"}) {
		t.Errorf("examples = %q", got)
	}
	if got := env.positions(t, id); !reflect.DeepEqual(got, []int16{0, 1}) {
		t.Errorf("positions = %v", got)
	}
	if got := groupNames(w); !reflect.DeepEqual(got, []string{"sport", "verbs"}) {
		t.Errorf("groups = %q", got)
	}

	// Частичное обновление не трогает остальные поля; forms сохраняются.
	w, err = env.svc.Update(ctx, id, UpdateParams{Translation: str("бегать")})
	if err != nil {
		t.Fatalf("Update translation: %v", err)
	}
	if w.Translation != "бегать" || w.Term != "run_ran_run" || len(w.Forms) != 3 || len(w.Examples) != 2 || len(w.Groups) != 2 {
		t.Errorf("partial update: %+v", w)
	}

	// Пустые списки очищают.
	w, err = env.svc.Update(ctx, id, UpdateParams{Examples: strs(), Groups: strs()})
	if err != nil {
		t.Fatalf("Update clear: %v", err)
	}
	if len(w.Examples) != 0 || len(w.Groups) != 0 {
		t.Errorf("want cleared, got %+v", w)
	}

	// Обычный term без "_" сбрасывает формы.
	w, err = env.svc.Update(ctx, id, UpdateParams{Term: str("run")})
	if err != nil {
		t.Fatalf("Update term: %v", err)
	}
	if w.Term != "run" || len(w.Forms) != 0 {
		t.Errorf("forms must be cleared: %+v", w)
	}

	t.Run("errors", func(t *testing.T) {
		tests := []struct {
			name   string
			id     int64
			p      UpdateParams
			status int
			code   string
		}{
			{"not found", 999999, UpdateParams{Translation: str("x")}, 404, "not_found"},
			{"duplicate case-insensitive", id, UpdateParams{Term: str("WALK"), Translation: str("Гулять")}, 409, "duplicate_word"},
			{"bad example", id, UpdateParams{Examples: strs("I [run")}, 400, "unbalanced_brackets"},
			{"bad forms", id, UpdateParams{Term: str("a_b")}, 400, "invalid_verb_forms"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := env.svc.Update(ctx, tt.id, tt.p)
				assertHTTPError(t, err, tt.status, tt.code)
			})
		}
		// После неудачного дубликата слово не изменилось (транзакция откатилась).
		w, err := env.svc.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if w.Term != "run" || w.Translation != "бегать" {
			t.Errorf("word changed after failed update: %+v", w)
		}
	})
}

func TestService_Delete(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.mustBulk(t, "a || 1 || [x] y || g")
	id := env.wordID(t, "a")

	if err := env.svc.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := env.svc.Get(ctx, id)
	assertHTTPError(t, err, 404, "not_found")
	if n := env.count(t, `SELECT count(*) FROM word_examples`); n != 0 {
		t.Errorf("examples must cascade, left %d", n)
	}
	if n := env.count(t, `SELECT count(*) FROM groups`); n != 1 {
		t.Errorf("group must survive word deletion, count %d", n)
	}
	assertHTTPError(t, env.svc.Delete(ctx, id), 404, "not_found")
}

func TestService_Unarchive(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	env.mustBulk(t, "a || 1\nb || 2")
	a, b := env.wordID(t, "a"), env.wordID(t, "b")
	env.exec(t, `UPDATE words SET level = 5, streak = 2, next_review_at = $2 WHERE id = $1`, a, testNow.AddDate(0, 1, 0))

	env.clock.Advance(48 * time.Hour)
	w, err := env.svc.Unarchive(ctx, a)
	if err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	if w.Level != 4 || w.Streak != 0 || !w.NextReviewAt.Equal(env.clock.Now()) {
		t.Errorf("unarchived = level %d streak %d next %v, want 4 0 %v", w.Level, w.Streak, w.NextReviewAt, env.clock.Now())
	}

	_, err = env.svc.Unarchive(ctx, a)
	assertHTTPError(t, err, 409, "not_archived")
	_, err = env.svc.Unarchive(ctx, b)
	assertHTTPError(t, err, 409, "not_archived")
	_, err = env.svc.Unarchive(ctx, 999999)
	assertHTTPError(t, err, 404, "not_found")
}

// TestModule_HTTP — сквозная проверка роутов и JSON поверх живой БД.
func TestModule_HTTP(t *testing.T) {
	env := newTestEnv(t)
	r := chi.NewRouter()
	r.Route("/api/words", (&Module{svc: env.svc}).Routes)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rr
	}

	rr := do(http.MethodPost, "/api/words/bulk", `{"text":"apple || яблоко || I eat an [apple] || food\nbroken"}`)
	if rr.Code != 200 {
		t.Fatalf("bulk: %d %s", rr.Code, rr.Body)
	}
	var bulk BulkResult
	if err := json.NewDecoder(rr.Body).Decode(&bulk); err != nil || bulk.Created != 1 || len(bulk.Errors) != 1 {
		t.Fatalf("bulk body = %+v (err %v)", bulk, err)
	}

	// Без завершающего слэша — так будет звать фронт.
	rr = do(http.MethodGet, "/api/words?level=1", "")
	if rr.Code != 200 {
		t.Fatalf("list: %d %s", rr.Code, rr.Body)
	}
	var list struct{ Words []Word }
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil || len(list.Words) != 1 {
		t.Fatalf("list body = %+v (err %v)", list, err)
	}
	id := list.Words[0].ID
	if hl := list.Words[0].Examples[0].Highlights; !reflect.DeepEqual(hl, []Span{{9, 14}}) {
		t.Errorf("highlights in JSON = %+v", hl)
	}

	path := fmt.Sprintf("/api/words/%d", id)
	if rr = do(http.MethodPatch, path, `{"translation":"яблочко"}`); rr.Code != 200 || !strings.Contains(rr.Body.String(), "яблочко") {
		t.Errorf("patch: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodPost, path+"/unarchive", ""); rr.Code != 409 {
		t.Errorf("unarchive non-archived: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, path, ""); rr.Code != 204 {
		t.Errorf("delete: %d %s", rr.Code, rr.Body)
	}
	if rr = do(http.MethodDelete, path, ""); rr.Code != 404 {
		t.Errorf("delete again: %d %s", rr.Code, rr.Body)
	}
}
