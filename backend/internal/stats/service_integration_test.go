//go:build integration

// Интеграционные тесты сервиса stats на живом Postgres (запуск — SPEC §15).
package stats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
)

var testNow = at(10, 12, 0)

func newTestEnv(t *testing.T) (*Service, dbtest.Fixtures) {
	t.Helper()
	pool := dbtest.New(t)
	return NewService(pool, clock.NewFixed(testNow)), dbtest.Fixtures{Pool: pool, Now: testNow}
}

func tptr(t time.Time) *time.Time { return &t }
func iptr(i int64) *int64         { return &i }

func TestService_Overview(t *testing.T) {
	svc, fx := newTestEnv(t)
	ctx := context.Background()
	verbs := fx.Group(t, "verbs")
	sport := fx.Group(t, "sport")
	lesson := fx.Group(t, "lesson")

	run := fx.Word(t, "run", 2, testNow, verbs, sport, lesson) // в трёх группах
	walk := fx.Word(t, "walk", 4, testNow, verbs, lesson)
	ball := fx.Word(t, "ball", 5, testNow, sport)
	fx.Word(t, "idle", 1, testNow)

	// Сегодня: две сессии по lesson (одна free), разобрано run+walk, затем снова run.
	s1 := fx.Session(t, dbtest.Session{GroupID: iptr(lesson), FinishedAt: tptr(at(10, 9, 0)), Total: 2, Good: 1, Again: 1, DurationSec: 100})
	fx.Review(t, s1, run, "good", true, at(10, 8, 59))
	fx.Review(t, s1, walk, "again", true, at(10, 8, 59))
	fx.Review(t, s1, walk, "good", false, at(10, 9, 0)) // повтор — не считается
	s2 := fx.Session(t, dbtest.Session{GroupID: iptr(sport), Free: true, FinishedAt: tptr(at(10, 11, 0)), Total: 2, Good: 2, DurationSec: 50})
	fx.Review(t, s2, run, "good", true, at(10, 10, 59))
	fx.Review(t, s2, ball, "good", true, at(10, 10, 59))
	// Незавершённая сессия сегодня — не в счёт.
	s3 := fx.Session(t, dbtest.Session{GroupID: iptr(verbs)})
	fx.Review(t, s3, run, "good", true, at(10, 11, 30))
	// Вчера и позавчера — для серии; вчерашняя — для ?day=.
	fx.Session(t, dbtest.Session{GroupID: iptr(verbs), FinishedAt: tptr(at(9, 20, 0)), Total: 3, Good: 3, DurationSec: 30})
	fx.Session(t, dbtest.Session{GroupID: iptr(verbs), FinishedAt: tptr(at(8, 20, 0)), Total: 1, Good: 1})

	o, err := svc.Overview(ctx, "")
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if d := o.Day; d.Day != "2026-11-10" || d.Sessions != 2 || d.Words != 4 || d.Good != 3 || d.Again != 1 || d.Seconds != 150 {
		t.Errorf("day = %+v", d)
	}
	if o.GroupsTrained != 2 {
		t.Errorf("groupsTrained = %d, want 2", o.GroupsTrained)
	}
	// run — во всех трёх, walk — verbs+lesson, ball — sport; уникально по слову.
	wantByGroup := []GroupWords{{lesson, "lesson", 2}, {sport, "sport", 2}, {verbs, "verbs", 2}}
	if !reflect.DeepEqual(o.ByGroup, wantByGroup) {
		t.Errorf("byGroup = %+v\nwant %+v", o.ByGroup, wantByGroup)
	}
	if o.Streak != (Streak{Current: 3, Best: 3}) {
		t.Errorf("streak = %+v", o.Streak)
	}
	if o.Totals != (Totals{Words: 4, Learned: 2, Levels: [5]int{1, 1, 0, 1, 1}}) {
		t.Errorf("totals = %+v", o.Totals)
	}

	o, err = svc.Overview(ctx, "2026-11-09")
	if err != nil {
		t.Fatalf("Overview yesterday: %v", err)
	}
	if o.Day.Day != "2026-11-09" || o.Day.Sessions != 1 || o.Day.Words != 3 || len(o.ByGroup) != 0 || o.Streak.Current != 3 {
		t.Errorf("yesterday = %+v", o)
	}

	// Пустой день — нули, не null.
	o, err = svc.Overview(ctx, "2026-10-01")
	if err != nil || o.Day.Sessions != 0 || o.Day.GoodShare != nil || o.ByGroup == nil {
		t.Errorf("empty day = %+v, %v", o, err)
	}
}

