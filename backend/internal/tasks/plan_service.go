package tasks

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
	"lexis/internal/tasks/store"
)

// PlanItem — пункт плана: дело (с вычисленными полями) и сколько его блоков на этот день.
type PlanItem struct {
	Task   Task `json:"task"`
	Start  int  `json:"start"` // минуты от полуночи
	Blocks int  `json:"blocks"`
}

// TopItem — предложение на день с оценкой и причинами (коды для i18n).
type TopItem struct {
	Task    Task     `json:"task"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

// Plan — ответ GET/PUT /api/tasks/plan/{day}.
type Plan struct {
	Day         string     `json:"day"`
	Today       string     `json:"today"`
	Editable    bool       `json:"editable"` // прошлые дни — только чтение
	Items       []PlanItem `json:"items"`
	TotalBlocks int        `json:"totalBlocks"`
	Settings    Settings   `json:"settings"`
	Top         []TopItem  `json:"top"`       // пусто для прошлых дней
	Leftovers   []Task     `json:"leftovers"` // «не доделал»: из планов прошлой недели, ещё открытые и не в этом дне
}

var errPastDay = httpx.BadRequest("past_day", "a past day plan is read-only")

// Plan — план дня + топ-5 и «не доделал» (для сегодня и будущих дней).
func (s *Service) Plan(ctx context.Context, dayS string) (Plan, error) {
	day := clock.TodayDate(s.clock)
	if dayS != "today" {
		var err error
		day, err = days.Parse(dayS)
		if err != nil {
			return Plan{}, err
		}
	}
	p, err := s.plan(ctx, store.New(s.pool), day)
	if err != nil {
		return Plan{}, fmt.Errorf("plan %s: %w", dayS, err)
	}
	return p, nil
}

func (s *Service) plan(ctx context.Context, q *store.Queries, day time.Time) (Plan, error) {
	sn, err := loadSnapshot(ctx, q)
	if err != nil {
		return Plan{}, err
	}
	rows, err := q.ListPlanItems(ctx, day)
	if err != nil {
		return Plan{}, fmt.Errorf("plan items: %w", err)
	}
	settings := loadSettings(ctx, q)
	today := clock.TodayDate(s.clock)
	p := Plan{
		Day: clock.FormatDate(day), Today: clock.FormatDate(today), Editable: !day.Before(today), Settings: settings,
		Items: make([]PlanItem, 0, len(rows)), Top: []TopItem{}, Leftovers: []Task{},
	}
	planned := make(map[int64]bool, len(rows))
	for _, r := range rows {
		t, ok := sn.task(r.TaskID)
		if !ok {
			continue // удалено каскадом между запросами
		}
		planned[r.TaskID] = true
		p.Items = append(p.Items, PlanItem{Task: t, Start: int(r.StartMin), Blocks: int(r.Blocks)})
		p.TotalBlocks += int(r.Blocks)
	}
	if !p.Editable {
		return p, nil
	}

	// Топ-5: невыполненные листья, которых ещё нет в этом дне.
	var cands []Candidate
	for _, r := range sn.rows {
		if a := sn.agg[r.ID]; a.Leaf && !a.Done() && !planned[r.ID] {
			cands = append(cands, Candidate{ID: r.ID, Importance: int(r.Importance), Blocks: int(r.Blocks), Deadline: r.Deadline, CreatedAt: r.CreatedAt})
		}
	}
	for _, sc := range Top(cands, day, TopN) {
		t, _ := sn.task(sc.ID)
		p.Top = append(p.Top, TopItem{Task: t, Score: sc.Score, Reasons: sc.Reasons})
	}

	// «Не доделал»: планы недели до этого дня; свежие — первыми, каждое дело один раз.
	past, err := q.ListPlanItemsRange(ctx, store.ListPlanItemsRangeParams{FromDay: day.AddDate(0, 0, -LeftoverDays), ToDay: day})
	if err != nil {
		return Plan{}, fmt.Errorf("past plans: %w", err)
	}
	for _, r := range past {
		a := sn.agg[r.TaskID]
		if planned[r.TaskID] || !a.Leaf || a.Done() {
			continue
		}
		planned[r.TaskID] = true // и дубли из разных дней отсекаются тем же
		t, _ := sn.task(r.TaskID)
		p.Leftovers = append(p.Leftovers, t)
	}
	return p, nil
}

// SavePlan заменяет план дня целиком. В план можно добавить только невыполненный лист;
// дело, выполненное уже после планирования, в своём дне остаётся.
func (s *Service) SavePlan(ctx context.Context, dayS string, in PlanInput) (Plan, error) {
	day, err := s.writableDay(dayS)
	if err != nil {
		return Plan{}, err
	}
	if err := in.Normalize(); err != nil {
		return Plan{}, err
	}
	return s.changePlan(ctx, day, func(q *store.Queries) error {
		sn, err := loadSnapshot(ctx, q)
		if err != nil {
			return err
		}
		old, err := q.ListPlanItems(ctx, day)
		if err != nil {
			return fmt.Errorf("plan items: %w", err)
		}
		wasPlanned := make(map[int64]bool, len(old))
		for _, r := range old {
			wasPlanned[r.TaskID] = true
		}
		for _, it := range in.Items {
			a, ok := sn.agg[it.TaskID]
			switch {
			case !ok:
				return httpx.BadRequest("invalid_task", fmt.Sprintf("task %d not found", it.TaskID))
			case !a.Leaf:
				return httpx.BadRequest("task_not_leaf", "a task with steps cannot be planned: plan its steps")
			case a.Done() && !wasPlanned[it.TaskID]:
				return httpx.BadRequest("task_done", "a done task cannot be added to a plan")
			}
		}
		if err := q.DeletePlan(ctx, day); err != nil {
			return fmt.Errorf("delete plan: %w", err)
		}
		items := slices.Clone(in.Items) // position — порядок по времени (для бэкапа и стабильной выдачи)
		slices.SortFunc(items, func(a, b PlanItemInput) int { return cmp.Compare(a.Start, b.Start) })
		for i, it := range items {
			if err := q.InsertPlanItem(ctx, store.InsertPlanItemParams{
				Day: day, TaskID: it.TaskID, Position: int16(i), Blocks: int16(it.Blocks), StartMin: int16(it.Start),
			}); err != nil {
				return fmt.Errorf("insert plan item: %w", err)
			}
		}
		return nil
	})
}

// AddPlanTask добавляет открытый лист в свободный слот текущего плана.
func (s *Service) AddPlanTask(ctx context.Context, dayS string, taskID int64) (Plan, error) {
	day, err := s.writableDay(dayS)
	if err != nil {
		return Plan{}, err
	}
	if taskID <= 0 {
		return Plan{}, httpx.BadRequest("invalid_task", "taskId must be a positive id")
	}
	return s.changePlan(ctx, day, func(q *store.Queries) error {
		task, err := q.GetTaskForUpdate(ctx, taskID)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.BadRequest("invalid_task", "task not found")
		}
		if err != nil {
			return fmt.Errorf("lock task: %w", err)
		}
		hasChildren, err := q.HasChildren(ctx, taskID)
		if err != nil {
			return fmt.Errorf("check task children: %w", err)
		}
		if hasChildren {
			return httpx.BadRequest("task_not_leaf", "a task with steps cannot be planned: plan its steps")
		}
		if task.DoneAt != nil {
			return httpx.BadRequest("task_done", "a done task cannot be added to a plan")
		}
		return s.appendPlanItem(ctx, q, day, task.ID, int(task.Blocks))
	})
}

// CreatePlanTask создаёт дело в выбранной колонке и добавляет его в план одной транзакцией.
func (s *Service) CreatePlanTask(ctx context.Context, dayS string, in TaskInput) (Plan, error) {
	day, err := s.writableDay(dayS)
	if err != nil {
		return Plan{}, err
	}
	f, err := in.Normalize()
	if err != nil {
		return Plan{}, err
	}
	return s.changePlan(ctx, day, func(q *store.Queries) error {
		task, err := s.createTask(ctx, q, in.ColumnID, f)
		if err != nil {
			return err
		}
		return s.appendPlanItem(ctx, q, day, task.ID, task.Blocks)
	})
}

func (s *Service) appendPlanItem(ctx context.Context, q *store.Queries, day time.Time, taskID int64, blocks int) error {
	rows, err := q.ListPlanItems(ctx, day)
	if err != nil {
		return fmt.Errorf("plan items: %w", err)
	}
	slots := make([]PlanItemInput, 0, len(rows))
	for _, row := range rows {
		if row.TaskID == taskID {
			return httpx.Conflict("plan_task_exists", "task is already planned for this day")
		}
		slots = append(slots, PlanItemInput{TaskID: row.TaskID, Start: int(row.StartMin), Blocks: int(row.Blocks)})
	}
	if len(slots) >= MaxPlanItems {
		return httpx.BadRequest("too_many_items", fmt.Sprintf("a day plan has at most %d items", MaxPlanItems))
	}
	settings := loadSettings(ctx, q)
	from := settings.DayStart
	now := s.clock.Now()
	if day.Equal(clock.DateOf(now)) {
		nowMin := ((now.Hour()*60 + now.Minute() + BlockMin - 1) / BlockMin) * BlockMin
		from = max(from, min(nowMin, settings.DayEnd-BlockMin))
	}
	start, ok := firstFree(slots, blocks, from, settings.DayEnd)
	if !ok {
		return httpx.BadRequest("plan_no_slot", "no free slot for the task")
	}
	if err := q.InsertPlanItem(ctx, store.InsertPlanItemParams{
		Day: day, TaskID: taskID, Position: int16(len(rows)), Blocks: int16(blocks), StartMin: int16(start),
	}); err != nil {
		return fmt.Errorf("insert plan item: %w", err)
	}
	return nil
}

func (s *Service) writableDay(dayS string) (time.Time, error) {
	day, err := days.Parse(dayS)
	if err != nil {
		return time.Time{}, err
	}
	if day.Before(clock.TodayDate(s.clock)) {
		return time.Time{}, errPastDay
	}
	return day, nil
}

func (s *Service) changePlan(ctx context.Context, day time.Time, change func(*store.Queries) error) (Plan, error) {
	var p Plan
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockPlanDay(ctx, day); err != nil {
			return fmt.Errorf("lock plan day: %w", err)
		}
		if day.Before(clock.TodayDate(s.clock)) {
			return errPastDay
		}
		if err := change(q); err != nil {
			return err
		}
		var err error
		p, err = s.plan(ctx, q, day)
		return err
	})
	if err != nil {
		return Plan{}, fmt.Errorf("save plan %s: %w", clock.FormatDate(day), err)
	}
	return p, nil
}

// ---------- настройки ----------

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	return loadSettings(ctx, store.New(s.pool)), nil
}

func (s *Service) UpdateSettings(ctx context.Context, p SettingsPatch) (Settings, error) {
	var out Settings
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		next, err := p.Apply(loadSettings(ctx, q))
		if err != nil {
			return err
		}
		for key, v := range map[string]int{SettingDayStart: next.DayStart, SettingDayEnd: next.DayEnd, SettingDayLimit: next.DayLimit} {
			raw, _ := json.Marshal(v) // int маршалится всегда
			if err := q.PutSetting(ctx, store.PutSettingParams{Key: key, Value: raw}); err != nil {
				return fmt.Errorf("put %s: %w", key, err)
			}
		}
		out = next
		return nil
	})
	if err != nil {
		return Settings{}, fmt.Errorf("update game settings: %w", err)
	}
	return out, nil
}

// loadSettings читает ключи из settings; нет ключа или мусор (в т.ч. после ручной правки
// или старого бэкапа) — значения по умолчанию, как у new_words_limit в training.
func loadSettings(ctx context.Context, q *store.Queries) Settings {
	s := DefaultSettings
	for key, dst := range map[string]*int{SettingDayStart: &s.DayStart, SettingDayEnd: &s.DayEnd, SettingDayLimit: &s.DayLimit} {
		raw, err := q.GetSetting(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		var v int
		if err == nil {
			err = json.Unmarshal(raw, &v)
		}
		if err != nil {
			slog.WarnContext(ctx, "tasks: bad setting, using default", "key", key, "raw", string(raw), "err", err)
			continue
		}
		*dst = v
	}
	if _, err := (SettingsPatch{}).Apply(s); err != nil {
		slog.WarnContext(ctx, "tasks: inconsistent day settings, using defaults", "settings", s)
		return DefaultSettings
	}
	return s
}
