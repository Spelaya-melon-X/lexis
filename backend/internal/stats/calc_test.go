package stats

import (
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*60*60)

// at — момент в МСК: день ноября 2026 и время.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 11, day, hour, minute, 0, 0, msk)
}

func TestStreaks(t *testing.T) {
	now := at(10, 12, 0)

	tests := []struct {
		name     string
		finished []time.Time
		want     Streak
	}{
		{"no sessions", nil, Streak{}},
		{"only today", []time.Time{at(10, 9, 0)}, Streak{1, 1}},
		{"several sessions same day count once", []time.Time{at(10, 9, 0), at(10, 10, 0), at(10, 11, 0)}, Streak{1, 1}},
		{"yesterday only — streak still alive today", []time.Time{at(9, 9, 0)}, Streak{1, 1}},
		{"day before yesterday — broken", []time.Time{at(8, 9, 0)}, Streak{0, 1}},
		{"3 days ending today", []time.Time{at(8, 9, 0), at(9, 9, 0), at(10, 9, 0)}, Streak{3, 3}},
		{"3 days ending yesterday, none today yet", []time.Time{at(7, 9, 0), at(8, 9, 0), at(9, 9, 0)}, Streak{3, 3}},
		{"best is older than current", []time.Time{
			at(1, 9, 0), at(2, 9, 0), at(3, 9, 0), at(4, 9, 0), // 4
			at(9, 9, 0), at(10, 9, 0), // 2
		}, Streak{2, 4}},
		{"unsorted input", []time.Time{at(10, 9, 0), at(8, 9, 0), at(9, 9, 0)}, Streak{3, 3}},
		{"calendar days, not 24h: 23:59 → 00:01", []time.Time{at(9, 23, 59), at(10, 0, 1)}, Streak{2, 2}},
		{"UTC from DB bucketed in user TZ",
			// 9 ноября 22:30 UTC = 10 ноября 01:30 МСК; 8 ноября 22:00 UTC = 9 ноября 01:00 МСК
			[]time.Time{time.Date(2026, 11, 9, 22, 30, 0, 0, time.UTC), time.Date(2026, 11, 8, 22, 0, 0, 0, time.UTC)},
			Streak{2, 2}},
		{"across month boundary", []time.Time{time.Date(2026, 10, 31, 12, 0, 0, 0, msk), at(1, 12, 0)}, Streak{0, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Streaks(tt.finished, now); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBucketByDay(t *testing.T) {
	g1, g2 := int64(1), int64(2)
	rows := []SessionRow{
		{FinishedAt: at(4, 9, 0), GroupID: &g1, Total: 4, Good: 2, Hard: 1, Again: 1, DurationSec: 120},
		{FinishedAt: at(4, 20, 0), GroupID: &g2, Total: 6, Good: 6, DurationSec: 60},
		{FinishedAt: at(6, 1, 0), Total: 0, DurationSec: 5}, // сессия без ответов
		{FinishedAt: at(3, 23, 59), Total: 9, Good: 9},      // до окна
		{FinishedAt: at(8, 0, 0), Total: 9, Good: 9},        // после окна
		// 5 ноября 22:00 UTC = 6 ноября 01:00 МСК
		{FinishedAt: time.Date(2026, 11, 5, 22, 0, 0, 0, time.UTC), Total: 2, Good: 1, Again: 1, DurationSec: 30},
	}
	got := BucketByDay(rows, at(4, 15, 0), 4) // 4..7 ноября; время first отбрасывается
	if len(got) != 4 || got[0].Day != "2026-11-04" || got[3].Day != "2026-11-07" {
		t.Fatalf("window = %+v", got)
	}

	type want struct {
		sessions, words, good, hard, again, seconds int
		share                                       *float64
	}
	f := func(v float64) *float64 { return &v }
	wants := []want{
		{2, 10, 8, 1, 1, 180, f(0.8)},
		{0, 0, 0, 0, 0, 0, nil},
		{2, 2, 1, 0, 1, 35, f(0.5)},
		{0, 0, 0, 0, 0, 0, nil},
	}
	for i, w := range wants {
		d := got[i]
		if d.Sessions != w.sessions || d.Words != w.words || d.Good != w.good || d.Hard != w.hard ||
			d.Again != w.again || d.Seconds != w.seconds || !floatPtrEqual(d.GoodShare, w.share) {
			t.Errorf("%s: got %+v (share %v), want %+v", d.Day, d, deref(d.GoodShare), w)
		}
	}
}

func TestGroupsTrained(t *testing.T) {
	g1, g2 := int64(1), int64(2)
	rows := []SessionRow{{GroupID: &g1}, {GroupID: &g2}, {GroupID: &g1}, {GroupID: nil}}
	if got := GroupsTrained(rows); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := GroupsTrained(nil); got != 0 {
		t.Errorf("empty: got %d", got)
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
