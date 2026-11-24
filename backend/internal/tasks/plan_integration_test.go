//go:build integration

// Распорядок дня (§18.3, B3) на живом Postgres: план, топ-5, «не доделал», настройки.
package tasks

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func planTitles(p Plan) []string {
	out := make([]string, len(p.Items))
	for i, it := range p.Items {
		out[i] = it.Task.Title
	}
	return out
}

func TestPlan(t *testing.T) {
	s, clk := newTestService(t) // сегодня — 2026-11-05
	ctx := context.Background()
	col := must(s.CreateColumn(ctx, "Входящие"))
	mk := func(title string, blocks, importance int, deadline *string) Task {
		return must(s.Create(ctx, TaskInput{Title: title, Blocks: blocks, Importance: importance, Deadline: deadline, ColumnID: col.ID}))
	}
	report := mk("Отчёт", 2, 5, new("2026-11-06"))
	paper := mk("Курсовая", 12, 7, new("2026-11-19"))
	closet := mk("Шкаф", 4, 2, nil)
	dream := mk("Мечта", 8, 9, nil)

	p := must(s.Plan(ctx, "2026-11-05"))
	if !p.Editable || len(p.Items) != 0 || p.Settings != DefaultSettings {
		t.Fatalf("empty plan: %+v", p)
	}
	var top []string
	for _, x := range p.Top {
		top = append(top, x.Task.Title)
	}
	// Курсовая 0.542 чуть выше Мечты 0.540 (в SPEC обе округлены до 0.54).
	if want := []string{"Отчёт", "Курсовая", "Мечта", "Шкаф"}; !reflect.DeepEqual(top, want) {
		t.Errorf("top = %v, want %v", top, want)
	}

	p = must(s.SavePlan(ctx, "2026-11-05", PlanInput{Items: []PlanItemInput{{TaskID: dream.ID, Start: 540, Blocks: 4}, {TaskID: report.ID, Start: 660, Blocks: 2}}}))
	if got := planTitles(p); !reflect.DeepEqual(got, []string{"Мечта", "Отчёт"}) || p.TotalBlocks != 6 {
		t.Errorf("saved plan: %v total %d", got, p.TotalBlocks)
	}
	// Порядок в ответе — по времени, а не по порядку в запросе.
	p = must(s.SavePlan(ctx, "2026-11-05", PlanInput{Items: []PlanItemInput{{TaskID: dream.ID, Start: 900, Blocks: 4}, {TaskID: report.ID, Start: 600, Blocks: 2}}}))
	if got := planTitles(p); !reflect.DeepEqual(got, []string{"Отчёт", "Мечта"}) || p.Items[0].Start != 600 || p.Items[1].Start != 900 {
		t.Errorf("plan by time: %v %+v", got, p.Items)
	}
	_, err := s.SavePlan(ctx, "2026-11-05", PlanInput{Items: []PlanItemInput{{TaskID: dream.ID, Start: 600, Blocks: 4}, {TaskID: report.ID, Start: 630, Blocks: 2}}})
	assertCode(t, err, 400, "plan_overlap")
	for _, x := range p.Top {
		if x.Task.ID == dream.ID || x.Task.ID == report.ID {
			t.Errorf("planned task %q is still in top", x.Task.Title)
		}
	}

	// Выполненное после планирования остаётся в своём дне и пересохраняется; новое выполненное — нельзя.
	must(s.Update(ctx, report.ID, Patch{Done: new(true)}))
	must(s.Update(ctx, closet.ID, Patch{Done: new(true)}))
	p = must(s.SavePlan(ctx, "2026-11-05", PlanInput{Items: []PlanItemInput{{TaskID: report.ID, Start: 540, Blocks: 2}, {TaskID: dream.ID, Start: 600, Blocks: 4}}}))
	if !p.Items[0].Task.Done {
		t.Errorf("done task must stay in its plan: %+v", p.Items[0])
	}
	_, err = s.SavePlan(ctx, "2026-11-05", PlanInput{Items: []PlanItemInput{{TaskID: closet.ID, Blocks: 1}}})
	assertCode(t, err, 400, "task_done")
	_, err = s.SavePlan(ctx, "2026-11-05", PlanInput{Items: []PlanItemInput{{TaskID: 999, Blocks: 1}}})
	assertCode(t, err, 400, "invalid_task")

	// Разбили дело на шаги — оно уходит из планов, а само больше не планируется.
	must(s.SavePlan(ctx, "2026-11-06", PlanInput{Items: []PlanItemInput{{TaskID: paper.ID, Start: 540, Blocks: 6}}}))
	must(s.Split(ctx, paper.ID, SplitInput{Steps: []Step{{Title: "Глава 1", Blocks: 6}}}))
	if p := must(s.Plan(ctx, "2026-11-06")); len(p.Items) != 0 {
		t.Errorf("split task must leave plans: %v", planTitles(p))
	}
	_, err = s.SavePlan(ctx, "2026-11-06", PlanInput{Items: []PlanItemInput{{TaskID: paper.ID, Blocks: 1}}})
	assertCode(t, err, 400, "task_not_leaf")

	// Прошлый день — только чтение, без топа.
	clk.Set(testNow.AddDate(0, 0, 1)) // завтра стало сегодня: 2026-11-06
	_, err = s.SavePlan(ctx, "2026-11-05", PlanInput{})
	assertCode(t, err, 400, "past_day")
	past := must(s.Plan(ctx, "2026-11-05"))
	if past.Editable || len(past.Top) != 0 || len(past.Items) != 2 {
		t.Errorf("past plan: %+v", past)
	}
	// «Не доделал»: «Мечта» осталась открытой во вчерашнем плане, «Отчёт» сделан — не предлагаем.
	today := must(s.Plan(ctx, "2026-11-06"))
	if len(today.Leftovers) != 1 || today.Leftovers[0].ID != dream.ID {
		t.Errorf("leftovers = %+v", today.Leftovers)
	}
	must(s.SavePlan(ctx, "2026-11-06", PlanInput{Items: []PlanItemInput{{TaskID: dream.ID, Start: 540, Blocks: 4}}}))
	if today := must(s.Plan(ctx, "2026-11-06")); len(today.Leftovers) != 0 {
		t.Errorf("leftover already planned today: %+v", today.Leftovers)
	}
}

