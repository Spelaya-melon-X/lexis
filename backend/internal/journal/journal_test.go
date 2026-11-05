package journal

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"lexis/internal/platform/days"
	"lexis/internal/platform/httpx"
)

func date(s string) time.Time {
	d, err := days.Parse(s)
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

func TestPatchNormalize(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
		wantWins []string // nil — поле wins не трогаем
	}{
		{"empty patch", `{}`, "", nil},
		{"rating 1", `{"rating": 1}`, "", nil},
		{"rating 10", `{"rating": 10}`, "", nil},
		{"rating null clears", `{"rating": null}`, "", nil},
		{"rating 0", `{"rating": 0}`, "invalid_rating", nil},
		{"rating 11", `{"rating": 11}`, "invalid_rating", nil},
		{"wins trimmed, blanks dropped", `{"wins": ["  ran 5k ", "", "   ", "read"]}`, "", []string{"ran 5k", "read"}},
		{"wins cleared", `{"wins": []}`, "", []string{}},
		{"win too long", `{"wins": ["` + strings.Repeat("я", MaxWinLen+1) + `"]}`, "text_too_long", nil},
		{"win at limit (runes, not bytes)", `{"wins": ["` + strings.Repeat("я", MaxWinLen) + `"]}`, "", []string{strings.Repeat("я", MaxWinLen)}},
		{"too many wins", `{"wins": [` + strings.TrimSuffix(strings.Repeat(`"x",`, MaxWins+1), ",") + `]}`, "too_many_items", nil},
		{"story too long", `{"story": "` + strings.Repeat("a", MaxStoryLen+1) + `"}`, "text_too_long", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Patch
			if err := json.Unmarshal([]byte(tt.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, err := p.Normalize()
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err != nil || tt.wantWins == nil {
				return
			}
			if got.Wins == nil || !reflect.DeepEqual(*got.Wins, tt.wantWins) {
				t.Errorf("wins = %v, want %v", got.Wins, tt.wantWins)
			}
		})
	}
}

func TestNormalizeDopamine(t *testing.T) {
	tests := []struct {
		name     string
		slot     int
		text     string
		want     string
		wantCode string
	}{
		{"ok", 0, "  кофе с другом ", "кофе с другом", ""},
		{"last slot", 6, "x", "x", ""},
		{"empty clears", 3, "   ", "", ""},
		{"slot -1", -1, "x", "", "invalid_slot"},
		{"slot 7", 7, "x", "", "invalid_slot"},
		{"140 runes ok", 1, strings.Repeat("ё", MaxDopamineLen), strings.Repeat("ё", MaxDopamineLen), ""},
		{"141 runes", 1, strings.Repeat("ё", MaxDopamineLen+1), "", "text_too_long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDopamine(tt.slot, tt.text)
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFillDays(t *testing.T) {
	r7 := 7
	got := FillDays(date("2026-10-30"), date("2026-11-02"), []SummaryRow{
		{Day: date("2026-10-31"), Rating: &r7, Dopamine: 3, Wins: 2, HasStory: true},
		{Day: date("2026-11-02"), Dopamine: 7},
		{Day: date("2026-11-10")}, // вне окна — игнорируется, не паникует
	})
	want := []DaySummary{
		{Day: "2026-10-30"},
		{Day: "2026-10-31", Rating: &r7, DopamineLevel: 3, Wins: 2, HasStory: true},
		{Day: "2026-11-01"},
		{Day: "2026-11-02", DopamineLevel: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FillDays =\n%+v\nwant\n%+v", got, want)
	}

	// Переход через конец месяца/года и пустое окно.
	if got := FillDays(date("2026-12-31"), date("2027-01-01"), nil); len(got) != 2 || got[1].Day != "2027-01-01" {
		t.Errorf("year boundary: %+v", got)
	}
	if got := FillDays(date("2026-11-02"), date("2026-11-01"), nil); len(got) != 0 {
		t.Errorf("inverted window must be empty: %+v", got)
	}
}
