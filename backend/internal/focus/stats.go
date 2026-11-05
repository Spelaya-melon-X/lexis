package focus

import (
	"cmp"
	"slices"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/days"
)

// Статистика «Игры» (§18.3, B5). Чистые функции: сервис достаёт сессии, дела и отвлечения,
// здесь — раскладка по дням, топ триггеров и рекорды.

const TopTriggers = 10

// SessionStat — завершённая (не отменённая) сессия с уже посчитанным таймлайном.
type SessionStat struct {
	ID           int64
	TaskTitle    string
	Day          time.Time // гражданская дата
	Outcome      Outcome
	Timeline     Timeline
	Distractions []DistractionStat
}

type DistractionStat struct {
	Trigger string
	GaveIn  bool
}

// DoneTask — выполненное дело-лист и день выполнения (в TZ пользователя).
type DoneTask struct {
	ID    int64     `json:"id"`
	Title string    `json:"title"`
	Day   time.Time `json:"-"`
	On    string    `json:"day"`
}

type DayStat struct {
	Day          string `json:"day"`
	Sessions     int    `json:"sessions"`
	WorkSec      int    `json:"workSec"`
	SetupSec     int    `json:"setupSec"`
	BreakSec     int    `json:"breakSec"`
	TasksDone    int    `json:"tasksDone"`
	Distractions int    `json:"distractions"`
	GaveIn       int    `json:"gaveIn"`
}

type TriggerStat struct {
	Trigger string `json:"trigger"`
	Total   int    `json:"total"`
	GaveIn  int    `json:"gaveIn"`
}

type SessionSummary struct {
	ID           int64  `json:"id"`
	TaskTitle    string `json:"taskTitle"`
	Day          string `json:"day"`
	Outcome      string `json:"outcome"`
	WorkSec      int    `json:"workSec"`
	Distractions int    `json:"distractions"`
	GaveIn       int    `json:"gaveIn"`
}

// Stats — ответ GET /api/focus/stats.
type Stats struct {
	From      string           `json:"from"`
	To        string           `json:"to"`
	Days      []DayStat        `json:"days"`   // все дни окна, включая пустые
	Totals    DayStat          `json:"totals"` // Day пустой
	Triggers  []TriggerStat    `json:"triggers"`
	TasksDone []DoneTask       `json:"tasksDone"`
	Sessions  []SessionSummary `json:"sessions"`
	Records   Records          `json:"records"`
}

// Summarize раскладывает сессии и выполненные дела по дням окна [from, to] и считает
// топ триггеров (частые сверху; при равенстве — по алфавиту, чтобы порядок был стабильным).
func Summarize(from, to time.Time, sessions []SessionStat, done []DoneTask) Stats {
	n := clock.DaysBetween(from, to) + 1
	st := Stats{
		From: clock.FormatDate(from), To: clock.FormatDate(to), Days: make([]DayStat, n),
		Triggers: []TriggerStat{}, TasksDone: []DoneTask{}, Sessions: []SessionSummary{},
	}
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

	triggers := map[string]*TriggerStat{}
	for _, s := range sessions {
		d := at(s.Day)
		if d == nil || s.Outcome == OutcomeAbandoned {
			continue
		}
		gave := 0
		for _, x := range s.Distractions {
			t := triggers[x.Trigger]
			if t == nil {
				t = &TriggerStat{Trigger: x.Trigger}
				triggers[x.Trigger] = t
			}
			t.Total++
			if x.GaveIn {
				t.GaveIn++
				gave++
			}
		}
		for _, acc := range []*DayStat{d, &st.Totals} {
			acc.Sessions++
			acc.WorkSec += s.Timeline.WorkSec
			acc.SetupSec += s.Timeline.SetupSec
			acc.BreakSec += s.Timeline.BreakSec
			acc.Distractions += len(s.Distractions)
			acc.GaveIn += gave
		}
		st.Sessions = append(st.Sessions, SessionSummary{
			ID: s.ID, TaskTitle: s.TaskTitle, Day: clock.FormatDate(s.Day), Outcome: string(s.Outcome),
			WorkSec: s.Timeline.WorkSec, Distractions: len(s.Distractions), GaveIn: gave,
		})
	}
	for _, t := range done {
		d := at(t.Day)
		if d == nil {
			continue
		}
		d.TasksDone++
		st.Totals.TasksDone++
		t.On = clock.FormatDate(t.Day)
		st.TasksDone = append(st.TasksDone, t)
	}
	for _, t := range triggers {
		st.Triggers = append(st.Triggers, *t)
	}
	slices.SortFunc(st.Triggers, func(a, b TriggerStat) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), cmp.Compare(a.Trigger, b.Trigger))
	})
	if len(st.Triggers) > TopTriggers {
		st.Triggers = st.Triggers[:TopTriggers]
	}
	return st
}

// DayWork — чистая работа за день (для рекордов).
type DayWork struct {
	Day     time.Time
	WorkSec int
}

type Best struct {
	Day     string `json:"day"` // день; для недели — её понедельник
	WorkSec int    `json:"workSec"`
}

// Records — за всё время: всего часов и дел, лучший день и лучшая неделя (с понедельника).
// «За неделю/месяц» — не ачивки, а рекорды (§18.3 B5).
type Records struct {
	TotalWorkSec int   `json:"totalWorkSec"`
	TotalTasks   int   `json:"totalTasks"`
	BestDay      *Best `json:"bestDay"`
	BestWeek     *Best `json:"bestWeek"`
}

// BuildRecords — при равенстве побеждает более ранний (рекорд, который потом лишь повторили).
func BuildRecords(work []DayWork, totalTasks int) Records {
	r := Records{TotalTasks: totalTasks}
	weeks := map[time.Time]int{}
	for _, w := range work {
		r.TotalWorkSec += w.WorkSec
		weeks[days.WeekStart(w.Day)] += w.WorkSec
		if w.WorkSec > 0 && (r.BestDay == nil || w.WorkSec > r.BestDay.WorkSec) {
			r.BestDay = &Best{Day: clock.FormatDate(w.Day), WorkSec: w.WorkSec}
		}
	}
	mondays := make([]time.Time, 0, len(weeks))
	for m := range weeks {
		mondays = append(mondays, m)
	}
	slices.SortFunc(mondays, func(a, b time.Time) int { return a.Compare(b) })
	for _, m := range mondays {
		if w := weeks[m]; w > 0 && (r.BestWeek == nil || w > r.BestWeek.WorkSec) {
			r.BestWeek = &Best{Day: clock.FormatDate(m), WorkSec: w}
		}
	}
	return r
}
