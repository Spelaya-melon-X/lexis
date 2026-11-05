// Package sport — раздел «Спорт» (SPEC §18.4, §18.5.7): тренировки, справочник упражнений.
// Еда и статистика — в фазе C2.
//
// Чистые правила (валидация упражнения и тренировки, тоннаж) — в этом файле; БД — service.go; HTTP — module.go.
package sport

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
)

const (
	MaxTitleLen    = 100
	MaxNotesLen    = 20000
	MaxNameLen     = 60
	MaxExercises   = 30 // в одной тренировке
	MaxSets        = 50 // подходов в упражнении
	MaxReps        = 10000
	MaxWeight      = 1000 // кг
	MaxDurationMin = 1440
	MaxKcal        = 20000
)

// EntryInput — упражнение в тренировке (§18.5.7): по имени из справочника (или новое) и одной строкой
// «подходы × повторы × вес». Вес необязателен — подтягивания, отжимания.
type EntryInput struct {
	Name   string   `json:"name"`
	Sets   int      `json:"sets"`
	Reps   int      `json:"reps"`
	Weight *float64 `json:"weight"` // кг; null — без веса
}

// Normalize: имя 1..60, подходы 1..50, повторы 1..10000, вес 0..1000 или null.
func (e EntryInput) Normalize() (EntryInput, error) {
	name, err := ExerciseName(e.Name)
	if err != nil {
		return e, err
	}
	e.Name = name
	if e.Sets < 1 || e.Sets > MaxSets || e.Reps < 1 || e.Reps > MaxReps || (e.Weight != nil && (*e.Weight < 0 || *e.Weight > MaxWeight)) {
		return e, httpx.BadRequest("invalid_set", fmt.Sprintf("sets 1..%d, reps 1..%d, weight 0..%d kg or empty", MaxSets, MaxReps, MaxWeight))
	}
	return e, nil
}

// Tonnage — подходы × повторы × вес; без веса — 0 (§18.4: тоннаж — только для упражнений с весом).
func Tonnage(sets, reps int, weight *float64) float64 {
	if weight == nil {
		return 0
	}
	return float64(sets*reps) * *weight
}

// WorkoutInput — тело POST/PUT /api/sport/workouts.
type WorkoutInput struct {
	Day         string       `json:"day"`
	Title       string       `json:"title"`
	Notes       string       `json:"notes"`
	DurationMin *int         `json:"durationMin"`
	Kcal        *int         `json:"kcal"`
	Exercises   []EntryInput `json:"exercises"`
}

// Workout — проверенная тренировка (подходы проверяются после того, как известна мера упражнения).
type Workout struct {
	Day         time.Time
	Title       string
	Notes       string
	DurationMin *int
	Kcal        *int
	Exercises   []EntryInput // проверенные
}

// Normalize проверяет поля тренировки. Будущий день — нельзя (как в дневнике: тренировка уже была).
func (in WorkoutInput) Normalize(today time.Time) (Workout, error) {
	day, err := days.Parse(in.Day)
	if err != nil {
		return Workout{}, err
	}
	if err := days.CheckWritable(day, today); err != nil {
		return Workout{}, err
	}
	w := Workout{Day: day, Title: collapse(in.Title), Notes: strings.TrimSpace(in.Notes), DurationMin: in.DurationMin, Kcal: in.Kcal}
	if w.Title == "" || utf8.RuneCountInString(w.Title) > MaxTitleLen {
		return Workout{}, httpx.BadRequest("invalid_title", fmt.Sprintf("title must be 1..%d characters", MaxTitleLen))
	}
	if utf8.RuneCountInString(w.Notes) > MaxNotesLen {
		return Workout{}, httpx.BadRequest("text_too_long", fmt.Sprintf("notes are longer than %d characters", MaxNotesLen))
	}
	if d := w.DurationMin; d != nil && (*d < 1 || *d > MaxDurationMin) {
		return Workout{}, httpx.BadRequest("invalid_duration", fmt.Sprintf("duration must be 1..%d minutes", MaxDurationMin))
	}
	if k := w.Kcal; k != nil && (*k < 0 || *k > MaxKcal) {
		return Workout{}, httpx.BadRequest("invalid_kcal", fmt.Sprintf("kcal must be 0..%d", MaxKcal))
	}
	if len(in.Exercises) > MaxExercises {
		return Workout{}, httpx.BadRequest("too_many_items", fmt.Sprintf("a workout has at most %d exercises", MaxExercises))
	}
	w.Exercises = make([]EntryInput, len(in.Exercises))
	for i, e := range in.Exercises {
		if w.Exercises[i], err = e.Normalize(); err != nil {
			return Workout{}, err
		}
	}
	return w, nil
}

// ExerciseName — имя упражнения: trim, схлопнутые пробелы, 1..60.
func ExerciseName(raw string) (string, error) {
	name := collapse(raw)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLen {
		return "", httpx.BadRequest("invalid_exercise_name", fmt.Sprintf("exercise name must be 1..%d characters", MaxNameLen))
	}
	return name, nil
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }
