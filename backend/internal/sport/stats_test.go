package sport

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func civil(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestNormalizeMeals(t *testing.T) {
	got, err := NormalizeMeals([]Meal{{Text: "  Овсянка  с ягодами ", Kcal: ip(350)}, {Text: " "}, {Text: "Яблоко"}})
	if err != nil || !reflect.DeepEqual(got, []Meal{{Text: "Овсянка с ягодами", Kcal: ip(350)}, {Text: "Яблоко"}}) {
		t.Fatalf("got %+v %v", got, err)
	}
	if got, _ := NormalizeMeals(nil); got == nil || len(got) != 0 {
		t.Errorf("nil → %#v, want empty non-nil", got)
	}
	many := make([]Meal, MaxMeals+1)
	for i := range many {
		many[i] = Meal{Text: "x"}
	}
	tests := []struct {
		name string
		in   []Meal
		code string
	}{
		{"too many", many, "too_many_items"},
		{"long", []Meal{{Text: strings.Repeat("я", 201)}}, "text_too_long"},
		{"negative kcal", []Meal{{Text: "x", Kcal: ip(-1)}}, "invalid_kcal"},
		{"huge kcal", []Meal{{Text: "x", Kcal: ip(10001)}}, "invalid_kcal"},
		{"empty items don't count to the limit", append(many[:MaxMeals:MaxMeals], Meal{Text: " "}), ""},
	}
	for _, tt := range tests {
		if _, err := NormalizeMeals(tt.in); code(err) != tt.code {
			t.Errorf("%s: code = %q, want %q", tt.name, code(err), tt.code)
		}
	}
	if SumKcal([]Meal{{Kcal: ip(300)}, {}, {Kcal: ip(50)}}) != 350 {
		t.Error("SumKcal")
	}
}

func TestSummarize(t *testing.T) {
	from, to := civil("2026-11-02"), civil("2026-11-04")
	workouts := []WorkoutStat{
		{Day: civil("2026-11-02"), Kcal: ip(400), DurationMin: ip(60), Entries: []EntryStat{
			{ExerciseID: 1, Name: "Присед", Sets: 3, Reps: 5, Weight: fp(80)},
			{ExerciseID: 2, Name: "Подтягивания", Sets: 4, Reps: 8},
		}},
		{Day: civil("2026-11-04"), Kcal: ip(300), Entries: []EntryStat{
			{ExerciseID: 1, Name: "Присед", Sets: 4, Reps: 5, Weight: fp(85)},
		}},
		{Day: civil("2026-11-04"), DurationMin: ip(30)}, // бег без ккал и весов
		{Day: civil("2026-11-05"), Kcal: ip(999)},       // вне окна
	}
	meals := []MealStat{
		{Day: civil("2026-11-02"), Kcal: ip(2000)},
		{Day: civil("2026-11-02"), Kcal: nil}, // без ккал — день всё равно «с едой»
		{Day: civil("2026-11-03")},            // только пункт без ккал → 0, а не null
	}
	st := Summarize(from, to, workouts, meals)
	wantDays := []DayStat{
		{Day: "2026-11-02", Workouts: 1, BurnedKcal: 400, DurationMin: 60, Tonnage: 1200, EatenKcal: ip(2000)},
		{Day: "2026-11-03", EatenKcal: ip(0)},
		{Day: "2026-11-04", Workouts: 2, BurnedKcal: 300, DurationMin: 30, Tonnage: 1700},
	}
	if !reflect.DeepEqual(st.Days, wantDays) {
		t.Errorf("days:\n got  %+v\n want %+v", st.Days, wantDays)
	}
	tot := st.Totals
	if tot.Workouts != 3 || tot.WorkoutDays != 2 || tot.BurnedKcal != 700 || tot.Tonnage != 2900 || tot.EatenKcal != 2000 || tot.FoodDays != 2 {
		t.Errorf("totals: %+v", tot)
	}
	if *tot.AvgBurned != 350 || *tot.AvgDuration != 45 || *tot.AvgTonnage != 1450 || *tot.AvgEatenPerDay != 1000 || *tot.AvgBurnedPerDay != 700.0/3 {
		t.Errorf("averages: burned %v dur %v ton %v eaten %v perDay %v", *tot.AvgBurned, *tot.AvgDuration, *tot.AvgTonnage, *tot.AvgEatenPerDay, *tot.AvgBurnedPerDay)
	}
	if len(st.Exercises) != 2 || st.Exercises[0] != (ExerciseStat{ID: 1, Name: "Присед", Workouts: 2, Sets: 7, Reps: 35, MaxWeight: st.Exercises[0].MaxWeight, Tonnage: 2900}) ||
		*st.Exercises[0].MaxWeight != 85 || st.Exercises[1].MaxWeight != nil || st.Exercises[1].Reps != 32 {
		t.Errorf("exercises: %+v", st.Exercises)
	}

	empty := Summarize(from, from, nil, nil)
	if len(empty.Days) != 1 || empty.Exercises == nil || empty.Totals.AvgBurned != nil || empty.Totals.AvgEatenPerDay != nil {
		t.Errorf("empty: %+v", empty)
	}
}

func TestProgress(t *testing.T) {
	got := Progress([]HistoryRow{
		{Day: civil("2026-11-02"), Sets: 3, Reps: 5, Weight: fp(80)},
		{Day: civil("2026-11-02"), Sets: 2, Reps: 3, Weight: fp(90)}, // второй раз в тот же день — в ту же точку
		{Day: civil("2026-11-05"), Sets: 4, Reps: 8},                 // без веса
	})
	want := []ProgressPoint{
		{Day: "2026-11-02", MaxWeight: fp(90), Tonnage: 1740, Sets: 5, Reps: 21},
		{Day: "2026-11-05", Tonnage: 0, Sets: 4, Reps: 32},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if p := Progress(nil); p == nil || len(p) != 0 {
		t.Errorf("empty: %#v", p)
	}
}
