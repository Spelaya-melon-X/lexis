package groups

import (
	"reflect"
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*60*60)

// at — момент в МСК: день ноября 2026 и время. 2 ноября 2026 — понедельник.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 11, day, hour, minute, 0, 0, msk)
}

func TestLevels(t *testing.T) {
	tests := []struct {
		name    string
		levels  Levels
		total   int
		percent int
	}{
		{"empty group", Levels{}, 0, 0},
		{"none mastered", Levels{3, 2, 1, 0, 0}, 6, 0},
		{"all archived", Levels{0, 0, 0, 0, 4}, 4, 100},
		{"master + archive counted", Levels{1, 1, 0, 1, 1}, 4, 50},
		{"rounds half up: 1/3 = 33%", Levels{2, 0, 0, 1, 0}, 3, 33},
		{"rounds half up: 2/3 = 67%", Levels{1, 0, 0, 1, 1}, 3, 67},
		{"rounds half up: 1/8 = 12.5% → 13%", Levels{7, 0, 0, 1, 0}, 8, 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.levels.Total(); got != tt.total {
				t.Errorf("Total = %d, want %d", got, tt.total)
			}
			if got := tt.levels.MasteredPercent(); got != tt.percent {
				t.Errorf("MasteredPercent = %d, want %d", got, tt.percent)
			}
		})
	}
}

func TestRepeatedToday(t *testing.T) {
	tests := []struct {
		name    string
		trained bool
		due     int
		want    bool
	}{
		{"trained, cards left (free session)", true, 5, true},
		{"not trained, nothing due", false, 0, true},
		{"not trained, cards due", false, 3, false},
		{"trained, nothing due", true, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepeatedToday(tt.trained, tt.due); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWeekStart(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"monday morning", at(2, 9, 0), at(2, 0, 0)},
		{"monday midnight", at(2, 0, 0), at(2, 0, 0)},
		{"wednesday", at(4, 15, 30), at(2, 0, 0)},
		{"sunday late", at(8, 23, 59), at(2, 0, 0)},
		{"next monday", at(9, 0, 1), at(9, 0, 0)},
		{"across month boundary", time.Date(2026, 12, 2, 10, 0, 0, 0, msk), at(30, 0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeekStart(tt.in); !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonthStart(t *testing.T) {
	if got, want := MonthStart(at(17, 13, 0)), at(1, 0, 0); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := MonthStart(at(1, 0, 0)), at(1, 0, 0); !got.Equal(want) {
		t.Errorf("first day: got %v, want %v", got, want)
	}
}

func TestRange_Days(t *testing.T) {
	tests := []struct {
		r    Range
		want int
		ok   bool
	}{
		{RangeWeek, 7, true}, {RangeMonth, 30, true}, {"year", 0, false}, {"", 0, false},
	}
	for _, tt := range tests {
		if got, ok := tt.r.Days(); got != tt.want || ok != tt.ok {
			t.Errorf("%q: got %d %v, want %d %v", tt.r, got, ok, tt.want, tt.ok)
		}
	}
}

func share(f float64) *float64 { return &f }

func TestProductivity(t *testing.T) {
	now := at(10, 12, 0) // окно week: 4..10 ноября

	tests := []struct {
		name     string
		sessions []SessionStat
		want     map[string]DayPoint // только непустые дни; остальные должны быть {Sessions:0, GoodShare:nil}
	}{
		{"no sessions", nil, nil},
		{"one session today",
			[]SessionStat{{FinishedAt: at(10, 9, 0), Total: 4, Good: 3}},
			map[string]DayPoint{"2026-11-10": {Day: "2026-11-10", Sessions: 1, GoodShare: share(0.75)}}},
		{"weighted by answers, not averaged per session",
			// 1/2 и 9/10 → 10/12, а не среднее (0.5+0.9)/2 = 0.7
			[]SessionStat{{FinishedAt: at(9, 9, 0), Total: 2, Good: 1}, {FinishedAt: at(9, 20, 0), Total: 10, Good: 9}},
			map[string]DayPoint{"2026-11-09": {Day: "2026-11-09", Sessions: 2, GoodShare: share(10.0 / 12.0)}}},
		{"session without answers counts, share stays nil",
			[]SessionStat{{FinishedAt: at(8, 9, 0), Total: 0, Good: 0}},
			map[string]DayPoint{"2026-11-08": {Day: "2026-11-08", Sessions: 1}}},
		{"first day of window included, day before excluded",
			[]SessionStat{{FinishedAt: at(4, 0, 0), Total: 1, Good: 1}, {FinishedAt: at(3, 23, 59), Total: 1, Good: 0}},
			map[string]DayPoint{"2026-11-04": {Day: "2026-11-04", Sessions: 1, GoodShare: share(1)}}},
		{"UTC time from DB bucketed in user TZ",
			// 9 ноября 22:30 UTC = 10 ноября 01:30 МСК
			[]SessionStat{{FinishedAt: time.Date(2026, 11, 9, 22, 30, 0, 0, time.UTC), Total: 2, Good: 0}},
			map[string]DayPoint{"2026-11-10": {Day: "2026-11-10", Sessions: 1, GoodShare: share(0)}}},
		{"future sessions ignored",
			[]SessionStat{{FinishedAt: at(11, 0, 0), Total: 1, Good: 1}},
			nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Productivity(tt.sessions, now, 7)
			if len(got) != 7 || got[0].Day != "2026-11-04" || got[6].Day != "2026-11-10" {
				t.Fatalf("window = %+v", got)
			}
			for _, p := range got {
				want, ok := tt.want[p.Day]
				if !ok {
					want = DayPoint{Day: p.Day}
				}
				if p.Sessions != want.Sessions || !floatPtrEqual(p.GoodShare, want.GoodShare) {
					t.Errorf("%s: got %d %v, want %d %v", p.Day, p.Sessions, deref(p.GoodShare), want.Sessions, deref(want.GoodShare))
				}
			}
		})
	}
}

func TestProductivity_MonthWindow(t *testing.T) {
	got := Productivity(nil, at(10, 12, 0), 30)
	days := make([]string, 0, 2)
	days = append(days, got[0].Day, got[29].Day)
	if !reflect.DeepEqual(days, []string{"2026-10-12", "2026-11-10"}) {
		t.Errorf("month window = %v", days)
	}
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a-*b < 1e-9 && *b-*a < 1e-9
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
