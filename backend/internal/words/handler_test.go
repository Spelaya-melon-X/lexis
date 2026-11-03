package words

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/httpx"
)

// Тесты без БД: только пути, которые не доходят до сервиса (парсер и валидация запроса).
// Остальное — в service_integration_test.go.
func newTestRouter() chi.Router {
	r := chi.NewRouter()
	r.Route("/api/words", (&Module{}).Routes)
	return r
}

func TestHandler_Parse(t *testing.T) {
	rr := httptest.NewRecorder()
	body := `{"text":"apple || яблоко || I eat an [apple]\nbroken"}`
	newTestRouter().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/words/parse", strings.NewReader(body)))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body)
	}
	var got ParseResult
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Cards) != 1 || got.Cards[0].Term != "apple" || len(got.Cards[0].Examples[0].Highlights) != 1 {
		t.Errorf("cards = %+v", got.Cards)
	}
	if len(got.Errors) != 1 || got.Errors[0].Code != CodeMissingTranslation || got.Errors[0].Line != 2 {
		t.Errorf("errors = %+v", got.Errors)
	}
}

func TestHandler_BadRequests(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		wantCode                 string
	}{
		{"parse unknown field", http.MethodPost, "/api/words/parse", `{"txt":"a || b"}`, "invalid_json"},
		{"bulk malformed json", http.MethodPost, "/api/words/bulk", `{"text":`, "invalid_json"},
		{"list level not a number", http.MethodGet, "/api/words/?level=x", "", "invalid_level"},
		{"list level out of range", http.MethodGet, "/api/words/?level=6", "", "invalid_level"},
		{"list level zero", http.MethodGet, "/api/words/?level=0", "", "invalid_level"},
		{"list group not a number", http.MethodGet, "/api/words/?group=food", "", "invalid_group"},
		{"list group negative", http.MethodGet, "/api/words/?group=-1", "", "invalid_group"},
		{"patch bad id", http.MethodPatch, "/api/words/abc", `{}`, "invalid_id"},
		{"delete zero id", http.MethodDelete, "/api/words/0", "", "invalid_id"},
		{"unarchive bad id", http.MethodPost, "/api/words/1x/unarchive", "", "invalid_id"},
		{"patch unknown field", http.MethodPatch, "/api/words/1", `{"level":5}`, "invalid_json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			newTestRouter().ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rr.Code, rr.Body)
			}
			var body struct {
				Error struct{ Code string } `json:"error"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	str := func(s string) *string { return &s }
	strs := func(s ...string) *[]string { return &s }

	tests := []struct {
		name     string
		p        UpdateParams
		wantCode string // "" — ошибки нет
		check    func(t *testing.T, v validatedUpdate)
	}{
		{"nothing to change", UpdateParams{}, "", func(t *testing.T, v validatedUpdate) {
			if v.term != nil || v.translation != nil || v.examples != nil || v.groups != nil {
				t.Errorf("want all nil, got %+v", v)
			}
		}},
		{"term normalized into forms", UpdateParams{Term: str(" go _ went_gone ")}, "", func(t *testing.T, v validatedUpdate) {
			if *v.term != "go_went_gone" || !equalOrEmpty(v.forms, []string{"go", "went", "gone"}) {
				t.Errorf("term %q forms %q", *v.term, v.forms)
			}
		}},
		{"plain term has no forms", UpdateParams{Term: str("well-known")}, "", func(t *testing.T, v validatedUpdate) {
			if *v.term != "well-known" || v.forms != nil {
				t.Errorf("term %q forms %q", *v.term, v.forms)
			}
		}},
		{"empty term", UpdateParams{Term: str("  ")}, "empty_term", nil},
		{"bad forms", UpdateParams{Term: str("go_went")}, "invalid_verb_forms", nil},
		{"empty translation", UpdateParams{Translation: str("")}, "empty_translation", nil},
		{"examples normalized, blanks and dups dropped", UpdateParams{Examples: strs("I [ go ]", " ", "I [go]", "x")}, "", func(t *testing.T, v validatedUpdate) {
			if len(v.examples) != 2 || v.examples[0].Text != "I [go]" || v.examples[1].Text != "x" {
				t.Errorf("examples %+v", v.examples)
			}
		}},
		{"empty examples list clears", UpdateParams{Examples: strs()}, "", func(t *testing.T, v validatedUpdate) {
			if v.examples == nil || len(v.examples) != 0 {
				t.Errorf("want empty non-nil, got %#v", v.examples)
			}
		}},
		{"bad example", UpdateParams{Examples: strs("ok", "I [go")}, "unbalanced_brackets", nil},
		{"groups normalized like tags", UpdateParams{Groups: strs("#Food", "food", "verbs, nouns")}, "", func(t *testing.T, v validatedUpdate) {
			if !equalOrEmpty(v.groups, []string{"food", "verbs", "nouns"}) {
				t.Errorf("groups %q", v.groups)
			}
		}},
		{"empty groups list clears", UpdateParams{Groups: strs("#")}, "", func(t *testing.T, v validatedUpdate) {
			if v.groups == nil || len(v.groups) != 0 {
				t.Errorf("want empty non-nil, got %#v", v.groups)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := validateUpdate(tt.p)
			if tt.wantCode != "" {
				assertHTTPError(t, err, http.StatusBadRequest, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, v)
		})
	}
}

// assertHTTPError проверяет, что err — *httpx.Error с нужным статусом и кодом (в т.ч. обёрнутая).
func assertHTTPError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) {
		t.Fatalf("want *httpx.Error %d %q, got %v", status, code, err)
	}
	if he.Status != status || he.Code != code {
		t.Errorf("got %d %q, want %d %q", he.Status, he.Code, status, code)
	}
}
