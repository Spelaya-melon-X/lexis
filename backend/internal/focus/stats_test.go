package focus

import (
	"reflect"
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

func TestSummarize(t *testing.T) {
	from, to := civil("2026-11-02"), civil("2026-11-04")
	sessions := []SessionStat{
		{ID: 1, TaskTitle: "Отчёт", Day: civil("2026-11-02"), Outcome: OutcomeDone,
			Timeline:     Timeline{WorkSec: 3600, SetupSec: 300, BreakSec: 300},
			Distractions: []DistractionStat{{"телефон", true}, {"чат", false}}},
		{ID: 2, TaskTitle: "Курсовая", Day: civil("2026-11-04"), Outcome: OutcomeStopped,
			Timeline:     Timeline{WorkSec: 1800, SetupSec: 60},
			Distractions: []DistractionStat{{"телефон", false}, {"телефон", true}, {"кот", true}}},
		{ID: 3, TaskTitle: "Мимо", Day: civil("2026-11-05"), Outcome: OutcomeDone, Timeline: Timeline{WorkSec: 999}}, // вне окна
		{ID: 4, TaskTitle: "Отмена", Day: civil("2026-11-03"), Outcome: OutcomeAbandoned, Timeline: Timeline{SetupSec: 60}},
	}
	done := []DoneTask{
		{ID: 7, Title: "Отчёт", Day: civil("2026-11-02")},
		{ID: 8, Title: "Шкаф", Day: civil("2026-11-03")}, // закрыли с доски, без фокуса
		{ID: 9, Title: "Раньше", Day: civil("2026-11-01")},
	}
	st := Summarize(from, to, sessions, done)

	want := []DayStat{
		{Day: "2026-11-02", Sessions: 1, WorkSec: 3600, SetupSec: 300, BreakSec: 300, TasksDone: 1, Distractions: 2, GaveIn: 1},
		{Day: "2026-11-03", TasksDone: 1},
		{Day: "2026-11-04", Sessions: 1, WorkSec: 1800, SetupSec: 60, Distractions: 3, GaveIn: 2},
	}
	if !reflect.DeepEqual(st.Days, want) {
		t.Errorf("days:\n got  %+v\n want %+v", st.Days, want)
	}
	if tot := st.Totals; tot.Sessions != 2 || tot.WorkSec != 5400 || tot.TasksDone != 2 || tot.Distractions != 5 || tot.GaveIn != 3 {
		t.Errorf("totals: %+v", tot)
	}
	wantTriggers := []TriggerStat{{"телефон", 3, 2}, {"кот", 1, 1}, {"чат", 1, 0}}
	if !reflect.DeepEqual(st.Triggers, wantTriggers) {
		t.Errorf("triggers = %+v", st.Triggers)
	}
	if len(st.TasksDone) != 2 || st.TasksDone[1].On != "2026-11-03" || len(st.Sessions) != 2 || st.Sessions[1].GaveIn != 2 {
		t.Errorf("lists: %+v %+v", st.TasksDone, st.Sessions)
	}

	empty := Summarize(from, from, nil, nil)
	if len(empty.Days) != 1 || empty.Triggers == nil || empty.TasksDone == nil || empty.Sessions == nil {
		t.Errorf("empty window must have one day and non-nil lists: %#v", empty)
	}
}

func TestSummarize_TopTriggersCut(t *testing.T) {
	var ds []DistractionStat
	for i := range TopTriggers + 3 {
		ds = append(ds, DistractionStat{Trigger: string(rune('a' + i))})
	}
	st := Summarize(civil("2026-11-02"), civil("2026-11-02"), []SessionStat{{Day: civil("2026-11-02"), Outcome: OutcomeDone, Distractions: ds}}, nil)
	if len(st.Triggers) != TopTriggers || st.Triggers[0].Trigger != "a" {
		t.Errorf("triggers = %+v", st.Triggers)
	}
}

func TestBuildRecords(t *testing.T) {
	work := []DayWork{
		{civil("2026-10-26"), 3600}, // пн
		{civil("2026-10-28"), 7200}, // ср той же недели → неделя 10800
		{civil("2026-11-02"), 7200}, // пн следующей; день вровень с 28-м — рекорд остаётся за ранним
		{civil("2026-11-03"), 0},
	}
	r := BuildRecords(work, 12)
	want := Records{
		TotalWorkSec: 18000, TotalTasks: 12,
		BestDay:  &Best{Day: "2026-10-28", WorkSec: 7200},
		BestWeek: &Best{Day: "2026-10-26", WorkSec: 10800},
	}
	if !reflect.DeepEqual(r, want) {
		t.Errorf("got %+v %+v %+v, want %+v %+v %+v", r, r.BestDay, r.BestWeek, want, want.BestDay, want.BestWeek)
	}
	if empty := BuildRecords(nil, 0); empty.BestDay != nil || empty.BestWeek != nil {
		t.Errorf("empty: %+v", empty)
	}
}
