package journal

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
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	// 00:30 МСК 3-го = 21:30 UTC 2-го: «сегодня» должно быть 3-е, а 4-е — будущее.
	r := chi.NewRouter()
	r.Route("/api/journal", New(nil, clock.NewFixed(time.Date(2026, 11, 3, 0, 30, 0, 0, msk)), nil).Routes)

	tests := []struct {
		name, method, path, body, wantCode string
	}{
		{"get bad day", http.MethodGet, "/api/journal/days/03.11.2026", "", "invalid_date"},
		{"get impossible day", http.MethodGet, "/api/journal/days/2026-02-30", "", "invalid_date"},
		{"list bad from", http.MethodGet, "/api/journal/days?from=x", "", "invalid_date"},
		{"list inverted", http.MethodGet, "/api/journal/days?from=2026-11-04&to=2026-11-03", "", "invalid_range"},
		{"list too long", http.MethodGet, "/api/journal/days?from=2024-01-01&to=2026-11-03", "", "invalid_range"},
		{"patch future", http.MethodPatch, "/api/journal/days/2026-11-04", `{"rating": 5}`, "future_day"},
		{"patch bad rating", http.MethodPatch, "/api/journal/days/2026-11-03", `{"rating": 11}`, "invalid_rating"},
		{"patch unknown field", http.MethodPatch, "/api/journal/days/2026-11-03", `{"mood": 5}`, "invalid_json"},
		{"dopamine slot not a number", http.MethodPut, "/api/journal/days/2026-11-03/dopamine/x", `{"text": "a"}`, "invalid_slot"},
		{"dopamine slot 7", http.MethodPut, "/api/journal/days/2026-11-03/dopamine/7", `{"text": "a"}`, "invalid_slot"},
		{"dopamine future", http.MethodPut, "/api/journal/days/2026-11-04/dopamine/0", `{"text": "a"}`, "future_day"},
		{"week bad date", http.MethodGet, "/api/journal/weeks/week45", "", "invalid_date"},
		{"week not monday", http.MethodGet, "/api/journal/weeks/2026-11-03", "", "not_monday"},
		{"save future week", http.MethodPut, "/api/journal/weeks/2026-11-09", `{"blocks":[]}`, "future_day"},
		{"save week too long", http.MethodPut, "/api/journal/weeks/2026-11-02", `{"blocks":[{"topic":"` + strings.Repeat("a", 201) + `","conclusion":""}]}`, "text_too_long"},
		{"save week unknown field", http.MethodPut, "/api/journal/weeks/2026-11-02", `{"blocks":[{"title":"x"}]}`, "invalid_json"},
		{"dopamine too long", http.MethodPut, "/api/journal/days/2026-11-03/dopamine/0", `{"text": "` + strings.Repeat("a", 141) + `"}`, "text_too_long"},
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
