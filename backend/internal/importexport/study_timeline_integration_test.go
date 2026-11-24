//go:build integration

package importexport

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"lexis/internal/ambitions"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/study"
)

func TestStudyTimelineBackup(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	clk := clock.NewFixed(time.Date(2026, 12, 31, 22, 0, 0, 0, time.UTC).In(time.FixedZone("MSK", 10800)))
	a := ambitions.NewService(pool, clk)
	learning := study.NewService(pool, clk)
	backups := NewService(pool, clk, nil)
	v, err := a.Change(ctx, "dreams", 0, "POST", ambitions.Input{Fields: ambitions.Fields{Title: "Study probability"}, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	dreamID := v.Dreams[0].ID
	v, err = a.Change(ctx, "goals", 0, "POST", ambitions.Input{Year: 2027, Month: new(int32(1)), DreamID: &dreamID})
	if err != nil {
		t.Fatal(err)
	}
	if v.Today != "2027-01-01" {
		t.Fatalf("server timezone: %s", v.Today)
	}
	v, err = a.Change(ctx, "years", 0, "POST", ambitions.Input{Year: 2040})
	if err != nil {
		t.Fatal(err)
	}
	st, err := learning.Change(ctx, "groups", 0, "POST", study.Input{Title: "Math"})
	if err != nil {
		t.Fatal(err)
	}
	parent := st.Groups[0].ID
	st, err = learning.Change(ctx, "groups", 0, "POST", study.Input{Title: "Theorems", ParentID: &parent})
	if err != nil {
		t.Fatal(err)
	}
	child := st.Groups[1].ID
	text := "[тип: теорема]\n  $||X||$  \n---\n**Answer**\n\n```text\n---\n[type: lemma]\n```"
	st, err = learning.Change(ctx, "bulk", 0, "POST", study.Input{GroupID: child, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if st.Cards[0].Front != "  $||X||$  " {
		t.Fatal("source changed")
	}
	if _, err = learning.Change(ctx, "groups", parent, "DELETE", study.Input{}); err == nil {
		t.Fatal("nonempty group deleted")
	}
	backup, err := backups.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Version != BackupVersion || backup.Study == nil || backup.Ambitions.Timeline == nil {
		t.Fatal("missing new sections")
	}
	raw, _ := json.Marshal(backup)
	decoded, err := DecodeBackup(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{1, 2, 3, 4} {
		old := Backup{Version: version, Settings: map[string]json.RawMessage{}}
		if version == 4 {
			snap := *backup.Ambitions
			snap.Timeline = nil
			old.Ambitions = &snap
		}
		if _, err = backups.Restore(ctx, old); err != nil {
			t.Fatalf("restore v%d: %v", version, err)
		}
		current, e := backups.ExportBackup(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(current.Study, backup.Study) || !reflect.DeepEqual(current.Ambitions.Timeline, backup.Ambitions.Timeline) {
			t.Fatalf("v%d erased new data", version)
		}
	}
	if err = backups.Reset(ctx, ScopeEnglish); err != nil {
		t.Fatal(err)
	}
	current, err := backups.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.Study, backup.Study) || !reflect.DeepEqual(current.Ambitions.Timeline, backup.Ambitions.Timeline) {
		t.Fatal("English reset erased data")
	}
	if err = backups.Reset(ctx, ScopeAll); err != nil {
		t.Fatal(err)
	}
	current, err = backups.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Study.Cards) != 0 || len(current.Study.Groups) != 0 || len(current.Ambitions.Timeline.Goals) != 0 || len(current.Ambitions.Timeline.Years) != 0 {
		t.Fatal("reset all missed data")
	}
	// Reversed group order must still restore deferred foreign keys correctly.
	decoded.Study.Groups[0], decoded.Study.Groups[1] = decoded.Study.Groups[1], decoded.Study.Groups[0]
	if _, err = backups.Restore(ctx, decoded); err != nil {
		t.Fatal(err)
	}
	current, err = backups.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.Study, backup.Study) || !reflect.DeepEqual(current.Ambitions, backup.Ambitions) {
		t.Fatal("roundtrip differs")
	}
	// Late DB constraint failure rolls back study/timeline writes as well.
	broken := backup
	newStudy := *backup.Study
	newStudy.Cards = append([]study.Card{}, newStudy.Cards...)
	newStudy.Cards[0].Front = "changed"
	broken.Study = &newStudy
	broken.Sport = &BackupSport{Workouts: []BackupWorkout{{Day: "2026-11-09", Title: "", Notes: ""}}}
	if _, err = backups.Restore(ctx, broken); err == nil {
		t.Fatal("expected rollback")
	}
	current, err = backups.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.Study, backup.Study) {
		t.Fatal("partial restore")
	}
	// Deleting the dream must retain the timeline placement, without a dangling link.
	v, err = a.Change(ctx, "dreams", dreamID, "DELETE", ambitions.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Timeline.Goals) != 1 || v.Timeline.Goals[0].DreamID != nil {
		t.Fatal("dream delete lost placement")
	}
	// Restoring an older snapshot with overlapping entity IDs cannot corrupt a retained timeline.
	old := Backup{Version: 4, Settings: map[string]json.RawMessage{}, Ambitions: &ambitions.Snapshot{Dreams: []ambitions.Dream{{ID: v.Timeline.Goals[0].ID, Fields: ambitions.Fields{Title: "Old dream"}, Status: "want"}}}}
	if _, err = backups.Restore(ctx, old); err != nil {
		t.Fatal(err)
	}
	v, err = a.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = v.Snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
}