func TestSettings(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	if got := must(s.Settings(ctx)); got != DefaultSettings {
		t.Errorf("defaults = %+v", got)
	}
	got := must(s.UpdateSettings(ctx, SettingsPatch{DayStart: new(8 * 60), DayLimit: new(14)}))
	if want := (Settings{DayStart: 480, DayEnd: 1380, DayLimit: 14}); got != want {
		t.Errorf("updated = %+v, want %+v", got, want)
	}
	if again := must(s.Settings(ctx)); again != got {
		t.Errorf("reread = %+v", again)
	}
	_, err := s.UpdateSettings(ctx, SettingsPatch{DayEnd: new(7 * 60)})
	assertCode(t, err, 400, "invalid_day_bounds")
	if p := must(s.Plan(ctx, "2026-11-05")); p.Settings != got {
		t.Errorf("plan settings = %+v", p.Settings)
	}
}

func TestCalendarMonthBoundaries(t *testing.T) {
	s, clk := newTestService(t)
	ctx := context.Background()
	clk.Set(time.Date(2026, 10, 31, 12, 0, 0, 0, testNow.Location()))
	col := must(s.CreateColumn(ctx, "Планы"))
	task := must(s.Create(ctx, TaskInput{Title: "Отчёт", Blocks: 2, Importance: 5, Deadline: new("2026-12-02"), ColumnID: col.ID}))
	for _, day := range []string{"2026-10-31", "2026-11-01", "2026-11-30", "2026-12-01"} {
		must(s.AddPlanTask(ctx, day, task.ID))
	}
	_, err := s.AddPlanTask(ctx, "2026-11-30", task.ID)
	assertCode(t, err, 409, "plan_task_exists")
	deadlineOnly := must(s.Create(ctx, TaskInput{Title: "Только дедлайн", Blocks: 1, Importance: 5, Deadline: new("2026-11-15"), ColumnID: col.ID}))

	// В UTC ещё октябрь, в Lexis уже ноябрь.
	clk.Set(time.Date(2026, 10, 31, 22, 0, 0, 0, time.UTC).In(testNow.Location()))
	calendar := must(s.Calendar(ctx, "", ""))
	want := []CalendarItem{
		{Day: "2026-11-01", TaskID: task.ID, Start: 540, Title: "Отчёт"},
		{Day: "2026-11-30", TaskID: task.ID, Start: 540, Title: "Отчёт"},
	}
	if calendar.From != "2026-11-01" || calendar.To != "2026-11-30" || calendar.Today != "2026-11-01" || !reflect.DeepEqual(calendar.Items, want) {
		t.Fatalf("calendar = %+v, want November dates and items %+v", calendar, want)
	}
	if explicit := must(s.Calendar(ctx, "2026-11-01", "2026-11-30")); !reflect.DeepEqual(explicit, calendar) {
		t.Errorf("explicit November range = %+v, want %+v", explicit, calendar)
	}
	past := must(s.Plan(ctx, "2026-10-31"))
	if past.Editable || len(past.Items) != 1 || past.Items[0].Task.ID != task.ID {
		t.Errorf("past plan = %+v, want read-only plan with task %d", past, task.ID)
	}
	_, err = s.AddPlanTask(ctx, past.Day, deadlineOnly.ID)
	assertCode(t, err, 400, "past_day")
	if today := must(s.Plan(ctx, "today")); today.Day != calendar.Today || !today.Editable {
		t.Errorf("today plan = %+v, want editable %s", today, calendar.Today)
	}
}

