package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>Lexis</title>")},
		"assets/app-abc123.js": {Data: []byte("console.log(1)")},
		"favicon.svg":          {Data: []byte("<svg/>")},
	}
	h := newHandler(fsys)

	tests := []struct {
		name, path   string
		status       int
		bodyContains string
		cache        string
	}{
		{"root → index", "/", 200, "<title>Lexis</title>", "no-cache"},
		{"index.html directly", "/index.html", 200, "<title>Lexis</title>", "no-cache"},
		{"hashed asset cached forever", "/assets/app-abc123.js", 200, "console.log", "public, max-age=31536000, immutable"},
		{"other static file", "/favicon.svg", 200, "<svg/>", ""},
		{"unknown path → index (SPA fallback)", "/groups/42", 200, "<title>Lexis</title>", "no-cache"},
		{"path traversal is cleaned", "/../../etc/passwd", 200, "<title>Lexis</title>", "no-cache"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rr.Code != tt.status || !strings.Contains(rr.Body.String(), tt.bodyContains) {
				t.Errorf("got %d %q", rr.Code, rr.Body)
			}
			if got := rr.Header().Get("Cache-Control"); got != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cache)
			}
		})
	}
}

func TestHandler_NotBuilt(t *testing.T) {
	h := newHandler(fstest.MapFS{".gitkeep": {}})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "npm run build") {
		t.Errorf("got %d %q", rr.Code, rr.Body)
	}
}

func TestEmbeddedHandlerBuilds(t *testing.T) {
	// Встроенный dist (в репо — только .gitkeep) не должен паниковать.
	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d", rr.Code)
	}
}
