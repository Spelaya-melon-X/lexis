package sport

import (
	"cmp"
	"slices"
	"time"

	"lexis/internal/platform/clock"
)

// Статистика спорта и прогресс упражнения (§18.4, C2). Чистые функции: сервис достаёт тренировки,
// еду и историю упражнения, здесь — раскладка по дням, итоги, средние.

// EntryStat — упражнение тренировки для статистики.
type EntryStat struct {
	ExerciseID int64
	Name       string
	Sets, Reps int
	Weight     *float64
}

type WorkoutStat struct {
	Day         time.Time
	Kcal        *int
	DurationMin *int
	Entries     []EntryStat
}

type MealStat struct {
	Day  time.Time
	Kcal *int
}

type DayStat struct {
	Day         string  `json:"day"`
	Workouts    int     `json:"workouts"`
	BurnedKcal  int     `json:"burnedKcal"`
	DurationMin int     `json:"durationMin"`
	Tonnage     float64 `json:"tonnage"`
	EatenKcal   *int    `json:"eatenKcal"` // null — еду в этот день не записывали (а не «0 ккал»)
}

type ExerciseStat struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Workouts  int      `json:"workouts"`
	Sets      int      `json:"sets"`
	Reps      int      `json:"reps"` // всего повторов: Σ подходы × повторы
	MaxWeight *float64 `json:"maxWeight"`
	Tonnage   float64  `json:"tonnage"`
}

// Totals — итоги окна. Средние — только по тем, где значение есть (как в итогах недели на фронте):
// день без записанной еды не тянет «съедено в среднем» к нулю.
type Totals struct {
	Workouts        int      `json:"workouts"`
	WorkoutDays     int      `json:"workoutDays"`
	BurnedKcal      int      `json:"burnedKcal"`
	DurationMin     int      `json:"durationMin"`
	Tonnage         float64  `json:"tonnage"`
	EatenKcal       int      `json:"eatenKcal"`
	FoodDays        int      `json:"foodDays"`
	AvgBurned       *float64 `json:"avgBurned"`       // на тренировку с ккал
	AvgDuration     *float64 `json:"avgDuration"`     // на тренировку с длительностью
	AvgTonnage      *float64 `json:"avgTonnage"`      // на тренировку с весами
	AvgEatenPerDay  *float64 `json:"avgEatenPerDay"`  // на день с записанной едой
	AvgBurnedPerDay *float64 `json:"avgBurnedPerDay"` // на день окна (дни без тренировок — 0: это и есть картина недели)
}

type Stats struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Days      []DayStat      `json:"days"`
	Totals    Totals         `json:"totals"`
	Exercises []ExerciseStat `json:"exercises"` // по тоннажу, потом по числу тренировок
}

// Summarize раскладывает тренировки и еду по всем дням окна [from, to].
func Summarize(from, to time.Time, workouts []WorkoutStat, meals []MealStat) Stats {
	n := clock.DaysBetween(from, to) + 1
	st := Stats{From: clock.FormatDate(from), To: clock.FormatDate(to), Days: make([]DayStat, n), Exercises: []ExerciseStat{}}
	for i := range st.Days {
		st.Days[i].Day = clock.FormatDate(from.AddDate(0, 0, i))
	}
	at := func(d time.Time) *DayStat {
		i := clock.DaysBetween(from, d)
		if i < 0 || i >= n {
			return nil
		}
		return &st.Days[i]
	}

	var burned, durations, tonnages []float64
	ex := map[int64]*ExerciseStat{}
	for _, w := range workouts {
		d := at(w.Day)
		if d == nil {
			continue
		}
		d.Workouts++
		st.Totals.Workouts++
		if w.Kcal != nil {
			d.BurnedKcal += *w.Kcal
			burned = append(burned, float64(*w.Kcal))
		}
		if w.DurationMin != nil {
			d.DurationMin += *w.DurationMin
			durations = append(durations, float64(*w.DurationMin))
		}
		wt := 0.0
		for _, e := range w.Entries {
			t := Tonnage(e.Sets, e.Reps, e.Weight)
			wt += t
			s := ex[e.ExerciseID]
			if s == nil {
				s = &ExerciseStat{ID: e.ExerciseID, Name: e.Name}
				ex[e.ExerciseID] = s
			}
			s.Workouts++
			s.Sets += e.Sets
			s.Reps += e.Sets * e.Reps
			s.Tonnage += t
			if e.Weight != nil && (s.MaxWeight == nil || *e.Weight > *s.MaxWeight) {
				s.MaxWeight = new(*e.Weight)
			}
		}
		d.Tonnage += wt
		if wt > 0 {
			tonnages = append(tonnages, wt)
		}
	}
	for _, m := range meals {
		d := at(m.Day)
		if d == nil {
			continue
		}
		if d.EatenKcal == nil {
			d.EatenKcal = new(0)
		}
		if m.Kcal != nil {
			*d.EatenKcal += *m.Kcal
		}
	}

	t := &st.Totals
	var eaten []float64
	for _, d := range st.Days {
		t.BurnedKcal += d.BurnedKcal
		t.DurationMin += d.DurationMin
		t.Tonnage += d.Tonnage
		if d.Workouts > 0 {
			t.WorkoutDays++
		}
		if d.EatenKcal != nil {
			t.FoodDays++
			t.EatenKcal += *d.EatenKcal
			eaten = append(eaten, float64(*d.EatenKcal))
		}
	}
	t.AvgBurned, t.AvgDuration, t.AvgTonnage, t.AvgEatenPerDay = avg(burned), avg(durations), avg(tonnages), avg(eaten)
	t.AvgBurnedPerDay = new(float64(t.BurnedKcal) / float64(n))

	for _, s := range ex {
		st.Exercises = append(st.Exercises, *s)
	}
	slices.SortFunc(st.Exercises, func(a, b ExerciseStat) int {
		return cmp.Or(cmp.Compare(b.Tonnage, a.Tonnage), cmp.Compare(b.Workouts, a.Workouts), cmp.Compare(a.Name, b.Name))
	})
	return st
}

func avg(xs []float64) *float64 {
	if len(xs) == 0 {
		return nil
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return new(sum / float64(len(xs)))
}

// HistoryRow — одна запись упражнения в тренировке.
type HistoryRow struct {
	Day        time.Time
	Sets, Reps int
	Weight     *float64
}

// ProgressPoint — упражнение за день (если в день было несколько записей — вместе).
type ProgressPoint struct {
	Day       string   `json:"day"`
	MaxWeight *float64 `json:"maxWeight"` // null — делал без веса
	Tonnage   float64  `json:"tonnage"`
	Sets      int      `json:"sets"`
	Reps      int      `json:"reps"` // всего повторов
}

// Progress — точки графика прогресса по дням, в порядке дат (rows уже по дням).
func Progress(rows []HistoryRow) []ProgressPoint {
	out := []ProgressPoint{}
	for _, r := range rows {
		day := clock.FormatDate(r.Day)
		if len(out) == 0 || out[len(out)-1].Day != day {
			out = append(out, ProgressPoint{Day: day})
		}
		p := &out[len(out)-1]
		p.Sets += r.Sets
		p.Reps += r.Sets * r.Reps
		p.Tonnage += Tonnage(r.Sets, r.Reps, r.Weight)
		if r.Weight != nil && (p.MaxWeight == nil || *r.Weight > *p.MaxWeight) {
			p.MaxWeight = new(*r.Weight)
		}
	}
	return out
}
