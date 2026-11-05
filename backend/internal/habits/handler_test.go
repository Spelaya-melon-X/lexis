package habits

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
	r := chi.NewRouter()
	r.Route("/api/habits", New(nil, clock.NewFixed(time.Date(2026, 11, 3, 0, 30, 0, 0, msk))).Routes)

	tests := []struct {
		name, method, path, body, wantCode string
	}{
		{"create empty name", http.MethodPost, "/api/habits", `{"name":" ","kind":"positive"}`, "invalid_habit_name"},
		{"create bad kind", http.MethodPost, "/api/habits", `{"name":"x","kind":"meh"}`, "invalid_kind"},
		{"create bad color", http.MethodPost, "/api/habits", `{"name":"x","kind":"positive","color":"red"}`, "invalid_color"},
		{"create unknown field", http.MethodPost, "/api/habits", `{"name":"x","kind":"positive","goal":1}`, "invalid_json"},
		{"patch bad id", http.MethodPatch, "/api/habits/abc", `{}`, "invalid_id"},
		{"patch kind is not editable", http.MethodPatch, "/api/habits/1", `{"kind":"negative"}`, "invalid_json"},
		{"stats bad range", http.MethodGet, "/api/habits/stats?from=2026-11-05&to=2026-11-01", "", "invalid_range"},
		{"checkins bad range", http.MethodGet, "/api/habits/checkins?from=2026-11-05&to=2026-11-01", "", "invalid_range"},
		{"save future day", http.MethodPut, "/api/habits/checkins/2026-11-04", `{"checkins":[]}`, "future_day"},
		{"save bad day", http.MethodPut, "/api/habits/checkins/today", `{"checkins":[]}`, "invalid_date"},
		{"save duplicate habit", http.MethodPut, "/api/habits/checkins/2026-11-03",
			`{"checkins":[{"habitId":1,"done":true},{"habitId":1,"done":false}]}`, "invalid_habit"},
		{"goal no title", http.MethodPost, "/api/habits/goals", `{"title":"","from":"2026-11-01","to":"2026-11-09"}`, "title_required"},
		{"goal inverted", http.MethodPost, "/api/habits/goals", `{"title":"x","from":"2026-11-09","to":"2026-11-01"}`, "invalid_range"},
		{"goal bad id", http.MethodPut, "/api/habits/goals/0", `{}`, "invalid_id"},
		{"note empty", http.MethodPost, "/api/habits/notes", `{"kind":"insight","body":"  "}`, "empty_text"},
		{"note bad kind", http.MethodPost, "/api/habits/notes", `{"kind":"idea","body":"x"}`, "invalid_kind"},
		{"notes bad habit filter", http.MethodGet, "/api/habits/notes?habit=abc", "", "invalid_habit"},
		{"notes bad kind filter", http.MethodGet, "/api/habits/notes?kind=idea", "", "invalid_kind"},
		{"notes bad cursor", http.MethodGet, "/api/habits/notes?before=-1", "", "invalid_cursor"},
		{"notes bad limit", http.MethodGet, "/api/habits/notes?limit=1000", "", "invalid_limit"},
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
