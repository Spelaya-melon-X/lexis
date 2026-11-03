package stats

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
)

// Тесты без БД: валидация параметров срабатывает раньше запросов.
func TestHandler_BadRequests(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/stats", New(nil, clock.NewFixed(at(10, 12, 0))).Routes)

	tests := []struct {
		name, path, wantCode string
	}{
		{"overview bad day", "/api/stats/overview?day=10.11.2026", "invalid_date"},
		{"overview impossible day", "/api/stats/overview?day=2026-02-30", "invalid_date"},
		{"calendar bad from", "/api/stats/calendar?from=yesterday", "invalid_date"},
		{"calendar bad to", "/api/stats/calendar?to=2026-13-01", "invalid_date"},
		{"calendar from after to", "/api/stats/calendar?from=2026-11-10&to=2026-11-09", "invalid_range"},
		{"calendar too long", "/api/stats/calendar?from=2025-01-01&to=2026-11-10", "invalid_range"},
		{"trends bad range", "/api/stats/trends?range=year", "invalid_range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tt.path, nil))
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
