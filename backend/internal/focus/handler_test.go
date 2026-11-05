package focus

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
)

// Тесты без БД: валидация срабатывает раньше запросов (пул nil — до него не доходим).
func TestHandler_BadRequests(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/focus", New(nil, clock.NewFixed(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)), nil, nil, nil).Routes)
	tests := []struct {
		name, method, path, body, wantCode string
	}{
		{"start no task", http.MethodPost, "/api/focus/sessions", `{"taskId":0}`, "invalid_task"},
		{"start unknown field", http.MethodPost, "/api/focus/sessions", `{"taskId":1,"x":1}`, "invalid_json"},
		{"bad session id", http.MethodPost, "/api/focus/sessions/abc/work", "", "invalid_id"},
		{"finish bad outcome", http.MethodPost, "/api/focus/sessions/1/finish", `{"outcome":"maybe"}`, "invalid_outcome"},
		{"distraction empty", http.MethodPost, "/api/focus/sessions/1/distractions", `{"trigger":" ","gaveIn":true}`, "empty_text"},
		{"ritual too long", http.MethodPut, "/api/focus/ritual", `{"items":["` + strings.Repeat("я", 101) + `"]}`, "text_too_long"},
		{"reward no title", http.MethodPost, "/api/focus/rewards", `{"title":"","price":5}`, "invalid_title"},
		{"reward bad price", http.MethodPost, "/api/focus/rewards", `{"title":"Кино","price":0}`, "invalid_price"},
		{"points bad cursor", http.MethodGet, "/api/focus/points?before=x", "", "invalid_cursor"},
		{"stats bad range", http.MethodGet, "/api/focus/stats?from=2026-11-05&to=2026-11-01", "", "invalid_range"},
		{"stats bad date", http.MethodGet, "/api/focus/stats?from=вчера", "", "invalid_date"},
		{"points bad limit", http.MethodGet, "/api/focus/points?limit=500", "", "invalid_limit"},
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
				t.Fatal(err)
			}
			if body.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}
