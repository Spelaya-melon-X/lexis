//go:build integration

// Интеграционные тесты фокуса и наград (§18.3, B4) на живом Postgres, с настоящим модулем tasks.
package focus

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
	"lexis/internal/platform/httpx"
	"lexis/internal/tasks"
)

var testNow = time.Date(2026, 11, 5, 10, 0, 0, 0, time.FixedZone("MSK", 3*60*60))

// fakeDopamine запоминает записи; full — все квадратики заняты.
type fakeDopamine struct {
	texts []string
	full  bool
}

func (f *fakeDopamine) AddDopamine(_ context.Context, text string) (bool, error) {
	if f.full {
		return false, nil
	}
	f.texts = append(f.texts, text)
	return true, nil
}

type recBus struct{ events []eventbus.Event }

func (b *recBus) Publish(e eventbus.Event) { b.events = append(b.events, e) }

type env struct {
	bus   *recBus
	svc   *Service
	tasks *tasks.Service
	clock *clock.Fixed
	dop   *fakeDopamine
	col   int64
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool := dbtest.New(t)
	clk := clock.NewFixed(testNow)
	bus := &recBus{}
	ts := tasks.NewService(pool, clk, bus)
	dop := &fakeDopamine{}
	col, err := ts.CreateColumn(context.Background(), "Входящие")
	if err != nil {
		t.Fatal(err)
	}
	return env{bus: bus, svc: NewService(pool, clk, ts, dop, bus), tasks: ts, clock: clk, dop: dop, col: col.ID}
}

