//go:build integration

// Интеграционные тесты importexport на живом Postgres (запуск — SPEC §15).
package importexport

import (
	"context"
	"encoding/json"
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
	"lexis/internal/words"
)

type nopBus struct{}

func (nopBus) Publish(eventbus.Event) {}

func newTestEnv(t *testing.T) (*Service, *words.Service, dbtest.Fixtures) {
	t.Helper()
	pool := dbtest.New(t)
	clk := clock.NewFixed(testDay)
	ws := words.NewService(pool, clk, nopBus{})
	return NewService(pool, clk, ws), ws, dbtest.Fixtures{Pool: pool, Now: testDay}
}

func mustImport(t *testing.T, svc *Service, data string) ImportResult {
	t.Helper()
	res, err := svc.Import(context.Background(), []byte(data))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return res
}

func TestImport_CSV(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()

	res := mustImport(t, svc, "term,translation,example,tags\n"+
		"apple,яблоко,I eat an [apple] <> An apple a day,food;nouns\n"+
		",пусто,,\n"+
		"go_went_gone,идти,I [went] home,verbs\n")
	if res.Kind != "words" || res.Created != 2 || res.Merged != 0 || len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Fatalf("result = %+v", res)
	}

	// Повторный импорт с тем же словом в другом регистре сливается, как bulk.
	res = mustImport(t, svc, "term,translation,tags\nAPPLE,ЯБЛОКО,fruits\n")
	if res.Created != 0 || res.Merged != 1 {
		t.Errorf("merge = %+v", res)
	}

	list, err := ws.List(ctx, words.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var apple words.Word
	for _, w := range list {
		if w.Term == "apple" {
			apple = w
		}
	}
	var groups []string
	for _, g := range apple.Groups {
		groups = append(groups, g.Name)
	}
	if len(apple.Examples) != 2 || !reflect.DeepEqual(groups, []string{"food", "fruits", "nouns"}) {
		t.Errorf("apple = %+v", apple)
	}
	if n := fx.Int(t, `SELECT count(*) FROM words WHERE cardinality(forms) = 3`); n != 1 {
		t.Errorf("verb forms not imported")
	}
}

func TestImport_JSONList(t *testing.T) {
	svc, _, fx := newTestEnv(t)
	res := mustImport(t, svc, `[{"term":"a","translation":"1","tags":"g"},{"term":"b","translation":"2"},{"term":"","translation":"x"}]`)
	if res.Kind != "words" || res.Created != 2 || len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Errorf("result = %+v", res)
	}
	if n := fx.Int(t, `SELECT count(*) FROM word_groups`); n != 1 {
		t.Errorf("word_groups = %d", n)
	}
}

// seed — небольшая, но полная база: группы, слова с примерами и формами, сессии с ответами,
// удалённая группа у сессии, ачивка, настройки.
func seed(t *testing.T, ws *words.Service, fx dbtest.Fixtures) {
	t.Helper()
	ctx := context.Background()
	if _, err := ws.Bulk(ctx, "apple || яблоко || I eat an [apple] <> Red [apple] || food nouns\n"+
		"go_went_gone || идти || I [go] <> I [went] || verbs\n"+
		"pear || груша || || food"); err != nil {
		t.Fatalf("Bulk: %v", err)
	}
	fx.Exec(t, `UPDATE words SET level = 4, streak = 1, last_good_at = $1, last_reviewed_at = $1 WHERE term = 'apple'`, testDay.Add(-time.Hour))
	food := fx.Int(t, `SELECT id FROM groups WHERE name = 'food'`)
	apple := fx.Int(t, `SELECT id FROM words WHERE term = 'apple'`)
	pear := fx.Int(t, `SELECT id FROM words WHERE term = 'pear'`)

	fin := testDay.Add(-30 * time.Minute)
	s1 := fx.Session(t, dbtest.Session{GroupID: &food, FinishedAt: &fin, Total: 2, Good: 1, Again: 1, DurationSec: 60})
	fx.Review(t, s1, apple, "good", true, fin)
	fx.Review(t, s1, pear, "again", true, fin)
	fx.Review(t, s1, pear, "good", false, fin)
	fx.Session(t, dbtest.Session{Free: true}) // незавершённая, без группы

	fx.Exec(t, `INSERT INTO achievements (code, unlocked_at) VALUES ('first_session', $1)`, fin)
	fx.Exec(t, `INSERT INTO settings (key, value) VALUES ('language', '"en"'), ('new_words_limit', '7')`)
	// Дыра в id: удалённое слово — восстановление не должно зависеть от непрерывности id.
	fx.Exec(t, `DELETE FROM words WHERE term = 'go_went_gone'`)
	if _, err := ws.Bulk(ctx, "go_went_gone || идти || I [go] <> I [went] || verbs"); err != nil {
		t.Fatalf("Bulk: %v", err)
	}
}

