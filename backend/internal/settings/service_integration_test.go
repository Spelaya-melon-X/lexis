//go:build integration

// Интеграционные тесты settings на живом Postgres (запуск — SPEC §15).
package settings

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/httpx"
)

func TestService(t *testing.T) {
	pool := dbtest.New(t)
	fx := dbtest.Fixtures{Pool: pool}
	svc := NewService(pool)
	ctx := context.Background()

	got, err := svc.Get(ctx)
	if err != nil || got != Defaults() {
		t.Fatalf("empty DB: %+v, %v", got, err)
	}

	lang := "en"
	got, err = svc.Update(ctx, Patch{Language: &lang})
	if err != nil || got != (Settings{"en", 10}) {
		t.Fatalf("update: %+v, %v", got, err)
	}
	limit := 3
	if got, err = svc.Update(ctx, Patch{NewWordsLimit: &limit}); err != nil || got != (Settings{"en", 3}) {
		t.Fatalf("partial update must keep language: %+v, %v", got, err)
	}
	// training читает тот же ключ как JSON-число.
	if n := fx.Int(t, `SELECT (value)::int FROM settings WHERE key = 'new_words_limit'`); n != 3 {
		t.Errorf("stored limit = %d", n)
	}

	bad := 1000
	_, err = svc.Update(ctx, Patch{NewWordsLimit: &bad})
	var he *httpx.Error
	if !errors.As(err, &he) || he.Code != "invalid_new_words_limit" {
		t.Errorf("want invalid_new_words_limit, got %v", err)
	}
	if got, _ := svc.Get(ctx); got != (Settings{"en", 3}) {
		t.Errorf("invalid update must not change anything: %+v", got)
	}
}

func TestModule_HTTP(t *testing.T) {
	pool := dbtest.New(t)
	r := chi.NewRouter()
	r.Route("/api/settings", New(pool).Routes)
	do := func(method, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, "/api/settings", strings.NewReader(body)))
		return rr
	}
	if rr := do(http.MethodGet, ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"newWordsLimit":10`) {
		t.Errorf("get: %d %s", rr.Code, rr.Body)
	}
	if rr := do(http.MethodPut, `{"language":"en"}`); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"language":"en"`) {
		t.Errorf("put: %d %s", rr.Code, rr.Body)
	}
	if rr := do(http.MethodPut, `{"lang":"en"}`); rr.Code != 400 {
		t.Errorf("unknown field: %d", rr.Code)
	}
	if rr := do(http.MethodPut, `{"language":"de"}`); rr.Code != 400 || !strings.Contains(rr.Body.String(), "invalid_language") {
		t.Errorf("bad language: %d %s", rr.Code, rr.Body)
	}
}
