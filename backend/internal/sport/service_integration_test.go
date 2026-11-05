//go:build integration

// Интеграционные тесты раздела «Спорт» (§18.4, §18.5.7) на живом Postgres.
package sport

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/httpx"
)

// Четверг, 5 ноября 2026: неделя — со 2-го по 8-е.
var testNow = time.Date(2026, 11, 5, 19, 0, 0, 0, time.FixedZone("MSK", 3*60*60))

func newService(t *testing.T) (*Service, *clock.Fixed) {
	t.Helper()
	clk := clock.NewFixed(testNow)
	return NewService(dbtest.New(t), clk), clk
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func assertCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) {
		t.Fatalf("want *httpx.Error %d %q, got %v", status, code, err)
	}
	if he.Status != status || he.Code != code {
		t.Errorf("got %d %q, want %d %q", he.Status, he.Code, status, code)
	}
}

func TestWorkouts(t *testing.T) {
	s, clk := newService(t)
	ctx := context.Background()

	legs := must(s.Create(ctx, WorkoutInput{Day: "2026-11-03", Title: "Ноги", DurationMin: ip(60), Kcal: ip(420), Exercises: []EntryInput{
		{Name: "Присед", Sets: 3, Reps: 5, Weight: fp(80)},
		{Name: "Подтягивания", Sets: 4, Reps: 8},
	}}))
	if legs.Tonnage != 1200 || len(legs.Exercises) != 2 || legs.Exercises[1].Weight != nil || legs.Exercises[1].Tonnage != 0 || *legs.Kcal != 420 {
		t.Fatalf("created: %+v", legs)
	}
	// Повторно по имени (другой регистр) — то же упражнение из справочника.
	clk.Advance(time.Hour)
	again := must(s.Create(ctx, WorkoutInput{Day: "2026-11-05", Title: "Ноги", Exercises: []EntryInput{
		{Name: "присед", Sets: 5, Reps: 5, Weight: fp(85)},
	}}))
	if again.Exercises[0].ExerciseID != legs.Exercises[0].ExerciseID || again.Exercises[0].Name != "Присед" {
		t.Errorf("exercise not reused: %+v", again.Exercises[0])
	}
	_, err := s.Create(ctx, WorkoutInput{Day: "2026-11-06", Title: "x"})
	assertCode(t, err, 400, "future_day")

	// Неделя по умолчанию — со 2-го по 8-е; тренировка прошлой недели не попадает.
	must(s.Create(ctx, WorkoutInput{Day: "2026-11-01", Title: "Бег", Kcal: ip(300)}))
	from, to, week, err := s.Workouts(ctx, "", "")
	if err != nil || clock.FormatDate(from) != "2026-11-02" || clock.FormatDate(to) != "2026-11-08" || len(week) != 2 {
		t.Fatalf("week: %v %v %d %v", from, to, len(week), err)
	}

	// PUT заменяет упражнения целиком.
	upd := must(s.Update(ctx, legs.ID, WorkoutInput{Day: "2026-11-03", Title: "Ноги лёгкие", Notes: "устал", Exercises: []EntryInput{
		{Name: "Присед", Sets: 2, Reps: 10, Weight: fp(40)},
	}}))
	if upd.Title != "Ноги лёгкие" || len(upd.Exercises) != 1 || upd.Tonnage != 800 || upd.Kcal != nil {
		t.Errorf("updated: %+v", upd)
	}
	_, err = s.Update(ctx, 999, WorkoutInput{Day: "2026-11-03", Title: "x"})
	assertCode(t, err, 404, "not_found")

	// Справочник: частые сверху; «прошлый раз» — из самой свежей тренировки.
	exs := must(s.Exercises(ctx, ""))
	var names []string
	for _, e := range exs {
		names = append(names, e.Name)
	}
	if !reflect.DeepEqual(names, []string{"Присед", "Подтягивания"}) {
		t.Fatalf("exercises = %v", names)
	}
	squat := exs[0]
	if squat.Uses != 2 || *squat.LastDay != "2026-11-05" || squat.Last == nil || *squat.Last != (Last{Sets: 5, Reps: 5, Weight: squat.Last.Weight}) || *squat.Last.Weight != 85 {
		t.Errorf("squat: %+v last %+v", squat, squat.Last)
	}
	// Подтягивания были только в тренировке, которую потом переписали, — сейчас нигде.
	if pull := exs[1]; pull.Uses != 0 || pull.LastDay != nil || pull.Last != nil {
		t.Errorf("unused pull-ups: %+v", pull)
	}
	if got := must(s.Exercises(ctx, "ПОД")); len(got) != 1 || got[0].Name != "Подтягивания" {
		t.Errorf("search: %+v", got)
	}
	if got := must(s.Titles(ctx, "")); len(got) == 0 || got[0] != "Ноги" {
		t.Errorf("titles = %v", got)
	}

	if e := must(s.RenameExercise(ctx, exs[1].ID, " Подтягивания  широким ")); e.Name != "Подтягивания широким" {
		t.Errorf("rename: %+v", e)
	}
	_, err = s.RenameExercise(ctx, exs[1].ID, "присед")
	assertCode(t, err, 409, "duplicate_exercise")
	assertCode(t, s.DeleteExercise(ctx, squat.ID), 409, "exercise_in_use")
	if err := s.DeleteExercise(ctx, exs[1].ID); err != nil {
		t.Errorf("delete unused: %v", err)
	}

	if err := s.Delete(ctx, again.ID); err != nil {
		t.Fatal(err)
	}
	assertCode(t, s.Delete(ctx, again.ID), 404, "not_found")
}
