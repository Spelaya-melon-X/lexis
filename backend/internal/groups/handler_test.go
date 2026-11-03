package groups

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

// Тесты без БД: валидация срабатывает раньше, чем сервис идёт в базу.
func TestHandler_BadRequests(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/groups", New(nil, clock.NewFixed(at(2, 12, 0)), nil).Routes)

	tests := []struct {
		name, method, path, body string
		wantCode                 string
	}{
		{"get bad id", http.MethodGet, "/api/groups/abc", "", "invalid_id"},
		{"get bad range", http.MethodGet, "/api/groups/1?range=year", "", "invalid_range"},
		{"patch zero id", http.MethodPatch, "/api/groups/0", `{}`, "invalid_id"},
		{"patch unknown field", http.MethodPatch, "/api/groups/1", `{"title":"x"}`, "invalid_json"},
		{"patch color not a string", http.MethodPatch, "/api/groups/1", `{"color":123}`, "invalid_json"},
		{"patch empty name", http.MethodPatch, "/api/groups/1", `{"name":" # "}`, "invalid_group_name"},
		{"patch two tags as name", http.MethodPatch, "/api/groups/1", `{"name":"food verbs"}`, "invalid_group_name"},
		{"patch bad color", http.MethodPatch, "/api/groups/1", `{"color":"red"}`, "invalid_color"},
		{"patch short color", http.MethodPatch, "/api/groups/1", `{"color":"#fff"}`, "invalid_color"},
		{"delete bad id", http.MethodDelete, "/api/groups/-5", "", "invalid_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
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

func TestOptionalString(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantSet bool
		wantVal *string
	}{
		{"absent — don't touch", `{}`, false, nil},
		{"null — clear", `{"color":null}`, true, nil},
		{"value — set", `{"color":"#00ff00"}`, true, ptr("#00ff00")},
		{"empty string is a value", `{"color":""}`, true, ptr("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p UpdateParams
			if err := json.Unmarshal([]byte(tt.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if p.Color.Set != tt.wantSet || (p.Color.Value == nil) != (tt.wantVal == nil) ||
				(tt.wantVal != nil && *p.Color.Value != *tt.wantVal) {
				t.Errorf("got %+v (value %v)", p.Color, p.Color.Value)
			}
		})
	}
}

func ptr(s string) *string { return &s }

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
