package training

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

// Тесты без БД: валидация запроса срабатывает раньше, чем сервис идёт в базу.
// Остальное — в service_integration_test.go.
func TestHandler_BadRequests(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/training", New(nil, clock.NewFixed(qNow), nil).Routes)

	tests := []struct {
		name, path, body string
		wantCode         string
	}{
		{"start malformed", "/api/training/start", `{"groupId":`, "invalid_json"},
		{"start unknown field", "/api/training/start", `{"groupId":1,"strategy":"en_ru","limit":5}`, "invalid_json"},
		{"start bad strategy", "/api/training/start", `{"groupId":1,"strategy":"en-ru"}`, "invalid_strategy"},
		{"start no strategy", "/api/training/start", `{"groupId":1}`, "invalid_strategy"},
		{"start no group", "/api/training/start", `{"strategy":"en_ru"}`, "invalid_group"},
		{"answer bad session id", "/api/training/abc/answer", `{"wordId":1,"answer":"good"}`, "invalid_id"},
		{"answer bad answer", "/api/training/1/answer", `{"wordId":1,"answer":"easy"}`, "invalid_answer"},
		{"answer no word", "/api/training/1/answer", `{"answer":"good"}`, "invalid_word"},
		{"finish bad id", "/api/training/0/finish", ``, "invalid_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))
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
