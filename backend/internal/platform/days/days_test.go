package days

import (
	"errors"
	"testing"
	"time"

	"lexis/internal/platform/httpx"
)

func date(s string) time.Time {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

func errCode(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	if err != nil {
		return "not an httpx.Error: " + err.Error()
	}
	return ""
}

func TestParse(t *testing.T) {
	for _, s := range []string{"03.11.2026", "2026-02-30", ""} {
		if code := errCode(func() error { _, err := Parse(s); return err }()); code != "invalid_date" {
			t.Errorf("Parse(%q) code = %q", s, code)
		}
	}
	if d := date("2026-11-03"); d.Location() != time.UTC || d.Day() != 3 {
		t.Errorf("Parse = %v, want 00:00 UTC", d)
	}
}

func TestCheckWritable(t *testing.T) {
	today := date("2026-11-03")
	tests := []struct {
		day      string
		wantCode string
	}{
		{"2026-11-03", ""},
		{"2026-11-02", ""},
		{"2020-01-01", ""},
		{"2026-11-04", "future_day"},
	}
	for _, tt := range tests {
		t.Run(tt.day, func(t *testing.T) {
			if code := errCode(CheckWritable(date(tt.day), today)); code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	today := date("2026-11-03")
	tests := []struct {
		name, from, to   string
		wantFrom, wantTo string
		wantCode         string
	}{
		{"defaults", "", "", "2026-10-05", "2026-11-03", ""},
		{"only to", "", "2026-10-31", "2026-10-02", "2026-10-31", ""},
		{"month incl. future days", "2026-11-01", "2026-11-30", "2026-11-01", "2026-11-30", ""},
		{"single day", "2026-11-03", "2026-11-03", "2026-11-03", "2026-11-03", ""},
		{"366 days", "2025-11-03", "2026-11-03", "2025-11-03", "2026-11-03", ""},
		{"367 days", "2025-11-02", "2026-11-03", "", "", "invalid_range"},
		{"from after to", "2026-11-04", "2026-11-03", "", "", "invalid_range"},
		{"bad from", "yesterday", "", "", "", "invalid_date"},
		{"bad to", "", "2026-02-30", "", "", "invalid_date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to, err := ParseRange(tt.from, tt.to, today, 30)
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err != nil {
				return
			}
			if !from.Equal(date(tt.wantFrom)) || !to.Equal(date(tt.wantTo)) {
				t.Errorf("range = %v..%v, want %s..%s", from, to, tt.wantFrom, tt.wantTo)
			}
		})
	}
}

func TestWeekStart(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026-11-02", "2026-11-02"}, // понедельник
		{"2026-11-05", "2026-11-02"},
		{"2026-11-08", "2026-11-02"}, // воскресенье
		{"2026-11-01", "2026-10-26"}, // через границу месяца
		{"2027-01-01", "2026-12-28"}, // через границу года
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := WeekStart(date(tt.in)); !got.Equal(date(tt.want)) {
				t.Errorf("WeekStart(%s) = %v, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseWeek(t *testing.T) {
	tests := []struct{ in, wantCode string }{
		{"2026-11-02", ""},
		{"2026-11-03", "not_monday"},
		{"2026-11-08", "not_monday"},
		{"week-45", "invalid_date"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := ParseWeek(tt.in)
			if code := errCode(err); code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}
