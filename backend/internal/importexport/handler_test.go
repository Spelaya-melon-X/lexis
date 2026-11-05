package importexport

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
)

// multipartBody собирает тело с файлом в поле field.
func multipartBody(t *testing.T, field, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, "words.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

// Тесты без БД: ошибки запроса и формата файла ловятся раньше, чем сервис идёт в базу.
func TestHandler_BadRequests(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/data", New(nil, clock.NewFixed(testDay), nil).Routes)

	type req struct {
		method, path string
		body         *bytes.Buffer
		contentType  string
	}
	file := func(field, content string) req {
		b, ct := multipartBody(t, field, content)
		return req{http.MethodPost, "/api/data/import", b, ct}
	}
	tests := []struct {
		name     string
		req      req
		status   int
		wantCode string
	}{
		{"import not multipart", req{http.MethodPost, "/api/data/import", bytes.NewBufferString(`{}`), "application/json"}, 400, "invalid_multipart"},
		{"import wrong field", file("upload", "term,translation\na,b"), 400, "missing_file"},
		{"import empty file", file("file", "  \n"), 400, "empty_file"},
		{"import csv without columns", file("file", "word,meaning\na,b"), 400, "csv_missing_columns"},
		{"import broken json list", file("file", `[{"term":`), 400, "invalid_json"},
		{"import backup of other version", file("file", `{"version":9}`), 400, "unsupported_backup_version"},
		{"export bad format", req{http.MethodGet, "/api/data/export?format=xml", &bytes.Buffer{}, ""}, 400, "invalid_format"},
		{"reset without confirm", req{http.MethodPost, "/api/data/reset", bytes.NewBufferString(`{}`), "application/json"}, 400, "confirm_required"},
		{"reset confirm false", req{http.MethodPost, "/api/data/reset", bytes.NewBufferString(`{"confirm":false}`), "application/json"}, 400, "confirm_required"},
		{"reset bad scope", req{http.MethodPost, "/api/data/reset", bytes.NewBufferString(`{"confirm":true,"scope":"words"}`), "application/json"}, 400, "invalid_scope"},
		{"reset no body", req{http.MethodPost, "/api/data/reset", &bytes.Buffer{}, ""}, 400, "invalid_json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hr := httptest.NewRequest(tt.req.method, tt.req.path, tt.req.body)
			if tt.req.contentType != "" {
				hr.Header.Set("Content-Type", tt.req.contentType)
			}
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, hr)
			if rr.Code != tt.status {
				t.Fatalf("status = %d, want %d (body %s)", rr.Code, tt.status, rr.Body)
			}
			var body struct {
				Error struct{ Code string } `json:"error"`
			}
			_ = json.NewDecoder(rr.Body).Decode(&body)
			if body.Error.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tt.wantCode)
			}
		})
	}
}

func TestHandler_ImportTooLarge(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/data", New(nil, clock.NewFixed(testDay), nil).Routes)
	body, ct := multipartBody(t, "file", "term,translation\n"+strings.Repeat("a,b\n", MaxImportBytes/4+1))
	hr := httptest.NewRequest(http.MethodPost, "/api/data/import", body)
	hr.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, hr)
	if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), "file_too_large") {
		t.Errorf("got %d %s", rr.Code, rr.Body.String()[:min(200, rr.Body.Len())])
	}
}