func TestCreatePlanTaskRollsBackWhenDayIsFull(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	col := must(s.CreateColumn(ctx, "Планы"))
	target := must(s.CreateColumn(ctx, "Учёба"))
	day := "2026-11-09"
	long := must(s.Create(ctx, TaskInput{Title: "Первая", Blocks: 40, Importance: 5, ColumnID: col.ID}))
	short := must(s.Create(ctx, TaskInput{Title: "Вторая", Blocks: 8, Importance: 5, ColumnID: col.ID}))
	longSlot := PlanItemInput{TaskID: long.ID, Start: 0, Blocks: 40}
	original := must(s.SavePlan(ctx, day, PlanInput{Items: []PlanItemInput{longSlot, {TaskID: short.ID, Start: 1200, Blocks: 8}}}))
	tasks := must(s.Tasks(ctx, FilterAll))
	in := TaskInput{Title: "Новая задача", Blocks: 2, Importance: 6, ColumnID: target.ID, Deadline: new("2026-11-13"), Note: "  Текст #учёба \n"}
	_, err := s.CreatePlanTask(ctx, day, in)
	assertCode(t, err, 400, "plan_no_slot")
	if got := must(s.Tasks(ctx, FilterAll)); !reflect.DeepEqual(got, tasks) {
		t.Errorf("failed creation changed the board: %+v", got)
	}
	if got := must(s.Plan(ctx, day)); !reflect.DeepEqual(got, original) {
		t.Errorf("failed creation changed the plan: %+v", got)
	}

	reduced := must(s.SavePlan(ctx, day, PlanInput{Items: []PlanItemInput{longSlot}}))
	created := must(s.CreatePlanTask(ctx, day, in))
	if len(created.Items) != 2 || !reflect.DeepEqual(created.Items[0], reduced.Items[0]) {
		t.Fatalf("creation must preserve previous items: %+v", created.Items)
	}
	item := created.Items[1]
	if item.Start != 1200 || item.Blocks != 2 || item.Task.ColumnID != target.ID ||
		item.Task.Note != in.Note || !reflect.DeepEqual(item.Task.Deadline, in.Deadline) {
		t.Errorf("created item = %+v", item)
	}
	if got := order(t, s, target.ID); !reflect.DeepEqual(got, []int64{item.Task.ID}) {
		t.Errorf("created task missing from target column: %v", got)
	}
}
