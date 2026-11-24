//go:build integration

package study

import (
	"context"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"testing"
	"time"
)

func TestKnowledgeScoreAndSelection(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	clk := clock.NewFixed(time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC))
	s := NewService(pool, clk)
	snap, err := s.Change(ctx, "groups", 0, "POST", Input{Title: "Math"})
	if err != nil {
		t.Fatal(err)
	}
	group := snap.Groups[0].ID
	for _, front := range []string{"A", "B", "C"} {
		snap, err = s.Change(ctx, "cards", 0, "POST", Input{GroupID: group, Kind: "lemma", Front: front, Back: "Answer"})
		if err != nil {
			t.Fatal(err)
		}
	}
	a, b := snap.Cards[0].ID, snap.Cards[1].ID
	input := ReviewInput{ID: "a1111111-1111-4111-8111-111111111111", CardID: a, Known: false}
	for i := 0; i < 2; i++ {
		if err := s.Record(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Record(ctx, ReviewInput{ID: "b1111111-1111-4111-8111-111111111111", CardID: b, Known: true}); err != nil {
		t.Fatal(err)
	}
	k, err := s.Knowledge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if k[0].Score != -1 || k[0].Total != 1 || k[1].Score != 1 || k[2].Score != 0 || k[2].LastReviewedAt != nil {
		t.Fatal(k)
	}
	selected, err := s.Practice(ctx, PracticeInput{GroupID: &group, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Cards[0].ID != a || selected.Cards[1].ID != snap.Cards[2].ID {
		t.Fatal(selected)
	}
	// A real repetition changes the score; retrying the same answer does not.
	input.ID = "a2222222-2222-4222-8222-222222222222"
	input.Known = true
	if err := s.Record(ctx, input); err != nil {
		t.Fatal(err)
	}
	k, err = s.Knowledge(ctx)
	if err != nil || k[0].Score != 0 || k[0].Total != 2 {
		t.Fatal(k, err)
	}
	history, err := ReadHistory(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteHistory(ctx, pool, history); err != nil {
		t.Fatal(err)
	}
	k, err = s.Knowledge(ctx)
	if err != nil || k[0].Score != 0 || k[0].Total != 2 {
		t.Fatal("score not reconstructed", k, err)
	}
	if _, err := s.Change(ctx, "cards", a, "DELETE", Input{}); err != nil {
		t.Fatal(err)
	}
	k, err = s.Knowledge(ctx)
	if err != nil || len(k) != 2 {
		t.Fatal(k, err)
	}
	history, err = ReadHistory(ctx, pool)
	if err != nil || len(history) != 3 {
		t.Fatal("deleting card erased history", err)
	}
}
