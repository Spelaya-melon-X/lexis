//go:build integration

// Интеграционные тесты habits на живом Postgres (запуск — SPEC §15).
package habits

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
)

func TestService_HabitsAndCheckins(t *testing.T) {
	pool := dbtest.New(t)
	fx := dbtest.Fixtures{Pool: pool}
	msk, _ := time.LoadLocation("Europe/Moscow")
	// 00:30 МСК 3-го — в UTC ещё 2-е: дата создания и «сегодня» должны быть 3-е.
	clk := clock.NewFixed(time.Date(2026, 11, 3, 0, 30, 0, 0, msk))
	svc := NewService(pool, clk)
	ctx := context.Background()

	run, err := svc.Create(ctx, Fields{Name: "Бег", Kind: KindPositive, TrackAmount: true, TrackRating: true, Unit: "км", Color: ptr("#16A34A")})
	if err != nil {
		t.Fatal(err)
	}
	if run.StartDay != "2026-11-03" || *run.Color != "#16a34a" || run.Archived {
		t.Fatalf("created = %+v", run)
	}
	smoke, err := svc.Create(ctx, Fields{Name: "Курение", Kind: KindNegative, TrackTrigger: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, Fields{Name: "бег", Kind: KindPositive}); errCode(err) != "duplicate_habit" {
		t.Fatalf("duplicate name (case-insensitive) must be 409, got %v", err)
	}

	// Форма дня: обе привычки, задним числом.
	cs, err := svc.SaveDay(ctx, "2026-11-01", []CheckinInput{
		{HabitID: run.ID, Done: true, Amount: ptr(5.5), Rating: ptr(8)},
		{HabitID: smoke.ID, Done: true, Trigger: ptr(" стресс ")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || *cs[0].Amount != 5.5 || *cs[0].Rating != 8 || *cs[1].Trigger != "стресс" {
		t.Fatalf("saved = %+v", cs)
	}
	// Правка: количество меняется, оценка убирается; курение снимаем.
	cs, err = svc.SaveDay(ctx, "2026-11-01", []CheckinInput{
		{HabitID: run.ID, Done: true, Amount: ptr(10.0)},
		{HabitID: smoke.ID, Done: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || *cs[0].Amount != 10 || cs[0].Rating != nil {
		t.Fatalf("edited = %+v", cs)
	}
	// Ошибка в одной строке — ничего не сохраняется.
	_, err = svc.SaveDay(ctx, "2026-11-02", []CheckinInput{
		{HabitID: smoke.ID, Done: true},
		{HabitID: run.ID, Done: true}, // amount_required
	})
	if errCode(err) != "amount_required" {
		t.Fatalf("want amount_required, got %v", err)
	}
	if _, err := svc.SaveDay(ctx, "2026-11-02", []CheckinInput{{HabitID: 999, Done: true}}); errCode(err) != "invalid_habit" {
		t.Fatalf("unknown habit: %v", err)
	}
	if n := fx.Int(t, `SELECT count(*) FROM habit_checkins WHERE day = '2026-11-02'`); n != 0 {
		t.Fatalf("failed save must not write anything, got %d rows", n)
	}

	// Окно отметок.
	from, to, all, err := svc.Checkins(ctx, "2026-11-01", "2026-11-03")
	if err != nil || len(all) != 1 || clock.FormatDate(from) != "2026-11-01" || clock.FormatDate(to) != "2026-11-03" {
		t.Fatalf("checkins: %v %v %+v %v", from, to, all, err)
	}

	// Архив: пропадает из списка, но остаётся с ?archived=true; порядок.
	if _, err := svc.Update(ctx, smoke.ID, Patch{Archived: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if hs, _ := svc.List(ctx, false); len(hs) != 1 || hs[0].ID != run.ID {
		t.Fatalf("active = %+v", hs)
	}
	if err := svc.Reorder(ctx, []int64{smoke.ID, run.ID}); err != nil {
		t.Fatal(err)
	}
	if hs, _ := svc.List(ctx, true); len(hs) != 2 || hs[0].ID != smoke.ID || !hs[0].Archived {
		t.Fatalf("all = %+v", hs)
	}
	if err := svc.Reorder(ctx, []int64{run.ID}); errCode(err) != "invalid_order" {
		t.Fatalf("partial order: %v", err)
	}
	if _, err := svc.Update(ctx, 999, Patch{}); errCode(err) != "not_found" {
		t.Fatalf("update missing: %v", err)
	}
}

func TestService_GoalsAndNotes(t *testing.T) {
	pool := dbtest.New(t)
	msk, _ := time.LoadLocation("Europe/Moscow")
	clk := clock.NewFixed(time.Date(2026, 11, 5, 20, 0, 0, 0, msk))
	svc := NewService(pool, clk)
	ctx := context.Background()

	run, _ := svc.Create(ctx, Fields{Name: "Бег", Kind: KindPositive})
	smoke, _ := svc.Create(ctx, Fields{Name: "Курение", Kind: KindNegative})
	for _, d := range []string{"2026-11-01", "2026-11-02", "2026-11-04"} {
		if _, err := svc.SaveDay(ctx, d, []CheckinInput{{HabitID: run.ID, Done: true}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.SaveDay(ctx, "2026-11-03", []CheckinInput{{HabitID: smoke.ID, Done: true}}); err != nil {
		t.Fatal(err)
	}

	g, err := svc.CreateGoal(ctx, GoalInput{HabitID: &run.ID, From: "2026-11-01", To: "2026-11-09"})
	if err != nil {
		t.Fatal(err)
	}
	if g.Progress == nil || *g.Progress != (Progress{Done: 3, Elapsed: 5, Total: 9}) {
		t.Fatalf("run goal = %+v", g.Progress)
	}
	gs, err := svc.CreateGoal(ctx, GoalInput{HabitID: &smoke.ID, From: "2026-11-01", To: "2026-11-09"})
	if err != nil || *gs.Progress != (Progress{Done: 4, Elapsed: 5, Total: 9}) {
		t.Fatalf("smoke goal = %+v %v", gs.Progress, err)
	}
	free, err := svc.CreateGoal(ctx, GoalInput{Title: "Спать до 23:00", From: "2026-12-01", To: "2026-12-07"})
	if err != nil || free.Progress != nil {
		t.Fatalf("free goal = %+v %v", free, err)
	}
	if _, err := svc.CreateGoal(ctx, GoalInput{HabitID: ptr(int64(999)), From: "2026-11-01", To: "2026-11-02"}); errCode(err) != "invalid_habit" {
		t.Fatalf("goal for unknown habit: %v", err)
	}

	// Окно — только пересекающиеся цели; без окна — все.
	if list, _ := svc.Goals(ctx, "2026-11-08", "2026-11-30"); len(list) != 2 {
		t.Errorf("goals in window = %d", len(list))
	}
	if list, _ := svc.Goals(ctx, "", ""); len(list) != 3 {
		t.Errorf("all goals = %d", len(list))
	}
	if _, err := svc.UpdateGoal(ctx, g.ID, GoalInput{HabitID: &run.ID, From: "2026-11-01", To: "2026-11-02"}); err != nil {
		t.Fatal(err)
	}

	// Лента: 5 записей, страницы по 2, фильтры.
	for i, k := range []string{NoteObservation, NoteInsight, NoteObservation, NoteInsight, NoteInsight} {
		var hid *int64
		if i%2 == 0 {
			hid = &run.ID
		}
		if _, err := svc.CreateNote(ctx, NoteInput{HabitID: hid, Kind: k, Body: "запись " + string(rune('A'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	page, next, err := svc.Notes(ctx, NoteFilter{Limit: 2})
	if err != nil || len(page) != 2 || page[0].Body != "запись E" || next == nil {
		t.Fatalf("page 1 = %+v next %v %v", page, next, err)
	}
	page, next, _ = svc.Notes(ctx, NoteFilter{Limit: 2, Before: *next})
	if len(page) != 2 || page[0].Body != "запись C" {
		t.Fatalf("page 2 = %+v", page)
	}
	page, next, _ = svc.Notes(ctx, NoteFilter{Limit: 2, Before: *next})
	if len(page) != 1 || next != nil {
		t.Fatalf("last page = %+v next %v", page, next)
	}
	if page, _, _ := svc.Notes(ctx, NoteFilter{HabitID: run.ID}); len(page) != 3 {
		t.Errorf("by habit = %d", len(page))
	}
	if page, _, _ := svc.Notes(ctx, NoteFilter{General: true, Kind: NoteInsight}); len(page) != 2 {
		t.Errorf("general insights = %d", len(page))
	}

	// Удаление привычки: отметки и цели — каскадом, наблюдения становятся общими.
	if err := svc.Delete(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.Goals(ctx, "", ""); len(list) != 2 {
		t.Errorf("goals after habit delete = %d", len(list))
	}
	if page, _, _ := svc.Notes(ctx, NoteFilter{General: true}); len(page) != 5 {
		t.Errorf("notes must survive as general, got %d", len(page))
	}
	if err := svc.Delete(ctx, run.ID); errCode(err) != "not_found" {
		t.Errorf("second delete: %v", err)
	}
}

func TestModule_HTTP(t *testing.T) {
	pool := dbtest.New(t)
	msk, _ := time.LoadLocation("Europe/Moscow")
	r := chi.NewRouter()
	r.Route("/api/habits", New(pool, clock.NewFixed(time.Date(2026, 11, 3, 12, 0, 0, 0, msk))).Routes)

	do := func(method, path, body string, want int) map[string]any {
		t.Helper()
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rr.Code != want {
			t.Fatalf("%s %s: status %d, want %d, body %s", method, path, rr.Code, want, rr.Body)
		}
		var out map[string]any
		_ = json.NewDecoder(rr.Body).Decode(&out)
		return out
	}

	h := do(http.MethodPost, "/api/habits", `{"name":"Вода","kind":"positive","trackAmount":true,"unit":"стаканов"}`, http.StatusCreated)
	id := int(h["id"].(float64))
	do(http.MethodPost, "/api/habits", `{"name":"вода","kind":"negative"}`, http.StatusConflict)

	got := do(http.MethodPut, "/api/habits/checkins/2026-11-03", `{"checkins":[{"habitId":`+strconv.Itoa(id)+`,"done":true,"amount":6}]}`, http.StatusOK)
	if cs := got["checkins"].([]any); len(cs) != 1 || cs[0].(map[string]any)["amount"] != 6.0 {
		t.Errorf("save day = %v", got)
	}
	do(http.MethodPut, "/api/habits/checkins/2026-11-03", `{"checkins":[{"habitId":`+strconv.Itoa(id)+`,"done":true,"rating":5,"amount":1}]}`, http.StatusBadRequest)

	got = do(http.MethodPatch, "/api/habits/"+strconv.Itoa(id), `{"color":"#2563eb","archived":true}`, http.StatusOK)
	if got["archived"] != true || got["color"] != "#2563eb" {
		t.Errorf("patch = %v", got)
	}
	if hs := do(http.MethodGet, "/api/habits", "", http.StatusOK)["habits"].([]any); len(hs) != 0 {
		t.Errorf("archived habit must be hidden: %v", hs)
	}
	do(http.MethodPost, "/api/habits/notes", `{"habitId":`+strconv.Itoa(id)+`,"kind":"insight","body":"пить с утра"}`, http.StatusCreated)
	got = do(http.MethodGet, "/api/habits/notes?habit="+strconv.Itoa(id), "", http.StatusOK)
	if ns := got["notes"].([]any); len(ns) != 1 || got["next"] != nil {
		t.Errorf("notes = %v", got)
	}
	do(http.MethodDelete, "/api/habits/"+strconv.Itoa(id), "", http.StatusNoContent)
	do(http.MethodDelete, "/api/habits/"+strconv.Itoa(id), "", http.StatusNotFound)
}
