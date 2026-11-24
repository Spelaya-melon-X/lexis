//go:build integration

package study

import (
	"context"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"sync"
	"testing"
	"time"
)

func TestReviewPersistence(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Moscow")
	clk := clock.NewFixed(time.Date(2026, 11, 12, 23, 59, 0, 0, loc))
	s := NewService(pool, clk)
	in := ReviewInput{ID: "a1111111-1111-4111-8111-111111111111", CardID: 1, Known: true}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Go(func() { errs <- s.Record(ctx, in) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	clk.Advance(2 * time.Minute)
	if err := s.Record(ctx, in); err != nil {
		t.Fatal(err)
	} // retry after midnight preserves the original day.
	bad := in
	bad.Known = false
	if s.Record(ctx, bad) == nil {
		t.Fatal("conflicting retry accepted")
	}
	second := in
	second.ID = "a2222222-2222-4222-8222-222222222222"
	second.Known = false
	second.Reverse = true
	if err := s.Record(ctx, second); err != nil {
		t.Fatal(err)
	}
	got, err := s.Stats(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Known != 1 || got.ActiveDays != 2 || got.BestStreak != 2 || got.Days[5].Total != 1 || got.Days[6].Total != 1 {
		t.Fatalf("%+v", got)
	}
	if _, err := s.Stats(ctx, 10000); err == nil {
		t.Fatal("unbounded stats range")
	}
	history, err := ReadHistory(ctx, pool)
	if err != nil || len(history) != 2 {
		t.Fatal(err, history)
	}
}
