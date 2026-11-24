package study

import (
	"lexis/internal/platform/clock"
	"testing"
	"time"
)

func TestFillStats(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	from, _ := clock.ParseDate("2026-11-10")
	to, _ := clock.ParseDate("2026-11-13")
	tests := []struct {
		name                       string
		reviews                    []Review
		total, known, active, best int
		daily                      []int
	}{
		{"empty", nil, 0, 0, 0, 0, []int{0, 0, 0, 0}},
		{"midnight and repeats", []Review{
			{ReviewInput: ReviewInput{Known: true}, ReviewedAt: time.Date(2026, 11, 9, 21, 0, 0, 0, time.UTC)},
			{ReviewInput: ReviewInput{Known: false}, ReviewedAt: time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)},
			{ReviewInput: ReviewInput{Known: true}, ReviewedAt: time.Date(2026, 11, 10, 21, 0, 0, 0, time.UTC)},
			{ReviewInput: ReviewInput{Known: true}, ReviewedAt: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)},
			{ReviewedAt: time.Date(2026, 11, 9, 20, 59, 0, 0, time.UTC)},
			{ReviewedAt: time.Date(2026, 11, 13, 21, 0, 0, 0, time.UTC)},
		}, 4, 3, 3, 2, []int{2, 1, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := FillStats(tt.reviews, from, to, loc)
			if s.Total != tt.total || s.Known != tt.known || s.ActiveDays != tt.active || s.BestStreak != tt.best || len(s.Days) != 4 {
				t.Fatalf("%+v", s)
			}
			for i, n := range tt.daily {
				if s.Days[i].Total != n {
					t.Fatal(s.Days)
				}
			}
		})
	}
}
func TestValidateHistory(t *testing.T) {
	good := Review{ReviewInput: ReviewInput{ID: "c4444444-4444-4444-8444-444444444444", CardID: 1, Known: true}, ReviewedAt: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}
	if err := ValidateHistory([]Review{good}); err != nil {
		t.Fatal(err)
	}
	for _, reviews := range [][]Review{{good, good}, {{ReviewInput: good.ReviewInput}}, {{ReviewInput: ReviewInput{ID: "bad", CardID: 1}, ReviewedAt: good.ReviewedAt}}} {
		if ValidateHistory(reviews) == nil {
			t.Fatal("invalid history accepted")
		}
	}
}
