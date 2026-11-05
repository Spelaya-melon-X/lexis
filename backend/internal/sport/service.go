package sport

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/db"
	"lexis/internal/platform/httpx"
	"lexis/internal/sport/store"
)

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

var (
	errWorkoutNotFound  = httpx.NotFound("workout not found")
	errExerciseNotFound = httpx.NotFound("exercise not found")
)

// Entry — упражнение внутри тренировки: подходы × повторы × вес.
type Entry struct {
	ExerciseID int64    `json:"exerciseId"`
	Name       string   `json:"name"`
	Sets       int      `json:"sets"`
	Reps       int      `json:"reps"`
	Weight     *float64 `json:"weight"`
	Tonnage    float64  `json:"tonnage"` // подходы × повторы × вес
}

// WorkoutOut — тренировка в API.
type WorkoutOut struct {
	ID          int64     `json:"id"`
	Day         string    `json:"day"`
	Title       string    `json:"title"`
	Notes       string    `json:"notes"`
	DurationMin *int      `json:"durationMin"`
	Kcal        *int      `json:"kcal"`
	Exercises   []Entry   `json:"exercises"`
	Tonnage     float64   `json:"tonnage"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Last — как упражнение делали в прошлый раз: форма подставляет это по умолчанию (§18.5.7).
type Last struct {
	Sets   int      `json:"sets"`
	Reps   int      `json:"reps"`
	Weight *float64 `json:"weight"`
}

// Exercise — упражнение справочника.
type Exercise struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Uses    int     `json:"uses"`
	LastDay *string `json:"lastDay"`
	Last    *Last   `json:"last"` // nil — ещё ни разу не делали
}

// ---------- тренировки ----------

// Workouts — тренировки окна ?from=&to= (по умолчанию — текущая неделя с понедельника).
func (s *Service) Workouts(ctx context.Context, fromS, toS string) (from, to time.Time, out []WorkoutOut, err error) {
	today := clock.TodayDate(s.clock)
	if fromS == "" && toS == "" {
		from = days.WeekStart(today)
		to = from.AddDate(0, 0, 6)
	} else if from, to, err = days.ParseRange(fromS, toS, today, 7); err != nil {
		return
	}
	q := store.New(s.pool)
	rows, err := q.ListWorkouts(ctx, store.ListWorkoutsParams{FromDay: from, ToDay: to})
	if err != nil {
		err = fmt.Errorf("workouts: %w", err)
		return
	}
	out, err = s.assemble(ctx, q, rows)
	return
}

func (s *Service) Get(ctx context.Context, id int64) (WorkoutOut, error) {
	q := store.New(s.pool)
	return s.get(ctx, q, id)
}

func (s *Service) get(ctx context.Context, q *store.Queries, id int64) (WorkoutOut, error) {
	row, err := q.GetWorkout(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkoutOut{}, errWorkoutNotFound
	}
	if err != nil {
		return WorkoutOut{}, fmt.Errorf("get workout %d: %w", id, err)
	}
	out, err := s.assemble(ctx, q, []store.Workout{row})
	if err != nil {
		return WorkoutOut{}, err
	}
	return out[0], nil
}

func (s *Service) Create(ctx context.Context, in WorkoutInput) (WorkoutOut, error) {
	w, err := in.Normalize(clock.TodayDate(s.clock))
	if err != nil {
		return WorkoutOut{}, err
	}
	var out WorkoutOut
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.CreateWorkout(ctx, store.CreateWorkoutParams{
			Day: w.Day, Title: w.Title, Notes: w.Notes, DurationMin: i32(w.DurationMin), Kcal: i32(w.Kcal), Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}
		if err := s.saveExercises(ctx, q, row.ID, w.Exercises); err != nil {
			return err
		}
		out, err = s.get(ctx, q, row.ID)
		return err
	})
	if err != nil {
		return WorkoutOut{}, fmt.Errorf("workout create: %w", err)
	}
	return out, nil
}

// Update заменяет тренировку целиком (упражнения и подходы — списком, как блоки недели).
func (s *Service) Update(ctx context.Context, id int64, in WorkoutInput) (WorkoutOut, error) {
	w, err := in.Normalize(clock.TodayDate(s.clock))
	if err != nil {
		return WorkoutOut{}, err
	}
	var out WorkoutOut
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		_, err := q.UpdateWorkout(ctx, store.UpdateWorkoutParams{
			ID: id, Day: w.Day, Title: w.Title, Notes: w.Notes, DurationMin: i32(w.DurationMin), Kcal: i32(w.Kcal), Now: s.clock.Now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errWorkoutNotFound
		}
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		if err := q.DeleteWorkoutExercises(ctx, id); err != nil {
			return fmt.Errorf("clear exercises: %w", err)
		}
		if err := s.saveExercises(ctx, q, id, w.Exercises); err != nil {
			return err
		}
		out, err = s.get(ctx, q, id)
		return err
	})
	if err != nil {
		return WorkoutOut{}, fmt.Errorf("workout update %d: %w", id, err)
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := store.New(s.pool).DeleteWorkout(ctx, id)
	if err != nil {
		return fmt.Errorf("workout delete %d: %w", id, err)
	}
	if n == 0 {
		return errWorkoutNotFound
	}
	return nil
}

// saveExercises пишет упражнения тренировки. Упражнение ищется в справочнике по имени без учёта
// регистра; нового нет — создаётся (get-or-create, как группы из тегов): в следующий раз оно в подсказках.
func (s *Service) saveExercises(ctx context.Context, q *store.Queries, workoutID int64, list []EntryInput) error {
	for pos, e := range list {
		ex, err := q.GetExerciseByName(ctx, e.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			ex, err = q.CreateExercise(ctx, store.CreateExerciseParams{Name: e.Name, Now: s.clock.Now()})
		}
		if err != nil {
			return fmt.Errorf("exercise %q: %w", e.Name, err)
		}
		if err := q.InsertWorkoutExercise(ctx, store.InsertWorkoutExerciseParams{
			WorkoutID: workoutID, ExerciseID: ex.ID, Position: int16(pos), Sets: int16(e.Sets), Reps: int32(e.Reps), Weight: e.Weight,
		}); err != nil {
			return fmt.Errorf("insert exercise: %w", err)
		}
	}
	return nil
}

// assemble добавляет к тренировкам упражнения — одним запросом на весь список, без N+1.
func (s *Service) assemble(ctx context.Context, q *store.Queries, rows []store.Workout) ([]WorkoutOut, error) {
	out := make([]WorkoutOut, len(rows))
	idx := make(map[int64]int, len(rows))
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i], idx[r.ID] = r.ID, i
		out[i] = WorkoutOut{
			ID: r.ID, Day: clock.FormatDate(r.Day), Title: r.Title, Notes: r.Notes, DurationMin: intp(r.DurationMin), Kcal: intp(r.Kcal),
			Exercises: []Entry{}, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	entries, err := q.WorkoutExercisesOf(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("workout exercises: %w", err)
	}
	for _, e := range entries {
		w := &out[idx[e.WorkoutID]]
		entry := Entry{ExerciseID: e.ExerciseID, Name: e.Name, Sets: int(e.Sets), Reps: int(e.Reps), Weight: e.Weight}
		entry.Tonnage = Tonnage(entry.Sets, entry.Reps, entry.Weight)
		w.Tonnage += entry.Tonnage
		w.Exercises = append(w.Exercises, entry)
	}
	return out, nil
}

// Titles — подсказки названия тренировки: частые сверху.
func (s *Service) Titles(ctx context.Context, query string) ([]string, error) {
	out, err := store.New(s.pool).Titles(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("titles: %w", err)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// ---------- справочник упражнений ----------

// Exercises — справочник (частые сверху) с тем, как делали в прошлый раз; query — подстрока имени.
func (s *Service) Exercises(ctx context.Context, query string) ([]Exercise, error) {
	q := store.New(s.pool)
	rows, err := q.ListExercises(ctx)
	if err != nil {
		return nil, fmt.Errorf("exercises: %w", err)
	}
	lastRows, err := q.LastExerciseEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("last entries: %w", err)
	}
	last := make(map[int64]*Last, len(lastRows))
	for _, l := range lastRows {
		last[l.ExerciseID] = &Last{Sets: int(l.Sets), Reps: int(l.Reps), Weight: l.Weight}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]Exercise, 0, len(rows))
	for _, r := range rows {
		if query != "" && !strings.Contains(strings.ToLower(r.Name), query) {
			continue
		}
		e := Exercise{ID: r.ID, Name: r.Name, Uses: int(r.Uses), Last: last[r.ID]}
		// max(day) по LEFT JOIN: у неиспользованного упражнения NULL, поэтому sqlc дал interface{} — pgx кладёт time.Time или nil.
		if d, ok := r.LastDay.(time.Time); ok {
			e.LastDay = new(clock.FormatDate(d))
		}
		out = append(out, e)
	}
	return out, nil
}

// RenameExercise — новое имя упражнения (во всех тренировках сразу: они ссылаются на справочник).
func (s *Service) RenameExercise(ctx context.Context, id int64, raw string) (Exercise, error) {
	name, err := ExerciseName(raw)
	if err != nil {
		return Exercise{}, err
	}
	r, err := store.New(s.pool).RenameExercise(ctx, store.RenameExerciseParams{ID: id, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return Exercise{}, errExerciseNotFound
	}
	if db.IsUniqueViolation(err) {
		return Exercise{}, httpx.Conflict("duplicate_exercise", "an exercise with this name already exists")
	}
	if err != nil {
		return Exercise{}, fmt.Errorf("exercise rename %d: %w", id, err)
	}
	return Exercise{ID: r.ID, Name: r.Name}, nil
}

// DeleteExercise — только неиспользованное (иначе 409 exercise_in_use: в тренировках на него ссылки).
func (s *Service) DeleteExercise(ctx context.Context, id int64) error {
	q := store.New(s.pool)
	used, err := q.ExerciseUsed(ctx, id)
	if err != nil {
		return fmt.Errorf("exercise used: %w", err)
	}
	if used {
		return httpx.Conflict("exercise_in_use", "the exercise is used in workouts")
	}
	n, err := q.DeleteExercise(ctx, id)
	if err != nil {
		return fmt.Errorf("exercise delete %d: %w", id, err)
	}
	if n == 0 {
		return errExerciseNotFound
	}
	return nil
}

// ---------- мост int ↔ int32 (в БД INT, в API — обычные числа) ----------

func i32(p *int) *int32 {
	if p == nil {
		return nil
	}
	v := int32(*p)
	return &v
}

func intp(p *int32) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

// ---------- еда (C2) ----------

// FoodDay — еда за день и, рядом, сожжённое на тренировках (§18.4 C4: без «баланса» — часы дают только активные ккал).
type FoodDay struct {
	Day        string `json:"day"`
	Meals      []Meal `json:"meals"`
	EatenKcal  int    `json:"eatenKcal"`
	BurnedKcal int    `json:"burnedKcal"`
}

func (s *Service) Food(ctx context.Context, dayS string) (FoodDay, error) {
	day, err := days.Parse(dayS)
	if err != nil {
		return FoodDay{}, err
	}
	return s.food(ctx, store.New(s.pool), day)
}

func (s *Service) food(ctx context.Context, q *store.Queries, day time.Time) (FoodDay, error) {
	rows, err := q.ListMeals(ctx, day)
	if err != nil {
		return FoodDay{}, fmt.Errorf("meals: %w", err)
	}
	burned, err := q.BurnedKcal(ctx, day)
	if err != nil {
		return FoodDay{}, fmt.Errorf("burned: %w", err)
	}
	out := FoodDay{Day: clock.FormatDate(day), Meals: make([]Meal, len(rows)), BurnedKcal: int(burned)}
	for i, r := range rows {
		out.Meals[i] = Meal{Text: r.Text, Kcal: intp(r.Kcal)}
	}
	out.EatenKcal = SumKcal(out.Meals)
	return out, nil
}

// SaveFood заменяет еду за день списком (будущие дни — нельзя: ещё не съедено).
func (s *Service) SaveFood(ctx context.Context, dayS string, meals []Meal) (FoodDay, error) {
	day, err := days.Parse(dayS)
	if err != nil {
		return FoodDay{}, err
	}
	if err := days.CheckWritable(day, clock.TodayDate(s.clock)); err != nil {
		return FoodDay{}, err
	}
	meals, err = NormalizeMeals(meals)
	if err != nil {
		return FoodDay{}, err
	}
	var out FoodDay
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.DeleteMeals(ctx, day); err != nil {
			return err
		}
		now := s.clock.Now()
		for i, m := range meals {
			if err := q.InsertMeal(ctx, store.InsertMealParams{Day: day, Position: int16(i), Text: m.Text, Kcal: i32(m.Kcal), Now: now}); err != nil {
				return err
			}
		}
		out, err = s.food(ctx, q, day)
		return err
	})
	if err != nil {
		return FoodDay{}, fmt.Errorf("save food %s: %w", dayS, err)
	}
	return out, nil
}

// MealSuggestions — «что съел» из прошлого (частые сверху) с ккал последней такой записи.
func (s *Service) MealSuggestions(ctx context.Context, query string) ([]Meal, error) {
	rows, err := store.New(s.pool).MealSuggestions(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("meal suggestions: %w", err)
	}
	// DISTINCT ON требует ORDER BY по тексту — частоту сортируем здесь.
	slices.SortStableFunc(rows, func(a, b store.MealSuggestionsRow) int { return cmp.Compare(b.Uses, a.Uses) })
	out := make([]Meal, 0, min(len(rows), maxSuggestions))
	for _, r := range rows[:min(len(rows), maxSuggestions)] {
		out = append(out, Meal{Text: r.Text, Kcal: intp(r.Kcal)})
	}
	return out, nil
}

const maxSuggestions = 15

// ---------- статистика и прогресс (C2) ----------

// Stats — окно ?from=&to= (по умолчанию последние 7 дней).
func (s *Service) Stats(ctx context.Context, fromS, toS string) (Stats, error) {
	from, to, err := days.ParseRange(fromS, toS, clock.TodayDate(s.clock), 7)
	if err != nil {
		return Stats{}, err
	}
	q := store.New(s.pool)
	rows, err := q.ListWorkouts(ctx, store.ListWorkoutsParams{FromDay: from, ToDay: to})
	if err != nil {
		return Stats{}, fmt.Errorf("workouts: %w", err)
	}
	ws, err := s.assemble(ctx, q, rows)
	if err != nil {
		return Stats{}, err
	}
	workouts := make([]WorkoutStat, len(ws))
	for i, w := range ws {
		workouts[i] = WorkoutStat{Day: rows[i].Day, Kcal: w.Kcal, DurationMin: w.DurationMin}
		for _, e := range w.Exercises {
			workouts[i].Entries = append(workouts[i].Entries, EntryStat{ExerciseID: e.ExerciseID, Name: e.Name, Sets: e.Sets, Reps: e.Reps, Weight: e.Weight})
		}
	}
	mealRows, err := q.MealsBetween(ctx, store.MealsBetweenParams{FromDay: from, ToDay: to})
	if err != nil {
		return Stats{}, fmt.Errorf("meals: %w", err)
	}
	meals := make([]MealStat, len(mealRows))
	for i, m := range mealRows {
		meals[i] = MealStat{Day: m.Day, Kcal: intp(m.Kcal)}
	}
	return Summarize(from, to, workouts, meals), nil
}

const defaultProgressDays = 180

// ExerciseProgress — упражнение по дням окна (по умолчанию полгода): макс. вес, тоннаж, подходы, повторы.
func (s *Service) ExerciseProgress(ctx context.Context, id int64, fromS, toS string) (Exercise, []ProgressPoint, error) {
	from, to, err := days.ParseRange(fromS, toS, clock.TodayDate(s.clock), defaultProgressDays)
	if err != nil {
		return Exercise{}, nil, err
	}
	q := store.New(s.pool)
	ex, err := q.GetExercise(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Exercise{}, nil, errExerciseNotFound
	}
	if err != nil {
		return Exercise{}, nil, fmt.Errorf("exercise %d: %w", id, err)
	}
	rows, err := q.ExerciseHistory(ctx, store.ExerciseHistoryParams{ExerciseID: id, FromDay: from, ToDay: to})
	if err != nil {
		return Exercise{}, nil, fmt.Errorf("history: %w", err)
	}
	hist := make([]HistoryRow, len(rows))
	for i, r := range rows {
		hist[i] = HistoryRow{Day: r.Day, Sets: int(r.Sets), Reps: int(r.Reps), Weight: r.Weight}
	}
	return Exercise{ID: ex.ID, Name: ex.Name}, Progress(hist), nil
}
