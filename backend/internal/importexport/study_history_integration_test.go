//go:build integration

package importexport

import (
	"context"
	"encoding/json"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/study"
	"testing"
	"time"
)

func TestStudyHistoryBackup(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	clk := clock.NewFixed(time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC))
	s := study.NewService(pool, clk)
	b := NewService(pool, clk, nil)
	if err := s.Record(ctx, study.ReviewInput{ID: "a1111111-1111-4111-8111-111111111111", CardID: 99, Known: true}); err != nil {
		t.Fatal(err)
	}
	backup, err := b.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Version != BackupVersion || backup.StudyHistory == nil || len(*backup.StudyHistory) != 1 {
		t.Fatal("missing history")
	}
	for _, version := range []int{1, 2, 3, 4, 5} {
		old := Backup{Version: version, Settings: map[string]json.RawMessage{}}
		if version == 5 {
			old.Study = backup.Study
		}
		if _, err := b.Restore(ctx, old); err != nil {
			t.Fatal(err)
		}
		stats, err := s.Stats(ctx, 30)
		if err != nil || stats.Total != 1 {
			t.Fatal("old backup erased history", version, err)
		}
	}
	raw, _ := json.Marshal(backup)
	decoded, err := DecodeBackup(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Reset(ctx, ScopeEnglish); err != nil {
		t.Fatal(err)
	}
	stats, err := s.Stats(ctx, 30)
	if err != nil || stats.Total != 1 {
		t.Fatal("English reset erased history", err)
	}
	if err := b.Reset(ctx, ScopeAll); err != nil {
		t.Fatal(err)
	}
	stats, err = s.Stats(ctx, 30)
	if err != nil || stats.Total != 0 {
		t.Fatal("all reset kept history", err)
	}
	if _, err := b.Restore(ctx, decoded); err != nil {
		t.Fatal(err)
	}
	stats, err = s.Stats(ctx, 30)
	if err != nil || stats.Total != 1 || stats.Known != 1 {
		t.Fatal("history roundtrip", err)
	}
	// A late error in a different section must roll back a replaced history too.
	broken := decoded
	empty := []study.Review{}
	broken.StudyHistory = &empty
	broken.Sport = &BackupSport{Workouts: []BackupWorkout{{Day: "2026-11-13", Title: ""}}}
	if _, err := b.Restore(ctx, broken); err == nil {
		t.Fatal("expected late error")
	}
	stats, err = s.Stats(ctx, 30)
	if err != nil || stats.Total != 1 {
		t.Fatal("partial history restore", err)
	}
	for _, version := range []int{1, 5} {
		bad := backup
		bad.Version = version
		raw, _ := json.Marshal(bad)
		if _, err := DecodeBackup(raw); err == nil {
			t.Fatal("new history allowed in old version")
		}
	}
}