func (e env) task(t *testing.T, title string, blocks int) tasks.Task {
	t.Helper()
	x, err := e.tasks.Create(context.Background(), tasks.TaskInput{Title: title, Blocks: blocks, Importance: 5, ColumnID: e.col})
	if err != nil {
		t.Fatal(err)
	}
	return x
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

func TestSession_Lifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	report := e.task(t, "Отчёт", 2)
	// В плане на сегодня — 3 блока: это минимум работы, а не объём дела.
	must(e.tasks.SavePlan(ctx, "2026-11-05", tasks.PlanInput{Items: []tasks.PlanItemInput{{TaskID: report.ID, Start: 600, Blocks: 3}}}))

	s := must(e.svc.Start(ctx, report.ID))
	if s.PlannedBlocks != 3 || s.Timeline.Phase != PhaseSetup || s.TaskTitle != "Отчёт" || s.Day != "2026-11-05" {
		t.Fatalf("started: %+v", s)
	}
	other := e.task(t, "Другое", 1)
	_, err := e.svc.Start(ctx, other.ID)
	assertCode(t, err, 409, "focus_active")

	e.clock.Advance(5 * time.Minute)
	s = must(e.svc.Act(ctx, s.ID, ActionWork))
	if s.Timeline.Phase != PhaseWork || s.Timeline.SetupSec != 300 {
		t.Errorf("work: %+v", s.Timeline)
	}
	e.clock.Advance(40 * time.Minute)
	s = must(e.svc.Act(ctx, s.ID, ActionBreak))
	_, err = e.svc.Act(ctx, s.ID, ActionBreak)
	assertCode(t, err, 409, "invalid_phase")
	// Разбить дело с активной сессией нельзя.
	_, err = e.tasks.Split(ctx, report.ID, tasks.SplitInput{Steps: []tasks.Step{{Title: "x", Blocks: 1}}})
	assertCode(t, err, 409, "focus_active")

	e.clock.Advance(3 * time.Minute)
	s = must(e.svc.Distract(ctx, s.ID, DistractionInput{Trigger: " телефон ", GaveIn: true}))
	if len(s.Distractions) != 1 || s.Distractions[0].Trigger != "телефон" || s.Timeline.Phase != PhaseBreak || s.Timeline.BreakSec != 180 {
		t.Errorf("distraction on break: %+v", s)
	}
	s = must(e.svc.Act(ctx, s.ID, ActionResume))
	e.clock.Advance(30 * time.Minute)
	s = must(e.svc.Distract(ctx, s.ID, DistractionInput{Trigger: "чат", GaveIn: false}))
	if act := must(e.svc.Active(ctx)); act == nil || act.ID != s.ID {
		t.Errorf("active = %+v", act)
	}

	s = must(e.svc.Finish(ctx, s.ID, OutcomeDone))
	// Работа 40 + 30 = 70 минут → 2 очка; перерыв 3 из заработанных 10.
	if s.Timeline.Phase != PhaseFinished || s.Timeline.WorkSec != 70*60 || s.Timeline.BreakSec != 180 || s.Points != 2 || *s.Outcome != "done" {
		t.Errorf("finished: %+v points %d", s.Timeline, s.Points)
	}
	if got := must(e.tasks.Get(ctx, report.ID)); !got.Done {
		t.Error("«Я закончил» must mark the task done")
	}
	finishedAt := testNow.Add(78 * time.Minute).UTC()
	want := []eventbus.Event{
		eventbus.FocusFinished{SessionID: s.ID, WorkSec: 70 * 60, Outcome: "done", At: finishedAt},
		eventbus.TaskCompleted{TaskID: report.ID, At: finishedAt},
	}
	if !reflect.DeepEqual(utcEvents(e.bus.events), want) {
		t.Errorf("events = %+v\nwant %+v", e.bus.events, want)
	}
	if act := must(e.svc.Active(ctx)); act != nil {
		t.Errorf("no active session expected, got %+v", act)
	}
	_, err = e.svc.Finish(ctx, s.ID, OutcomeStopped)
	assertCode(t, err, 409, "focus_finished")
	if got := must(e.svc.Triggers(ctx, "")); !reflect.DeepEqual(got, []string{"чат", "телефон"}) && !reflect.DeepEqual(got, []string{"телефон", "чат"}) {
		t.Errorf("triggers = %v", got)
	}
	if got := must(e.svc.Triggers(ctx, "ТЕЛ")); !reflect.DeepEqual(got, []string{"телефон"}) {
		t.Errorf("triggers by query = %v", got)
	}
	// Сессия без плана — минимум = объём дела; закрылась во время перерыва — перерыв закрыт.
	s2 := must(e.svc.Start(ctx, other.ID))
	if s2.PlannedBlocks != 1 {
		t.Errorf("planned blocks without plan = %d", s2.PlannedBlocks)
	}
	_, err = e.svc.Finish(ctx, s2.ID, OutcomeDone)
	assertCode(t, err, 409, "invalid_phase") // «закончил» до начала работы — нельзя
	must(e.svc.Act(ctx, s2.ID, ActionWork))
	e.clock.Advance(20 * time.Minute)
	must(e.svc.Act(ctx, s2.ID, ActionBreak))
	e.clock.Advance(5 * time.Minute)
	s2 = must(e.svc.Finish(ctx, s2.ID, OutcomeStopped))
	if s2.Breaks[0].EndedAt == nil || s2.Points != 0 || must(e.tasks.Get(ctx, other.ID)).Done {
		t.Errorf("stopped on a break: %+v", s2)
	}
}

func TestSession_AbandonAndRules(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	x := e.task(t, "Дело", 2)
	s := must(e.svc.Start(ctx, x.ID))
	_, err := e.svc.Finish(ctx, s.ID, OutcomeStopped)
	assertCode(t, err, 409, "invalid_phase")
	s = must(e.svc.Finish(ctx, s.ID, OutcomeAbandoned))
	if *s.Outcome != "abandoned" || s.Points != 0 {
		t.Errorf("abandoned: %+v", s)
	}
	if len(e.bus.events) != 0 {
		t.Errorf("abandoned session must not publish: %+v", e.bus.events)
	}

	_, err = e.svc.Start(ctx, 999)
	assertCode(t, err, 400, "invalid_task")
	must(e.tasks.Split(ctx, x.ID, tasks.SplitInput{Steps: []tasks.Step{{Title: "шаг", Blocks: 1}}}))
	_, err = e.svc.Start(ctx, x.ID)
	assertCode(t, err, 400, "task_not_leaf")
	done := e.task(t, "Сделано", 1)
	must(e.tasks.Update(ctx, done.ID, tasks.Patch{Done: new(true)}))
	_, err = e.svc.Start(ctx, done.ID)
	assertCode(t, err, 400, "task_done")
	_, err = e.svc.Act(ctx, 999, ActionWork)
	assertCode(t, err, 404, "not_found")
}

