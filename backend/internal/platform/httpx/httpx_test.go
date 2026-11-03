package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeErrorBody(t *testing.T, rr *httptest.ResponseRecorder) errorPayload {
	t.Helper()
	var body errorBody
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v (raw %q)", err, rr.Body.String())
	}
	return body.Error
}

func TestDecode(t *testing.T) {
	type req struct {
		Text string `json:"text"`
	}
	tests := []struct {
		name       string
		body       string
		wantStatus int // 0 — ошибки нет
		wantCode   string
	}{
		{"ok", `{"text":"apple || яблоко"}`, 0, ""},
		{"unknown field", `{"text":"a","oops":1}`, 400, "invalid_json"},
		{"malformed", `{"text":`, 400, "invalid_json"},
		{"two values", `{"text":"a"}{"text":"b"}`, 400, "invalid_json"},
		{"too large", `{"text":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, 413, "body_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var v req
			err := Decode(httptest.NewRecorder(), r, &v)

			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var he *Error
			if !errors.As(err, &he) {
				t.Fatalf("want *Error, got %v", err)
			}
			if he.Status != tt.wantStatus || he.Code != tt.wantCode {
				t.Errorf("got %d %q, want %d %q", he.Status, he.Code, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"http error", NotFound("word not found"), 404, "not_found", "word not found"},
		{"wrapped http error", fmt.Errorf("words.Get: %w", BadRequest("bad_level", "level must be 1..5")), 400, "bad_level", "level must be 1..5"},
		// детали внутренней ошибки наружу не утекают
		{"plain error", errors.New("pq: connection refused"), 500, "internal", "internal server error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			WriteError(rr, httptest.NewRequest(http.MethodGet, "/", nil), tt.err)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			got := decodeErrorBody(t, rr)
			if got.Code != tt.wantCode || got.Message != tt.wantMsg {
				t.Errorf("body = %+v, want code %q msg %q", got, tt.wantCode, tt.wantMsg)
			}
		})
	}
}

func TestHandle(t *testing.T) {
	ok := Handle(func(w http.ResponseWriter, r *http.Request) error {
		JSON(w, http.StatusCreated, map[string]int{"created": 3})
		return nil
	})
	rr := httptest.NewRecorder()
	ok(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}

	fail := Handle(func(w http.ResponseWriter, r *http.Request) error {
		return Conflict("duplicate", "already exists")
	})
	rr = httptest.NewRecorder()
	fail(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusConflict || decodeErrorBody(t, rr).Code != "duplicate" {
		t.Errorf("got %d %q", rr.Code, rr.Body.String())
	}
}

func TestRecover(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	if got := decodeErrorBody(t, rr).Code; got != "internal" {
		t.Errorf("code = %q, want internal", got)
	}
}

func TestRecover_AbortHandlerRepanics(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", rec)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestStatusRecorder(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    int
	}{
		{"implicit 200", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hi")) }, 200},
		{"explicit 404", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }, 404},
		{"no write at all", func(w http.ResponseWriter, r *http.Request) {}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
			tt.handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.status != tt.want {
				t.Errorf("status = %d, want %d", rec.status, tt.want)
			}
		})
	}
}
