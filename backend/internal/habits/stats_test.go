package habits

import (
	"encoding/json"
	"testing"
)

func TestCount(t *testing.T) {
	r := func(v int16) *int16 { return &v }
	f := func(v float64) *float64 { return &v }
	// Окно — неделя 2..8 ноября, сегодня 5-е.
	from, to, today := date("2026-11-02"), date("2026-11-08"), date("2026-11-05")
	vals := []DayValue{
		{Day: "2026-11-01", Amount: f(100)}, // до окна — не считается
		{Day: "2026-11-02", Amount: f(5), Rating: r(8)},
		{Day: "2026-11-04", Amount: f(2.5), Rating: r(6)},
		{Day: "2026-11-05"},
		{Day: "2026-11-07", Amount: f(9)}, // будущее — не считается
	}

	tests := []struct {
		name  string
		kind  string
		start string
		vals  []DayValue
		want  string // JSON — так nil-указатели и числа сравниваются наглядно
	}{
		{"positive, full window", KindPositive, "2026-10-01", vals,
			`{"days":4,"checked":3,"amount":7.5,"avgRating":7,"bestClean":0}`},
		{"negative: clean streak between slips", KindNegative, "2026-10-01", vals,
			`{"days":4,"checked":3,"amount":7.5,"avgRating":7,"bestClean":1}`},
		{"negative, no slips", KindNegative, "2026-10-01", nil,
			`{"days":4,"checked":0,"amount":null,"avgRating":null,"bestClean":4}`},
		{"created mid-window", KindNegative, "2026-11-04", nil,
			`{"days":2,"checked":0,"amount":null,"avgRating":null,"bestClean":2}`},
		{"created after today", KindPositive, "2026-11-06", nil,
			`{"days":0,"checked":0,"amount":null,"avgRating":null,"bestClean":0}`},
		{"checkin before creation still counts", KindPositive, "2026-11-04", []DayValue{{Day: "2026-11-02"}},
			`{"days":2,"checked":1,"amount":null,"avgRating":null,"bestClean":0}`},
		{"duplicate day counted once", KindPositive, "2026-10-01", []DayValue{{Day: "2026-11-02"}, {Day: "2026-11-02"}},
			`{"days":4,"checked":1,"amount":null,"avgRating":null,"bestClean":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := json.Marshal(Count(tt.kind, date(tt.start), from, to, today, tt.vals))
			if string(got) != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}

	// Окно целиком в прошлом: учитываются все его дни.
	got, _ := json.Marshal(Count(KindNegative, date("2026-10-01"), date("2026-10-26"), date("2026-11-01"), today,
		[]DayValue{{Day: "2026-10-28"}}))
	if want := `{"days":7,"checked":1,"amount":null,"avgRating":null,"bestClean":4}`; string(got) != want {
		t.Errorf("past window: got %s, want %s", got, want)
	}
}