func TestService_Calendar(t *testing.T) {
	svc, fx := newTestEnv(t)
	ctx := context.Background()
	fx.Session(t, dbtest.Session{FinishedAt: tptr(at(10, 9, 0)), Total: 5, Good: 4, DurationSec: 60})
	fx.Session(t, dbtest.Session{FinishedAt: tptr(time.Date(2025, 11, 11, 9, 0, 0, 0, msk)), Total: 1, Good: 1})
	fx.Session(t, dbtest.Session{FinishedAt: tptr(time.Date(2025, 11, 10, 9, 0, 0, 0, msk)), Total: 7}) // за окном по умолчанию

	c, err := svc.Calendar(ctx, "", "")
	if err != nil {
		t.Fatalf("Calendar: %v", err)
	}
	if c.From != "2025-11-11" || c.To != "2026-11-10" || len(c.Days) != 365 {
		t.Fatalf("default window = %s..%s, %d days", c.From, c.To, len(c.Days))
	}
	if first, last := c.Days[0], c.Days[364]; first.Words != 1 || last.Words != 5 || last.Seconds != 60 {
		t.Errorf("first %+v, last %+v", first, last)
	}

	c, err = svc.Calendar(ctx, "2026-11-09", "2026-11-10")
	if err != nil || len(c.Days) != 2 || c.Days[0].Sessions != 0 || c.Days[1].Sessions != 1 {
		t.Errorf("custom window = %+v, %v", c, err)
	}
}

func TestService_Trends(t *testing.T) {
	svc, fx := newTestEnv(t)
	fx.Session(t, dbtest.Session{FinishedAt: tptr(at(4, 9, 0)), Total: 2, Good: 1, Hard: 1})
	fx.Session(t, dbtest.Session{FinishedAt: tptr(at(3, 23, 59)), Total: 9}) // 8-й день назад — вне week

	tr, err := svc.Trends(context.Background(), RangeWeek)
	if err != nil {
		t.Fatalf("Trends: %v", err)
	}
	if len(tr.Days) != 7 || tr.Days[0].Day != "2026-11-04" || tr.Days[0].Words != 2 || tr.Days[0].Hard != 1 {
		t.Errorf("week = %+v", tr.Days)
	}
	tr, err = svc.Trends(context.Background(), RangeMonth)
	if err != nil || len(tr.Days) != 30 {
		t.Errorf("month: %d days, %v", len(tr.Days), err)
	}
}

func TestModule_HTTP(t *testing.T) {
	svc, fx := newTestEnv(t)
	fx.Session(t, dbtest.Session{FinishedAt: tptr(at(10, 9, 0)), Total: 2, Good: 2})

	r := chi.NewRouter()
	r.Route("/api/stats", (&Module{svc: svc}).Routes)
	get := func(path string, out any) int {
		t.Helper()
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code == 200 {
			if err := json.NewDecoder(rr.Body).Decode(out); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
		}
		return rr.Code
	}

	var o Overview
	if code := get("/api/stats/overview", &o); code != 200 || o.Day.Words != 2 || o.Streak.Current != 1 {
		t.Errorf("overview: %d %+v", code, o)
	}
	var c Calendar
	if code := get("/api/stats/calendar?from=2026-11-01&to=2026-11-10", &c); code != 200 || len(c.Days) != 10 {
		t.Errorf("calendar: %d %d days", code, len(c.Days))
	}
	var tr Trends
	if code := get("/api/stats/trends", &tr); code != 200 || tr.Range != RangeWeek || len(tr.Days) != 7 {
		t.Errorf("trends default: %d %+v", code, tr)
	}
}
