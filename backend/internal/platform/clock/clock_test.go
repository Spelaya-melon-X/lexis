package clock

import (
	"testing"
	"time"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}

func TestStartOfDay(t *testing.T) {
	msk := mustLoad(t, "Europe/Moscow")
	berlin := mustLoad(t, "Europe/Berlin")

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"midday msk", time.Date(2026, 11, 2, 15, 30, 0, 0, msk), time.Date(2026, 11, 2, 0, 0, 0, 0, msk)},
		{"just after midnight", time.Date(2026, 11, 2, 0, 0, 1, 0, msk), time.Date(2026, 11, 2, 0, 0, 0, 0, msk)},
		{"just before midnight", time.Date(2026, 11, 2, 23, 59, 59, 0, msk), time.Date(2026, 11, 2, 0, 0, 0, 0, msk)},
		// 22:00 UTC 2-го = 01:00 МСК 3-го: день берётся в таймзоне самого значения.
		{"utc input keeps utc day", time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
		{"same instant in msk is next day", time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC).In(msk), time.Date(2026, 11, 3, 0, 0, 0, 0, msk)},
		{"dst day berlin", time.Date(2026, 3, 29, 12, 0, 0, 0, berlin), time.Date(2026, 3, 29, 0, 0, 0, 0, berlin)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StartOfDay(tt.in); !got.Equal(tt.want) {
				t.Errorf("StartOfDay(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestDayBounds_DST(t *testing.T) {
	// В день перехода на летнее время в Берлине сутки длятся 23 часа.
	berlin := mustLoad(t, "Europe/Berlin")
	start, end := DayBounds(time.Date(2026, 3, 29, 12, 0, 0, 0, berlin))
	if got := end.Sub(start); got != 23*time.Hour {
		t.Errorf("DST day length = %v, want 23h", got)
	}
}

func TestDaysBetween(t *testing.T) {
	msk := mustLoad(t, "Europe/Moscow")
	d := func(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, msk) }

	tests := []struct {
		name string
		a, b time.Time
		want int
	}{
		{"same moment", d(2, 10, 0), d(2, 10, 0), 0},
		{"same day", d(2, 0, 1), d(2, 23, 59), 0},
		{"across midnight", d(2, 23, 59), d(3, 0, 1), 1},
		{"almost two days but one date apart", d(2, 0, 1), d(3, 23, 59), 1},
		{"week", d(2, 9, 0), d(9, 8, 0), 7},
		{"backwards", d(5, 0, 0), d(2, 0, 0), -3},
		// b в UTC: 21:30 UTC 2-го = 00:30 МСК 3-го → разные даты по Москве
		{"b in other tz", d(2, 12, 0), time.Date(2026, 11, 2, 21, 30, 0, 0, time.UTC), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DaysBetween(tt.a, tt.b); got != tt.want {
				t.Errorf("DaysBetween = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFixed(t *testing.T) {
	msk := mustLoad(t, "Europe/Moscow")
	c := NewFixed(time.Date(2026, 11, 2, 23, 0, 0, 0, msk))

	if c.Location() != msk {
		t.Fatalf("Location = %v, want %v", c.Location(), msk)
	}
	c.Advance(2 * time.Hour)
	if want := time.Date(2026, 11, 3, 0, 0, 0, 0, msk); !Today(c).Equal(want) {
		t.Errorf("Today after advance = %v, want %v", Today(c), want)
	}
}
