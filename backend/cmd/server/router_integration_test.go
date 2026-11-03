//go:build integration

// Смоук-тест всего приложения: роутер собирается как в main, сценарий идёт через HTTP.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
)

func TestRouter_EndToEnd(t *testing.T) {
	pool := dbtest.New(t)
	clk := clock.NewFixed(time.Date(2026, 11, 2, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60)))
	bus := eventbus.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(bus.Close)
	h, err := newRouter(context.Background(), pool, clk, bus)
	if err != nil {
		t.Fatalf("newRouter: %v", err)
	}

	call := func(method, path, body string, out any) int {
		t.Helper()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		h.ServeHTTP(rr, req)
		if out != nil {
			if err := json.NewDecoder(rr.Body).Decode(out); err != nil {
				t.Fatalf("%s %s: decode: %v (status %d)", method, path, err, rr.Code)
			}
		}
		return rr.Code
	}

	if code := call(http.MethodGet, "/healthz", "", nil); code != 200 {
		t.Fatalf("healthz: %d", code)
	}
	var apiErr struct{ Error struct{ Code string } }
	if code := call(http.MethodGet, "/api/nope", "", &apiErr); code != 404 || apiErr.Error.Code != "not_found" {
		t.Errorf("unknown api: %d %+v", code, apiErr)
	}
	if code := call(http.MethodDelete, "/api/settings", "", &apiErr); code != 405 || apiErr.Error.Code != "method_not_allowed" {
		t.Errorf("wrong method: %d %+v", code, apiErr)
	}
	if code := call(http.MethodGet, "/", "", nil); code != 200 && code != 503 { // 503 — фронт не собран
		t.Errorf("frontend: %d", code)
	}

	// Ввод → группа появилась.
	var bulk struct{ Created int }
	call(http.MethodPost, "/api/words/bulk", `{"text":"apple || яблоко || I eat an [apple] || food\npear || груша || || food"}`, &bulk)
	if bulk.Created != 2 {
		t.Fatalf("bulk: %+v", bulk)
	}
	var groups struct {
		Groups []struct {
			ID   int64
			Name string
			Due  int
		}
	}
	call(http.MethodGet, "/api/groups", "", &groups)
	if len(groups.Groups) != 1 || groups.Groups[0].Due != 2 {
		t.Fatalf("groups: %+v", groups)
	}
	gid := groups.Groups[0].ID

	// Тренировка: старт → ответы → итоги.
	var start struct {
		SessionID *int64
		Cards     []struct{ WordID int64 }
	}
	call(http.MethodPost, "/api/training/start", fmt.Sprintf(`{"groupId":%d,"strategy":"en_ru"}`, gid), &start)
	if start.SessionID == nil || len(start.Cards) != 2 {
		t.Fatalf("start: %+v", start)
	}
	for _, c := range start.Cards {
		if code := call(http.MethodPost, fmt.Sprintf("/api/training/%d/answer", *start.SessionID),
			fmt.Sprintf(`{"wordId":%d,"answer":"good"}`, c.WordID), nil); code != 200 {
			t.Fatalf("answer: %d", code)
		}
	}
	var fin struct{ Total, Good int }
	call(http.MethodPost, fmt.Sprintf("/api/training/%d/finish", *start.SessionID), "", &fin)
	if fin.Total != 2 || fin.Good != 2 {
		t.Errorf("finish: %+v", fin)
	}

	// Статистика и ачивки видят сессию сразу.
	var ov struct {
		Day    struct{ Words int }
		Streak struct{ Current int }
	}
	call(http.MethodGet, "/api/stats/overview", "", &ov)
	if ov.Day.Words != 2 || ov.Streak.Current != 1 {
		t.Errorf("overview: %+v", ov)
	}
	var ach struct {
		Achievements []struct {
			Code       string
			UnlockedAt *time.Time
		}
	}
	call(http.MethodGet, "/api/achievements", "", &ach)
	if len(ach.Achievements) == 0 || ach.Achievements[0].Code != "first_session" || ach.Achievements[0].UnlockedAt == nil {
		t.Errorf("achievements: %+v", ach)
	}

	// Группа «повторена», бэкап отдаётся.
	call(http.MethodGet, "/api/groups", "", &groups)
	var settings struct{ Language string }
	if code := call(http.MethodGet, "/api/settings", "", &settings); code != 200 || settings.Language != "ru" {
		t.Errorf("settings: %d %+v", code, settings)
	}
	var backup struct {
		Version int
		Words   []any
	}
	if code := call(http.MethodGet, "/api/data/export", "", &backup); code != 200 || backup.Version != 1 || len(backup.Words) != 2 {
		t.Errorf("export: %d %+v", code, backup)
	}
}
