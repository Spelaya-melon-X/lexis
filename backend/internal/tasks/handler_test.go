package tasks

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
	r.Route("/api/tasks", New(nil, clock.NewFixed(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)), nil).Routes)

	tests := []struct {
		name, method, path, body, wantCode string
	}{
		{"list bad filter", http.MethodGet, "/api/tasks?done=maybe", "", "invalid_filter"},
		{"create empty title", http.MethodPost, "/api/tasks", `{"title":" ","blocks":1,"importance":5,"columnId":1}`, "invalid_title"},
		{"create bad blocks", http.MethodPost, "/api/tasks", `{"title":"x","blocks":0,"importance":5,"columnId":1}`, "invalid_blocks"},
		{"create no column", http.MethodPost, "/api/tasks", `{"title":"x","blocks":1,"importance":5}`, "invalid_column"},
		{"create unknown field", http.MethodPost, "/api/tasks", `{"title":"x","blocks":1,"importance":5,"columnId":1,"parentId":2}`, "invalid_json"},
		{"patch bad id", http.MethodPatch, "/api/tasks/abc", `{}`, "invalid_id"},
		{"move no column", http.MethodPost, "/api/tasks/1/move", `{"beforeId":null}`, "invalid_column"},
		{"type empty name", http.MethodPost, "/api/tasks/types", `{"name":""}`, "invalid_type_name"},
		{"type bad color", http.MethodPost, "/api/tasks/types", `{"name":"x","color":"#xyz"}`, "invalid_color"},
		{"column empty name", http.MethodPost, "/api/tasks/columns", `{"name":"  "}`, "invalid_column_name"},
		{"column delete bad to", http.MethodDelete, "/api/tasks/columns/1?to=abc", "", "invalid_column"},
		{"column bad id", http.MethodPatch, "/api/tasks/columns/0", `{"name":"x"}`, "invalid_id"},
		{"split no steps", http.MethodPost, "/api/tasks/1/split", `{"steps":[]}`, "invalid_steps"},
		{"split bad id", http.MethodPost, "/api/tasks/x/split", `{"steps":[]}`, "invalid_id"},
		{"tree bad id", http.MethodGet, "/api/tasks/0/tree", "", "invalid_id"},
		{"plan bad day", http.MethodGet, "/api/tasks/plan/current", "", "invalid_date"},
		{"calendar inverted dates", http.MethodGet, "/api/tasks/plan?from=2026-12-01&to=2026-11-01", "", "invalid_range"},
		{"add to past plan", http.MethodPost, "/api/tasks/plan/2026-11-04/items", `{"taskId":1}`, "past_day"},
		{"add invalid task", http.MethodPost, "/api/tasks/plan/2026-11-05/items", `{"taskId":0}`, "invalid_task"},
		{"create in future invalid title", http.MethodPost, "/api/tasks/plan/2026-11-06/tasks", `{"title":" ","blocks":1,"importance":5,"columnId":1}`, "invalid_title"},
		{"save past day", http.MethodPut, "/api/tasks/plan/2026-11-04", `{"items":[]}`, "past_day"},
		{"save plan dup", http.MethodPut, "/api/tasks/plan/2026-11-05", `{"items":[{"taskId":1,"start":540,"blocks":1},{"taskId":1,"start":600,"blocks":1}]}`, "invalid_task"},
		{"save plan overlap", http.MethodPut, "/api/tasks/plan/2026-11-05", `{"items":[{"taskId":1,"start":540,"blocks":2},{"taskId":2,"start":570,"blocks":1}]}`, "plan_overlap"},
		{"save plan bad blocks", http.MethodPut, "/api/tasks/plan/2026-11-06", `{"items":[{"taskId":1,"blocks":0}]}`, "invalid_blocks"},
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
