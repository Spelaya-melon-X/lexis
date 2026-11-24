package importexport

import (
	"encoding/json"
	"testing"

	"lexis/internal/ambitions"
	"lexis/internal/study"
)

func TestNewBackupSectionsValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Backup)
		valid  bool
	}{
		{"v5", func(b *Backup) { s := study.Empty(); b.Study = &s; a := ambitions.Empty(); b.Ambitions = &a }, true},
		{"old study", func(b *Backup) { b.Version = 4; s := study.Empty(); b.Study = &s }, false},
		{"old timeline", func(b *Backup) { b.Version = 4; a := ambitions.Empty(); b.Ambitions = &a }, false},
		{"dangling group", func(b *Backup) {
			s := study.Empty()
			s.Cards = append(s.Cards, study.Card{ID: 1, GroupID: 9, Kind: "theorem", Front: "A", Back: "B"})
			b.Study = &s
		}, false},
		{"dangling dream", func(b *Backup) {
			a := ambitions.Empty()
			a.Timeline.Goals = append(a.Timeline.Goals, ambitions.TimelineGoal{ID: 1, Year: 2026, Title: "A", DreamID: new(int64(9))})
			b.Ambitions = &a
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBackup()
			tt.mutate(&b)
			raw, _ := json.Marshal(b)
			_, err := DecodeBackup(raw)
			if (err == nil) != tt.valid {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestStudyHistoryVersion(t *testing.T) {
	b := validBackup()
	history := []study.Review{}
	b.StudyHistory = &history
	b.Version = 5
	raw, _ := json.Marshal(b)
	if _, err := DecodeBackup(raw); err == nil {
		t.Fatal("v5 accepted review history")
	}
	b.Version = 6
	raw, _ = json.Marshal(b)
	if _, err := DecodeBackup(raw); err != nil {
		t.Fatal(err)
	}
}