// canonical заменяет id порядковыми номерами (ссылки — тоже), чтобы сравнивать снимки
// до и после восстановления: новые id не обязаны совпадать со старыми.
func canonical(t *testing.T, b Backup) string {
	t.Helper()
	b.ExportedAt = time.Time{}
	gIdx, wIdx := map[int64]int64{}, map[int64]int64{}
	for i := range b.Groups {
		gIdx[b.Groups[i].ID] = int64(i)
		b.Groups[i].ID = int64(i)
	}
	for i := range b.Words {
		wIdx[b.Words[i].ID] = int64(i)
		b.Words[i].ID = int64(i)
		for j, g := range b.Words[i].GroupIDs {
			b.Words[i].GroupIDs[j] = gIdx[g]
		}
	}
	for i := range b.Sessions {
		b.Sessions[i].ID = int64(i)
		if g := b.Sessions[i].GroupID; g != nil {
			v := gIdx[*g]
			b.Sessions[i].GroupID = &v
		}
		for j := range b.Sessions[i].Reviews {
			b.Sessions[i].Reviews[j].WordID = wIdx[b.Sessions[i].Reviews[j].WordID]
		}
	}
	raw, err := json.MarshalIndent(b, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestBackup_RoundTrip(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()
	seed(t, ws, fx)

	b1, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatalf("ExportBackup: %v", err)
	}
	if len(b1.Words) != 3 || len(b1.Groups) != 3 || len(b1.Sessions) != 2 || len(b1.Achievements) != 1 || len(b1.Settings) != 2 {
		t.Fatalf("snapshot = %d words %d groups %d sessions %d ach %d settings",
			len(b1.Words), len(b1.Groups), len(b1.Sessions), len(b1.Achievements), len(b1.Settings))
	}

	// Меняем базу, потом восстанавливаем через Import (как с фронта: JSON-файл).
	if _, err := ws.Bulk(ctx, "extra || лишнее || || junk"); err != nil {
		t.Fatal(err)
	}
	fx.Exec(t, `UPDATE settings SET value = '"ru"' WHERE key = 'language'`)
	raw, _ := json.Marshal(b1)
	res := mustImport(t, svc, string(raw))
	if res.Kind != "backup" || res.Restored == nil || *res.Restored != (RestoreCounts{Groups: 3, Words: 3, Sessions: 2, Reviews: 3, Achievements: 1}) {
		t.Fatalf("restore result = %+v", res)
	}

	b2, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatalf("ExportBackup after restore: %v", err)
	}
	if c1, c2 := canonical(t, b1), canonical(t, b2); c1 != c2 {
		t.Errorf("round trip differs:\n--- before\n%s\n--- after\n%s", c1, c2)
	}
	// После восстановления обычная работа продолжается: новые id не конфликтуют.
	if _, err := ws.Bulk(ctx, "new || новое || || food"); err != nil {
		t.Errorf("Bulk after restore: %v", err)
	}
}

func TestRestore_InvalidRollsBack(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()
	seed(t, ws, fx)
	b, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Ссылки целы, но уровень 9 нарушает CHECK — падает посреди транзакции.
	b.Words[len(b.Words)-1].Level = 9
	b.Words[0].Term = "changed"
	_, err = svc.Restore(ctx, b)
	assertCode(t, err, "invalid_backup")

	if n := fx.Int(t, `SELECT count(*) FROM words WHERE term = 'apple'`); n != 1 {
		t.Errorf("data must be untouched after failed restore")
	}
	if n := fx.Int(t, `SELECT count(*) FROM settings`); n != 2 {
		t.Errorf("settings must be untouched, got %d", n)
	}
}

func TestReset(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	seed(t, ws, fx)
	if err := svc.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for _, table := range []string{"words", "word_examples", "groups", "word_groups", "training_sessions", "reviews", "achievements"} {
		if n := fx.Int(t, `SELECT count(*) FROM `+table); n != 0 {
			t.Errorf("%s not empty: %d", table, n)
		}
	}
	if n := fx.Int(t, `SELECT count(*) FROM settings`); n != 2 {
		t.Errorf("settings must survive reset, got %d", n)
	}
	if id := fx.Group(t, "fresh"); id != 1 {
		t.Errorf("ids must restart, got %d", id)
	}
}

func TestModule_HTTP(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	seed(t, ws, fx)
	r := chi.NewRouter()
	r.Route("/api/data", (&Module{svc: svc}).Routes)
	do := func(method, path string, body *strings.Reader, ct string) *httptest.ResponseRecorder {
		hr := httptest.NewRequest(method, path, body)
		if ct != "" {
			hr.Header.Set("Content-Type", ct)
		}
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, hr)
		return rr
	}

	rr := do(http.MethodGet, "/api/data/export?format=csv", strings.NewReader(""), "")
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(rr.Header().Get("Content-Disposition"), "lexis-words-2026-11-02.csv") ||
		!strings.Contains(rr.Body.String(), "apple,яблоко,I eat an [apple] <> Red [apple],food nouns") {
		t.Errorf("export csv: %d %v\n%s", rr.Code, rr.Header(), rr.Body)
	}

	rr = do(http.MethodGet, "/api/data/export", strings.NewReader(""), "")
	if rr.Code != 200 || !strings.Contains(rr.Header().Get("Content-Disposition"), "lexis-backup-2026-11-02.json") {
		t.Fatalf("export json: %d %v", rr.Code, rr.Header())
	}
	backup := rr.Body.String()

	if rr = do(http.MethodPost, "/api/data/reset", strings.NewReader(`{"confirm":true}`), "application/json"); rr.Code != 204 {
		t.Fatalf("reset: %d %s", rr.Code, rr.Body)
	}

	body, ct := multipartBody(t, "file", backup)
	rr = do(http.MethodPost, "/api/data/import", strings.NewReader(body.String()), ct)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"kind":"backup"`) {
		t.Errorf("import backup: %d %s", rr.Code, rr.Body)
	}
	if n := fx.Int(t, `SELECT count(*) FROM words`); n != 3 {
		t.Errorf("words after HTTP restore = %d", n)
	}
}
