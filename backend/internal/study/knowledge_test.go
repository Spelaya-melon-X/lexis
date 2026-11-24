package study

import (
	"reflect"
	"testing"
	"time"
)

func TestSelectPractice(t *testing.T) {
	snap := Empty()
	snap.Groups = []Group{{ID: 1, Title: "Math"}, {ID: 2, Title: "Child", ParentID: new(int64(1))}, {ID: 3, Title: "Other"}}
	for _, id := range []int64{10, 11, 12, 13, 14} {
		group := int64(1)
		if id == 11 {
			group = 2
		}
		if id == 14 {
			group = 3
		}
		snap.Cards = append(snap.Cards, Card{ID: id, GroupID: group, Kind: "lemma", Front: "A", Back: "B"})
	}
	recent := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	old := recent.AddDate(0, 0, -1)
	knowledge := []Knowledge{{CardID: 10, Score: 5, LastReviewedAt: &old}, {CardID: 11, Score: -2, LastReviewedAt: &recent}, {CardID: 12, Score: 0, LastReviewedAt: &old}, {CardID: 13, Score: 0}, {CardID: 14, Score: -9}}
	tests := []struct {
		name      string
		in        PracticeInput
		want      []int64
		available int
		invalid   bool
	}{
		{"lowest within subtree", PracticeInput{GroupID: new(int64(1)), Limit: 2}, []int64{11, 13}, 4, false},
		{"all library", PracticeInput{Limit: 10}, []int64{14, 11, 13, 12, 10}, 5, false},
		{"selected only", PracticeInput{GroupID: new(int64(1)), CardIDs: []int64{12, 10}, Limit: 1}, []int64{12}, 2, false},
		{"child only", PracticeInput{GroupID: new(int64(2)), Limit: 10}, []int64{11}, 1, false},
		{"invalid limit", PracticeInput{Limit: 0}, nil, 0, true},
		{"foreign selection", PracticeInput{GroupID: new(int64(1)), CardIDs: []int64{14}, Limit: 2}, nil, 0, true},
		{"duplicate selection", PracticeInput{CardIDs: []int64{10, 10}, Limit: 2}, nil, 0, true},
		{"missing group", PracticeInput{GroupID: new(int64(99)), Limit: 2}, nil, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectPractice(snap, knowledge, tt.in)
			if tt.invalid {
				if err == nil {
					t.Fatal("accepted invalid selection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ids := []int64{}
			for _, c := range got.Cards {
				ids = append(ids, c.ID)
			}
			if !reflect.DeepEqual(ids, tt.want) || got.Available != tt.available {
				t.Fatal(got)
			}
		})
	}
	if snap.Cards[0].ID != 10 {
		t.Fatal("selection mutated library")
	}
}
