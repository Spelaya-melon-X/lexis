package sport

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
	r.Route("/api/sport", New(nil, clock.NewFixed(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC))).Routes)
	tests := []struct {
		name, method, path, body, wantCode string
	}{
		{"future day", http.MethodPost, "/api/sport/workouts", `{"day":"2026-11-06","title":"Бег","exercises":[]}`, "future_day"},
		{"no title", http.MethodPost, "/api/sport/workouts", `{"day":"2026-11-05","title":"","exercises":[]}`, "invalid_title"},
		{"bad kcal", http.MethodPost, "/api/sport/workouts", `{"day":"2026-11-05","title":"Бег","kcal":-1,"exercises":[]}`, "invalid_kcal"},
		{"zero sets", http.MethodPost, "/api/sport/workouts", `{"day":"2026-11-05","title":"Зал","exercises":[{"name":"Присед","sets":0,"reps":10}]}`, "invalid_set"},
		{"no exercise name", http.MethodPost, "/api/sport/workouts", `{"day":"2026-11-05","title":"Зал","exercises":[{"name":"","sets":3,"reps":10}]}`, "invalid_exercise_name"},
		{"rename empty", http.MethodPatch, "/api/sport/exercises/1", `{"name":" "}`, "invalid_exercise_name"},
		{"unknown field", http.MethodPost, "/api/sport/workouts", `{"day":"2026-11-05","title":"Зал","mood":5}`, "invalid_json"},
		{"bad id", http.MethodPut, "/api/sport/workouts/abc", `{}`, "invalid_id"},
		{"bad range", http.MethodGet, "/api/sport/workouts?from=2026-11-05&to=2026-11-01", "", "invalid_range"},
		{"food bad day", http.MethodGet, "/api/sport/meals/вчера", "", "invalid_date"},
		{"food future", http.MethodPut, "/api/sport/meals/2026-11-06", `{"meals":[]}`, "future_day"},
		{"food bad kcal", http.MethodPut, "/api/sport/meals/2026-11-05", `{"meals":[{"text":"Каша","kcal":-5}]}`, "invalid_kcal"},
		{"stats bad range", http.MethodGet, "/api/sport/stats?from=2026-11-05&to=2026-11-01", "", "invalid_range"},
		{"progress bad id", http.MethodGet, "/api/sport/exercises/x/progress", "", "invalid_id"},
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
