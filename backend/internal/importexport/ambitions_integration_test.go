//go:build integration

package importexport

import (
	"context"
	"encoding/json"
	"lexis/internal/ambitions"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestAmbitionsBackup(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	clk := clock.NewFixed(time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC))
	a := ambitions.NewService(pool, clk)
	s := NewService(pool, clk, nil)
	change := func(kind string, id int64, method string, in ambitions.Input) ambitions.View {
		t.Helper()
		v, err := a.Change(ctx, kind, id, method, in)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v := change("categories", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Travel"}})
	cid := v.Categories[0].ID
	v = change("projects", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Trip"}})
	pid := v.Projects[0].ID
	v = change("dreams", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Paris"}, Status: "want", CategoryID: &cid, ProjectID: &pid})
	v = change("steps", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Prepare"}, ProjectID: &pid})
	parent := v.Steps[0].ID
	v = change("steps", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Visa"}, ProjectID: &pid, ParentID: &parent})
	leaf := v.Steps[1].ID
	change("steps", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Book flight", Position: 1}, ProjectID: &pid, Requires: []int64{leaf}})
	backup, err := s.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Version != BackupVersion || backup.Ambitions == nil {
		t.Fatal(backup)
	}
	raw, _ := json.Marshal(backup)
	decoded, err := DecodeBackup(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Missing section in old backups must leave the new data intact.

	for _, version := range []int{1, 2, 3} {
		old := Backup{Version: version, Settings: backup.Settings}
		if _, err := s.Restore(ctx, old); err != nil {
			t.Fatal(err)
		}
		current, err := s.ExportBackup(ctx)
		if err != nil || !reflect.DeepEqual(current.Ambitions, backup.Ambitions) {
			t.Fatalf("v%d backup lost ambitions: %v", version, err)
		}
	}
	slices.Reverse(decoded.Ambitions.Steps) // child before parent must restore correctly.
	change("projects", pid, "DELETE", ambitions.Input{})
	if _, err := s.Restore(ctx, decoded); err != nil {
		t.Fatal(err)
	}
	current, err := s.ExportBackup(ctx)
	if err != nil || !reflect.DeepEqual(current.Ambitions, backup.Ambitions) {
		t.Fatalf("round trip: %v\n%+v\n%+v", err, current.Ambitions, backup.Ambitions)
	}
	// A database CHECK failing after ambitions were written must roll back the entire restore.
	broken := decoded
	snap := *decoded.Ambitions
	snap.Projects = slices.Clone(snap.Projects)
	snap.Projects[0].Title = "Must roll back"
	broken.Ambitions = &snap
	broken.Settings = map[string]json.RawMessage{"new_words_limit": json.RawMessage(`-100`)}
	// Force a late failure in sport restoration, after ambitions.Write.
	broken.Sport = &BackupSport{Workouts: []BackupWorkout{{Day: "2026-11-09", Title: "", Notes: ""}}}
	if _, err := s.Restore(ctx, broken); err == nil {
		t.Fatal("expected invalid backup")
	}
	current, err = s.ExportBackup(ctx)
	if err != nil || !reflect.DeepEqual(current.Ambitions, backup.Ambitions) {
		t.Fatal("rollback changed ambitions", err)
	}
	if err := s.Reset(ctx, ScopeEnglish); err != nil {
		t.Fatal(err)
	}
	current, _ = s.ExportBackup(ctx)
	if !reflect.DeepEqual(current.Ambitions, backup.Ambitions) {
		t.Fatal("english reset touched ambitions")
	}
	if err := s.Reset(ctx, ScopeAll); err != nil {
		t.Fatal(err)
	}
	current, _ = s.ExportBackup(ctx)
	if len(current.Ambitions.Dreams) != 0 || len(current.Ambitions.Steps) != 0 {
		t.Fatal("all reset left ambitions")
	}
}