func TestRewardsAndPoints(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	movie := must(e.svc.CreateReward(ctx, RewardInput{Title: " Кино ", Price: 2}))
	cake := must(e.svc.CreateReward(ctx, RewardInput{Title: "Торт", Price: 50}))
	if movie.Title != "Кино" {
		t.Errorf("reward: %+v", movie)
	}
	_, err := e.svc.Buy(ctx, movie.ID, false)
	assertCode(t, err, 409, "not_enough_points")

	// Зарабатываем: 65 минут работы → 2 очка.
	x := e.task(t, "Дело", 4)
	s := must(e.svc.Start(ctx, x.ID))
	must(e.svc.Act(ctx, s.ID, ActionWork))
	e.clock.Advance(65 * time.Minute)
	must(e.svc.Finish(ctx, s.ID, OutcomeStopped))

	p := must(e.svc.Buy(ctx, movie.ID, true))
	if p.Balance != 0 || p.Dopamine != "added" || !reflect.DeepEqual(e.dop.texts, []string{"Кино"}) {
		t.Errorf("buy: %+v dopamine %v", p, e.dop.texts)
	}
	_, err = e.svc.Buy(ctx, movie.ID, false)
	assertCode(t, err, 409, "not_enough_points")

	balance, entries, next, err := e.svc.Points(ctx, nil, 1)
	if err != nil || balance != 0 || len(entries) != 1 || entries[0].Kind != "purchase" || entries[0].Delta != -2 || next == nil {
		t.Fatalf("ledger page 1: %d %+v %v %v", balance, entries, next, err)
	}
	_, entries, next, _ = e.svc.Points(ctx, next, 1)
	if len(entries) != 1 || entries[0].Kind != "focus" || entries[0].Delta != 2 || entries[0].Title != "Дело" || next != nil {
		t.Errorf("ledger page 2: %+v %v", entries, next)
	}

	must(e.svc.UpdateReward(ctx, cake.ID, RewardPatch{Archived: new(true)}))
	_, err = e.svc.Buy(ctx, cake.ID, false)
	assertCode(t, err, 409, "reward_archived")
	if rs := must(e.svc.Rewards(ctx, false)); len(rs) != 1 || rs[0].ID != movie.ID {
		t.Errorf("active rewards: %+v", rs)
	}
	if err := e.svc.ReorderRewards(ctx, []int64{cake.ID, movie.ID}); err != nil {
		t.Fatal(err)
	}
	if rs := must(e.svc.Rewards(ctx, true)); rs[0].ID != cake.ID {
		t.Errorf("order: %+v", rs)
	}
	// Удаление награды не трогает журнал: трата остаётся с названием-снимком.
	if err := e.svc.DeleteReward(ctx, movie.ID); err != nil {
		t.Fatal(err)
	}
	if _, entries, _, _ := e.svc.Points(ctx, nil, 10); len(entries) != 2 || entries[0].Title != "Кино" {
		t.Errorf("ledger after delete: %+v", entries)
	}
	assertCode(t, e.svc.DeleteReward(ctx, movie.ID), 404, "not_found")
}

func TestRitualAndSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if got := must(e.svc.Ritual(ctx)); len(got) != 0 {
		t.Errorf("empty ritual: %v", got)
	}
	must(e.svc.SaveRitual(ctx, []string{" Поднял стол ", "", "Налил воды"}))
	if got := must(e.svc.Ritual(ctx)); !reflect.DeepEqual(got, []string{"Поднял стол", "Налил воды"}) {
		t.Errorf("ritual: %v", got)
	}
	if got := e.svc.Settings(ctx); got.SetupMin != DefaultSetupMin {
		t.Errorf("default settings: %+v", got)
	}
	if got := must(e.svc.UpdateSettings(ctx, SettingsPatch{SetupMin: new(7)})); got.SetupMin != 7 || e.svc.Settings(ctx).SetupMin != 7 {
		t.Errorf("settings: %+v", got)
	}
}

func TestStats(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	report := e.task(t, "Отчёт", 2)
	closet := e.task(t, "Шкаф", 1)

	// Сессия: 5 мин настройки, 40 мин работы, перерыв 5 мин внутри, одно отвлечение.
	s := must(e.svc.Start(ctx, report.ID))
	e.clock.Advance(5 * time.Minute)
	must(e.svc.Act(ctx, s.ID, ActionWork))
	e.clock.Advance(20 * time.Minute)
	must(e.svc.Act(ctx, s.ID, ActionBreak))
	e.clock.Advance(5 * time.Minute)
	must(e.svc.Act(ctx, s.ID, ActionResume))
	must(e.svc.Distract(ctx, s.ID, DistractionInput{Trigger: "телефон", GaveIn: true}))
	e.clock.Advance(20 * time.Minute)
	must(e.svc.Finish(ctx, s.ID, OutcomeDone))
	// Отменённая — не в статистике; «Шкаф» закрыт с доски, без фокуса.
	a := must(e.svc.Start(ctx, closet.ID))
	must(e.svc.Finish(ctx, a.ID, OutcomeAbandoned))
	must(e.tasks.Update(ctx, closet.ID, tasks.Patch{Done: new(true)}))

	st := must(e.svc.Stats(ctx, "2026-11-04", "2026-11-05"))
	if len(st.Days) != 2 || st.Days[0].Sessions != 0 {
		t.Fatalf("days: %+v", st.Days)
	}
	today := st.Days[1]
	want := DayStat{Day: "2026-11-05", Sessions: 1, WorkSec: 40 * 60, SetupSec: 300, BreakSec: 300, TasksDone: 2, Distractions: 1, GaveIn: 1}
	if today != want {
		t.Errorf("today:\n got  %+v\n want %+v", today, want)
	}
	if !reflect.DeepEqual(st.Triggers, []TriggerStat{{"телефон", 1, 1}}) || len(st.Sessions) != 1 || len(st.TasksDone) != 2 {
		t.Errorf("lists: %+v %+v %+v", st.Triggers, st.Sessions, st.TasksDone)
	}
	r := st.Records
	if r.TotalWorkSec != 40*60 || r.TotalTasks != 2 || r.BestDay == nil || r.BestDay.Day != "2026-11-05" || r.BestWeek == nil || r.BestWeek.Day != "2026-11-02" {
		t.Errorf("records: %+v %+v %+v", r, r.BestDay, r.BestWeek)
	}
	// Время в фокусе — на карточке дела.
	if got := must(e.tasks.Get(ctx, report.ID)); got.WorkSec != 40*60 {
		t.Errorf("task workSec = %d", got.WorkSec)
	}
	_, err := e.svc.Stats(ctx, "2026-11-05", "2026-11-01")
	assertCode(t, err, 400, "invalid_range")
}

// utcEvents приводит время событий к UTC: DeepEqual сравнивает и зону, а pgx отдаёт время не в MSK.
func utcEvents(es []eventbus.Event) []eventbus.Event {
	out := make([]eventbus.Event, len(es))
	for i, e := range es {
		switch v := e.(type) {
		case eventbus.TaskCompleted:
			v.At = v.At.UTC()
			out[i] = v
		case eventbus.FocusFinished:
			v.At = v.At.UTC()
			out[i] = v
		default:
			out[i] = e
		}
	}
	return out
}
