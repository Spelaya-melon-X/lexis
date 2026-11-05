//go:build integration

// Теги с путём (§18.2): #путешествия/аэропорт создаёт дерево групп при вводе.
package words

import (
	"context"
	"reflect"
	"testing"
)

// parentName — имя родителя группы ("" — корень, "-" — группы нет).
func (e testEnv) parentName(t *testing.T, name string) string {
	t.Helper()
	var parent *string
	err := e.pool.QueryRow(context.Background(),
		`SELECT p.name FROM groups g LEFT JOIN groups p ON p.id = g.parent_id WHERE g.name = $1`, name).Scan(&parent)
	if err != nil {
		return "-"
	}
	if parent == nil {
		return ""
	}
	return *parent
}

func TestBulk_GroupPaths(t *testing.T) {
	env := newTestEnv(t)

	res := env.mustBulk(t, "plane || самолёт || || #Travel/Airport\ntaxi || такси || || #airport #travel/hotel")
	if res.Created != 2 || len(res.Errors) != 0 {
		t.Fatalf("bulk: %+v", res)
	}
	for name, want := range map[string]string{"travel": "", "airport": "travel", "hotel": "travel"} {
		if got := env.parentName(t, name); got != want {
			t.Errorf("parent of %s = %q, want %q", name, got, want)
		}
	}
	// Слово привязано только к листу пути, не к предкам.
	words, err := env.svc.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got := groupNames(findWord(t, words, "plane")); !reflect.DeepEqual(got, []string{"airport"}) {
		t.Errorf("plane groups = %v", got)
	}
	if got := groupNames(findWord(t, words, "taxi")); !reflect.DeepEqual(got, []string{"airport", "hotel"}) {
		t.Errorf("taxi groups = %v", got)
	}

	// Ошибки сохранения и разбора — одним списком по строкам; строка с ошибкой не пишет ни слово, ни группы.
	res = env.mustBulk(t, "ok || да || || #airport/gates\n"+
		"bad || плохо || || #hotel/airport #newgroup\n"+
		"deep || глубоко || || #gates/door\n"+
		"broken || сломано || || #x//y")
	want := []LineError{
		{Line: 2, Text: "bad || плохо || || #hotel/airport #newgroup", Code: CodeGroupPathConflict},
		{Line: 3, Text: "deep || глубоко || || #gates/door", Code: CodeGroupTooDeep},
		{Line: 4, Text: "broken || сломано || || #x//y", Code: CodeInvalidGroupPath},
	}
	if res.Created != 1 || !reflect.DeepEqual(res.Errors, want) {
		t.Errorf("bulk with errors: created=%d errors=%+v", res.Created, res.Errors)
	}
	if n := env.count(t, `SELECT count(*) FROM words WHERE term IN ('bad', 'deep')`); n != 0 {
		t.Errorf("words from failed lines saved: %d", n)
	}
	if n := env.count(t, `SELECT count(*) FROM groups WHERE name IN ('newgroup', 'door')`); n != 0 {
		t.Errorf("groups from failed lines created: %d", n)
	}
	if got := env.parentName(t, "gates"); got != "airport" {
		t.Errorf("gates parent = %q", got)
	}
}

func TestUpdate_GroupPaths(t *testing.T) {
	env := newTestEnv(t)
	env.mustBulk(t, "plane || самолёт || || #travel/airport\nbed || кровать || || #hotel")
	id := env.wordID(t, "plane")

	_, err := env.svc.Update(context.Background(), id, UpdateParams{Groups: &[]string{"#travel/hotel"}})
	assertHTTPError(t, err, 400, string(CodeGroupPathConflict)) // hotel уже в корне
	_, err = env.svc.Update(context.Background(), id, UpdateParams{Groups: &[]string{"a//b"}})
	assertHTTPError(t, err, 400, string(CodeInvalidGroupPath))

	w, err := env.svc.Update(context.Background(), id, UpdateParams{Groups: &[]string{"travel/beach", "hotel"}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := groupNames(w); !reflect.DeepEqual(got, []string{"beach", "hotel"}) {
		t.Errorf("groups = %v", got)
	}
	if got := env.parentName(t, "beach"); got != "travel" {
		t.Errorf("beach parent = %q", got)
	}
}
