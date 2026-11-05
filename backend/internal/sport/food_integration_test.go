//go:build integration

// Еда, статистика и прогресс (§18.4, C2) на живом Postgres.
package sport

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestFood(t *testing.T) {
	s, clk := newService(t)
	ctx := context.Background()
	must(s.Create(ctx, WorkoutInput{Day: "2026-11-05", Title: "Бег", Kcal: ip(300)}))
	must(s.Create(ctx, WorkoutInput{Day: "2026-11-05", Title: "Зал", Kcal: ip(200)}))

	empty := must(s.Food(ctx, "2026-11-05"))
	if len(empty.Meals) != 0 || empty.EatenKcal != 0 || empty.BurnedKcal != 500 {
		t.Fatalf("empty day: %+v", empty)
	}
	day := must(s.SaveFood(ctx, "2026-11-05", []Meal{{Text: " Овсянка ", Kcal: ip(350)}, {Text: ""}, {Text: "Яблоко"}, {Text: "Курица с рисом", Kcal: ip(600)}}))
	if len(day.Meals) != 3 || day.Meals[0].Text != "Овсянка" || day.EatenKcal != 950 || day.BurnedKcal != 500 {
		t.Errorf("saved: %+v", day)
	}
	_, err := s.SaveFood(ctx, "2026-11-06", nil)
	assertCode(t, err, 400, "future_day")

	// Подсказки: частые сверху, ккал — из самой свежей записи.
	clk.Advance(24 * time.Hour)
	must(s.SaveFood(ctx, "2026-11-06", []Meal{{Text: "овсянка", Kcal: ip(300)}}))
	sug := must(s.MealSuggestions(ctx, ""))
	if len(sug) != 3 || sug[0].Text != "овсянка" || *sug[0].Kcal != 300 {
		t.Errorf("suggestions: %+v", sug)
	}
	if got := must(s.MealSuggestions(ctx, "КУР")); len(got) != 1 || got[0].Text != "Курица с рисом" {
		t.Errorf("search: %+v", got)
	}
	// Пустой список — очистить день.
	if d := must(s.SaveFood(ctx, "2026-11-05", nil)); len(d.Meals) != 0 {
		t.Errorf("cleared: %+v", d)
	}
}

func TestStatsAndProgress(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	must(s.Create(ctx, WorkoutInput{Day: "2026-11-02", Title: "Ноги", Kcal: ip(400), DurationMin: ip(60), Exercises: []EntryInput{
		{Name: "Присед", Sets: 3, Reps: 5, Weight: fp(80)},
	}}))
	must(s.Create(ctx, WorkoutInput{Day: "2026-11-04", Title: "Ноги", Kcal: ip(300), Exercises: []EntryInput{
		{Name: "Присед", Sets: 4, Reps: 5, Weight: fp(85)},
		{Name: "Подтягивания", Sets: 4, Reps: 8},
	}}))
	must(s.SaveFood(ctx, "2026-11-03", []Meal{{Text: "Каша", Kcal: ip(2100)}}))

	st := must(s.Stats(ctx, "2026-11-02", "2026-11-05"))
	if len(st.Days) != 4 || st.Days[1].EatenKcal == nil || *st.Days[1].EatenKcal != 2100 || st.Days[0].EatenKcal != nil || st.Days[2].Tonnage != 1700 {
		t.Fatalf("days: %+v", st.Days)
	}
	if tot := st.Totals; tot.Workouts != 2 || tot.BurnedKcal != 700 || tot.Tonnage != 2900 || tot.FoodDays != 1 || *tot.AvgEatenPerDay != 2100 || *tot.AvgBurned != 350 {
		t.Errorf("totals: %+v", tot)
	}
	if names := []string{st.Exercises[0].Name, st.Exercises[1].Name}; !reflect.DeepEqual(names, []string{"Присед", "Подтягивания"}) {
		t.Errorf("exercises: %+v", st.Exercises)
	}

	squat := must(s.Exercises(ctx, "присед"))[0]
	ex, points, err := s.ExerciseProgress(ctx, squat.ID, "", "")
	if err != nil || ex.Name != "Присед" || len(points) != 2 || *points[0].MaxWeight != 80 || *points[1].MaxWeight != 85 || points[1].Tonnage != 1700 {
		t.Errorf("progress: %+v %+v %v", ex, points, err)
	}
	_, _, err = s.ExerciseProgress(ctx, 999, "", "")
	assertCode(t, err, 404, "not_found")
}
