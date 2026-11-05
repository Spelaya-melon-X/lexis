package clock

import (
	"testing"
	"time"
)

func TestDateOf(t *testing.T) {
	msk := mustLoad(t, "Europe/Moscow")

	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"midday msk", time.Date(2026, 11, 3, 15, 0, 0, 0, msk), "2026-11-03"},
		{"00:30 msk is still that day", time.Date(2026, 11, 3, 0, 30, 0, 0, msk), "2026-11-03"},
		{"23:59 msk", time.Date(2026, 11, 3, 23, 59, 0, 0, msk), "2026-11-03"},
		// 22:00 UTC 2-го = 01:00 МСК 3-го: дата берётся в зоне самого значения.
		{"utc instant", time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC), "2026-11-02"},
		{"same instant in msk", time.Date(2026, 11, 2, 22, 0, 0, 0, time.UTC).In(msk), "2026-11-03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DateOf(tt.in)
			if FormatDate(got) != tt.want {
				t.Errorf("DateOf(%v) = %s, want %s", tt.in, FormatDate(got), tt.want)
			}
			if got.Location() != time.UTC || got.Hour() != 0 || got.Minute() != 0 {
				t.Errorf("DateOf must return 00:00 UTC, got %v", got)
			}
		})
	}
}

func TestTodayDate(t *testing.T) {
	msk := mustLoad(t, "Europe/Moscow")
	c := NewFixed(time.Date(2026, 11, 3, 1, 0, 0, 0, msk)) // 22:00 UTC 2-го
	if got := FormatDate(TodayDate(c)); got != "2026-11-03" {
		t.Errorf("TodayDate = %s, want 2026-11-03 (TZ пользователя, не UTC)", got)
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"2026-11-03", false},
		{"2028-02-29", false},
		{"2026-02-30", true},
		{"2026-13-01", true},
		{"03.11.2026", true},
		{"2026-11-03T00:00:00Z", true},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDate(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (FormatDate(got) != tt.in || !got.Equal(DateOf(got))) {
				t.Errorf("ParseDate(%q) = %v: not a round-trip 00:00 UTC date", tt.in, got)
			}
		})
	}
}
