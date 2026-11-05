package sport

import (
	"errors"
	"strings"
	"testing"
	"time"

	"lexis/internal/platform/httpx"
)

func code(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	if err != nil {
		return "unexpected: " + err.Error()
	}
	return ""
}

func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }

func TestEntryInput_Normalize(t *testing.T) {
	got, err := EntryInput{Name: "  Жим   лёжа ", Sets: 3, Reps: 10, Weight: fp(60)}.Normalize()
	if err != nil || got.Name != "Жим лёжа" {
		t.Fatalf("got %+v %v", got, err)
	}
	tests := []struct {
		name string
		in   EntryInput
		code string
	}{
		{"bodyweight", EntryInput{Name: "Подтягивания", Sets: 4, Reps: 8}, ""},
		{"empty bar", EntryInput{Name: "Жим", Sets: 1, Reps: 1, Weight: fp(0)}, ""},
		{"no name", EntryInput{Name: " ", Sets: 3, Reps: 10}, "invalid_exercise_name"},
		{"long name", EntryInput{Name: strings.Repeat("я", 61), Sets: 3, Reps: 10}, "invalid_exercise_name"},
		{"zero sets", EntryInput{Name: "x", Sets: 0, Reps: 10}, "invalid_set"},
		{"51 sets", EntryInput{Name: "x", Sets: 51, Reps: 10}, "invalid_set"},
		{"zero reps", EntryInput{Name: "x", Sets: 3, Reps: 0}, "invalid_set"},
		{"too heavy", EntryInput{Name: "x", Sets: 3, Reps: 1, Weight: fp(1001)}, "invalid_set"},
		{"negative weight", EntryInput{Name: "x", Sets: 3, Reps: 1, Weight: fp(-1)}, "invalid_set"},
	}
	for _, tt := range tests {
		if _, err := tt.in.Normalize(); code(err) != tt.code {
			t.Errorf("%s: code = %q, want %q", tt.name, code(err), tt.code)
		}
	}
}

func TestTonnage(t *testing.T) {
	tests := []struct {
		sets, reps int
		weight     *float64
		want       float64
	}{
		{3, 10, fp(60), 1800},
		{4, 8, fp(62.5), 2000},
		{4, 8, nil, 0}, // без веса — не в тоннаж
		{1, 1, fp(0), 0},
	}
	for _, tt := range tests {
		if got := Tonnage(tt.sets, tt.reps, tt.weight); got != tt.want {
			t.Errorf("Tonnage(%d, %d, %v) = %v, want %v", tt.sets, tt.reps, tt.weight, got, tt.want)
		}
	}
}

func TestWorkoutInput_Normalize(t *testing.T) {
	today := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	valid := func() WorkoutInput {
		return WorkoutInput{Day: "2026-11-05", Title: "  Ноги  и   спина ", Notes: " легко \n", DurationMin: ip(60), Kcal: ip(450),
			Exercises: []EntryInput{{Name: " Присед ", Sets: 3, Reps: 5, Weight: fp(80)}}}
	}
	w, err := valid().Normalize(today)
	if err != nil || w.Title != "Ноги и спина" || w.Notes != "легко" || w.Exercises[0].Name != "Присед" {
		t.Fatalf("got %+v %v", w, err)
	}

	tests := []struct {
		name   string
		mutate func(*WorkoutInput)
		code   string
	}{
		{"past day is fine", func(in *WorkoutInput) { in.Day = "2026-10-01" }, ""},
		{"no exercises is fine (just a run)", func(in *WorkoutInput) { in.Exercises = nil }, ""},
		{"future day", func(in *WorkoutInput) { in.Day = "2026-11-06" }, "future_day"},
		{"bad day", func(in *WorkoutInput) { in.Day = "вчера" }, "invalid_date"},
		{"empty title", func(in *WorkoutInput) { in.Title = " " }, "invalid_title"},
		{"long title", func(in *WorkoutInput) { in.Title = strings.Repeat("я", 101) }, "invalid_title"},
		{"long notes", func(in *WorkoutInput) { in.Notes = strings.Repeat("я", MaxNotesLen+1) }, "text_too_long"},
		{"zero duration", func(in *WorkoutInput) { in.DurationMin = ip(0) }, "invalid_duration"},
		{"negative kcal", func(in *WorkoutInput) { in.Kcal = ip(-1) }, "invalid_kcal"},
		{"bad exercise", func(in *WorkoutInput) { in.Exercises[0].Reps = 0 }, "invalid_set"},
		{"too many exercises", func(in *WorkoutInput) { in.Exercises = make([]EntryInput, MaxExercises+1) }, "too_many_items"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := valid()
			tt.mutate(&in)
			if _, err := in.Normalize(today); code(err) != tt.code {
				t.Errorf("code = %q, want %q", code(err), tt.code)
			}
		})
	}
}

func TestExerciseName(t *testing.T) {
	if got, err := ExerciseName("  Жим   лёжа "); err != nil || got != "Жим лёжа" {
		t.Errorf("got %q %v", got, err)
	}
	for _, bad := range []string{"", "  ", strings.Repeat("я", 61)} {
		if _, err := ExerciseName(bad); code(err) != "invalid_exercise_name" {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
