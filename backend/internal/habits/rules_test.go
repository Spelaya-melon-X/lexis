package habits

import (
	"encoding/json"
	"errors"
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

func ptr[T any](v T) *T { return &v }

func TestFieldsNormalize(t *testing.T) {
	tests := []struct {
		name     string
		in       Fields
		want     Fields
		wantCode string
	}{
		{"ok", Fields{Name: "  Бег  по   утрам ", Kind: KindPositive, Color: ptr("#16A34A"), Unit: " км "},
			Fields{Name: "Бег по утрам", Kind: KindPositive, Color: ptr("#16a34a"), Unit: "км"}, ""},
		{"negative, no color", Fields{Name: "Курение", Kind: KindNegative}, Fields{Name: "Курение", Kind: KindNegative}, ""},
		{"empty name", Fields{Name: "   ", Kind: KindPositive}, Fields{}, "invalid_habit_name"},
		{"name 60 runes", Fields{Name: strings.Repeat("я", MaxNameLen), Kind: KindPositive},
			Fields{Name: strings.Repeat("я", MaxNameLen), Kind: KindPositive}, ""},
		{"name 61 runes", Fields{Name: strings.Repeat("я", MaxNameLen+1), Kind: KindPositive}, Fields{}, "invalid_habit_name"},
		{"bad kind", Fields{Name: "x", Kind: "neutral"}, Fields{}, "invalid_kind"},
		{"bad color", Fields{Name: "x", Kind: KindPositive, Color: ptr("green")}, Fields{}, "invalid_color"},
		{"short color", Fields{Name: "x", Kind: KindPositive, Color: ptr("#fff")}, Fields{}, "invalid_color"},
		{"unit too long", Fields{Name: "x", Kind: KindPositive, Unit: strings.Repeat("u", MaxUnitLen+1)}, Fields{}, "invalid_unit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Normalize()
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err != nil {
				return
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestPatchApply(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-48 * time.Hour)
	base := Fields{Name: "Бег", Kind: KindPositive, Color: ptr("#16a34a"), TrackAmount: true, Unit: "км"}

	tests := []struct {
		name         string
		body         string
		archivedAt   *time.Time
		wantName     string
		wantColor    *string
		wantAmount   bool
		wantArchived *time.Time
		wantCode     string
	}{
		{"empty patch keeps all", `{}`, nil, "Бег", ptr("#16a34a"), true, nil, ""},
		{"rename + normalize", `{"name": "  Бег  утром "}`, nil, "Бег утром", ptr("#16a34a"), true, nil, ""},
		{"color null clears", `{"color": null}`, nil, "Бег", nil, true, nil, ""},
		{"turn off amount", `{"trackAmount": false}`, nil, "Бег", ptr("#16a34a"), false, nil, ""},
		{"archive sets now", `{"archived": true}`, nil, "Бег", ptr("#16a34a"), true, &now, ""},
		{"archive again keeps date", `{"archived": true}`, &earlier, "Бег", ptr("#16a34a"), true, &earlier, ""},
		{"unarchive", `{"archived": false}`, &earlier, "Бег", ptr("#16a34a"), true, nil, ""},
		{"bad name", `{"name": ""}`, nil, "", nil, false, nil, "invalid_habit_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Patch
			if err := json.Unmarshal([]byte(tt.body), &p); err != nil {
				t.Fatal(err)
			}
			f, arch, err := p.Apply(base, tt.archivedAt, now)
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err != nil {
				return
			}
			if f.Name != tt.wantName || (f.Color == nil) != (tt.wantColor == nil) || (f.Color != nil && *f.Color != *tt.wantColor) || f.TrackAmount != tt.wantAmount {
				t.Errorf("fields = %+v", f)
			}
			if (arch == nil) != (tt.wantArchived == nil) || (arch != nil && !arch.Equal(*tt.wantArchived)) {
				t.Errorf("archivedAt = %v, want %v", arch, tt.wantArchived)
			}
		})
	}
}

func TestValidateCheckin(t *testing.T) {
	all := Tracks{Trigger: true, Rating: true, Amount: true}
	none := Tracks{}
	tests := []struct {
		name     string
		tracks   Tracks
		in       CheckinInput
		want     CheckinInput
		wantCode string
	}{
		{"plain done", none, CheckinInput{HabitID: 1, Done: true}, CheckinInput{HabitID: 1, Done: true}, ""},
		{"undone drops fields", all, CheckinInput{HabitID: 1, Rating: ptr(11), Trigger: ptr("x")}, CheckinInput{HabitID: 1}, ""},
		{"all fields", all, CheckinInput{HabitID: 1, Done: true, Trigger: ptr("  стресс "), Rating: ptr(7), Amount: ptr(5.2)},
			CheckinInput{HabitID: 1, Done: true, Trigger: ptr("стресс"), Rating: ptr(7), Amount: ptr(5.2)}, ""},
		{"blank trigger → nil", Tracks{Trigger: true}, CheckinInput{HabitID: 1, Done: true, Trigger: ptr("   ")},
			CheckinInput{HabitID: 1, Done: true}, ""},
		{"rating & trigger optional", Tracks{Trigger: true, Rating: true}, CheckinInput{HabitID: 1, Done: true},
			CheckinInput{HabitID: 1, Done: true}, ""},
		{"amount zero ok", Tracks{Amount: true}, CheckinInput{HabitID: 1, Done: true, Amount: ptr(0.0)},
			CheckinInput{HabitID: 1, Done: true, Amount: ptr(0.0)}, ""},
		{"amount required", Tracks{Amount: true}, CheckinInput{HabitID: 1, Done: true}, CheckinInput{}, "amount_required"},
		{"trigger not enabled", none, CheckinInput{HabitID: 1, Done: true, Trigger: ptr("x")}, CheckinInput{}, "field_not_enabled"},
		{"rating not enabled", none, CheckinInput{HabitID: 1, Done: true, Rating: ptr(5)}, CheckinInput{}, "field_not_enabled"},
		{"amount not enabled", none, CheckinInput{HabitID: 1, Done: true, Amount: ptr(1.0)}, CheckinInput{}, "field_not_enabled"},
		{"rating 0", all, CheckinInput{HabitID: 1, Done: true, Rating: ptr(0), Amount: ptr(1.0)}, CheckinInput{}, "invalid_rating"},
		{"rating 11", all, CheckinInput{HabitID: 1, Done: true, Rating: ptr(11), Amount: ptr(1.0)}, CheckinInput{}, "invalid_rating"},
		{"negative amount", all, CheckinInput{HabitID: 1, Done: true, Amount: ptr(-1.0)}, CheckinInput{}, "invalid_amount"},
		{"huge amount", all, CheckinInput{HabitID: 1, Done: true, Amount: ptr(2e6)}, CheckinInput{}, "invalid_amount"},
		{"trigger too long", all, CheckinInput{HabitID: 1, Done: true, Amount: ptr(1.0), Trigger: ptr(strings.Repeat("я", MaxTriggerLen+1))},
			CheckinInput{}, "text_too_long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateCheckin(tt.tracks, tt.in)
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err != nil {
				return
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("got %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestGoalInputNormalize(t *testing.T) {
	tests := []struct {
		name     string
		in       GoalInput
		wantCode string
	}{
		{"habit goal, no title", GoalInput{HabitID: ptr(int64(1)), From: "2026-11-01", To: "2026-11-09"}, ""},
		{"free text goal", GoalInput{Title: "Ложиться до 23:00", From: "2026-11-01", To: "2026-11-01"}, ""},
		{"no habit, no title", GoalInput{Title: "   ", From: "2026-11-01", To: "2026-11-09"}, "title_required"},
		{"to before from", GoalInput{Title: "x", From: "2026-11-09", To: "2026-11-01"}, "invalid_range"},
		{"366 days", GoalInput{Title: "x", From: "2026-01-01", To: "2027-01-01"}, ""},
		{"367 days", GoalInput{Title: "x", From: "2026-01-01", To: "2027-01-02"}, "invalid_range"},
		{"bad date", GoalInput{Title: "x", From: "1 ноября", To: "2026-11-09"}, "invalid_date"},
		{"title too long", GoalInput{Title: strings.Repeat("x", MaxGoalTitle+1), From: "2026-11-01", To: "2026-11-02"}, "text_too_long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.in.Normalize()
			if code := errCode(err); code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestGoalProgress(t *testing.T) {
	// Цель 1..9 ноября, отметки 1, 2, 5 и 20 (последняя вне цели).
	checked := map[string]bool{"2026-11-01": true, "2026-11-02": true, "2026-11-05": true, "2026-11-20": true}
	start, end := date("2026-11-01"), date("2026-11-09")
	tests := []struct {
		name, kind, today string
		want              Progress
	}{
		{"positive, mid-goal", KindPositive, "2026-11-05", Progress{Done: 3, Elapsed: 5, Total: 9}},
		{"negative, mid-goal: clean days", KindNegative, "2026-11-05", Progress{Done: 2, Elapsed: 5, Total: 9}},
		{"positive, after the goal", KindPositive, "2026-11-30", Progress{Done: 3, Elapsed: 9, Total: 9}},
		{"negative, after the goal", KindNegative, "2026-11-30", Progress{Done: 6, Elapsed: 9, Total: 9}},
		{"not started yet", KindPositive, "2026-10-31", Progress{Done: 0, Elapsed: 0, Total: 9}},
		{"first day", KindPositive, "2026-11-01", Progress{Done: 1, Elapsed: 1, Total: 9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GoalProgress(tt.kind, start, end, date(tt.today), checked); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNoteInputNormalize(t *testing.T) {
	tests := []struct {
		name     string
		in       NoteInput
		wantBody string
		wantCode string
	}{
		{"ok, trimmed, inner newlines kept", NoteInput{Kind: NoteInsight, Body: "\n Вывод:\nбегать утром \n"}, "Вывод:\nбегать утром", ""},
		{"observation", NoteInput{Kind: NoteObservation, Body: "x"}, "x", ""},
		{"empty", NoteInput{Kind: NoteInsight, Body: "  \n "}, "", "empty_text"},
		{"bad kind", NoteInput{Kind: "idea", Body: "x"}, "", "invalid_kind"},
		{"too long", NoteInput{Kind: NoteInsight, Body: strings.Repeat("x", MaxNoteLen+1)}, "", "text_too_long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Normalize()
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err == nil && got.Body != tt.wantBody {
				t.Errorf("body = %q, want %q", got.Body, tt.wantBody)
			}
		})
	}
}

func TestCheckOrder(t *testing.T) {
	existing := []int64{1, 2, 3}
	tests := []struct {
		name string
		ids  []int64
		ok   bool
	}{
		{"permutation", []int64{3, 1, 2}, true},
		{"same", []int64{1, 2, 3}, true},
		{"missing", []int64{1, 2}, false},
		{"extra", []int64{1, 2, 3, 4}, false},
		{"duplicate", []int64{1, 1, 2}, false},
		{"unknown", []int64{1, 2, 9}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckOrder(tt.ids, existing)
			if (err == nil) != tt.ok {
				t.Errorf("err = %v, want ok=%v", err, tt.ok)
			}
			if err != nil && errCode(err) != "invalid_order" {
				t.Errorf("code = %q", errCode(err))
			}
		})
	}
}
