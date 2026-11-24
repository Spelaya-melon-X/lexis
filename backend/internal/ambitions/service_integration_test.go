//go:build integration

package ambitions

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
)

func TestServiceLifecycle(t *testing.T) {
	pool := dbtest.New(t)
	clk := clock.NewFixed(time.Date(2026, 11, 9, 0, 30, 0, 0, time.FixedZone("MSK", 10800)))
	svc := NewService(pool, clk)
	ctx := context.Background()
	change := func(kind string, id int64, method string, in Input) View {
		t.Helper()
		v, err := svc.Change(ctx, kind, id, method, in)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v := change("projects", 0, "POST", Input{Fields: Fields{Title: "Research"}})
	pid := v.Projects[0].ID
	v = change("steps", 0, "POST", Input{Fields: Fields{Title: "Topic"}, ProjectID: &pid})
	a := v.Steps[0].ID
	v = change("steps", 0, "POST", Input{Fields: Fields{Title: "Sources", Position: 1}, ProjectID: &pid, Requires: []int64{a}})
	b := v.Steps[1].ID
	if v.States[b].Ready || v.Today != "2026-11-09" {
		t.Fatalf("%+v", v)
	}
	_, err := svc.Change(ctx, "steps", b, "PUT", Input{Fields: Fields{Title: "Sources"}, ProjectID: &pid, Requires: []int64{a}, Done: true})
	if code(err) != "ambition_blocked" {
		t.Fatal(err)
	}
	change("steps", a, "PUT", Input{Fields: Fields{Title: "Topic"}, ProjectID: &pid, Done: true})
	v = change("steps", b, "PUT", Input{Fields: Fields{Title: "Sources", Position: 1}, ProjectID: &pid, Requires: []int64{a}, Done: true})
	if v.Summaries[pid].Progress != 100 {
		t.Fatal(v.Summaries)
	}
	_, err = svc.Change(ctx, "steps", a, "PUT", Input{Fields: Fields{Title: "Topic"}, ProjectID: &pid})
	if code(err) != "ambition_blocked" {
		t.Fatal(err)
	}
	v, err = NewService(pool, clk).Get(ctx)
	if err != nil || !v.States[a].Done {
		t.Fatalf("persisted: %v %v", v, err)
	}
	v = change("dreams", 0, "POST", Input{Fields: Fields{Title: "Publish research"}, ProjectID: &pid, Status: "active"})
	did := v.Dreams[0].ID
	v = change("projects", pid, "DELETE", Input{})
	if len(v.Steps) != 0 || len(v.Dependencies) != 0 || v.Dreams[0].ProjectID != nil {
		t.Fatal(v)
	}
	v, err = svc.Convert(ctx, did, Fields{Title: "New research"})
	if err != nil || len(v.Projects) != 1 || v.Dreams[0].ProjectID == nil {
		t.Fatalf("convert: %v %v", v, err)
	}
	// A second conversion must not create an orphan project.
	_, err = svc.Convert(ctx, did, Fields{Title: "Duplicate"})
	if code(err) != "ambition_linked" {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Route("/api/ambitions", New(pool, clk).Routes)
	req := httptest.NewRequest("POST", "/api/ambitions/dreams", strings.NewReader(`{"title":"Paris","unexpected":true}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

func TestConcurrentCreates(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	svc := NewService(pool, clock.NewFixed(time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)))
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			_, err := svc.Change(ctx, "dreams", 0, "POST", Input{Fields: Fields{Title: "An idea"}, Status: "want"})
			errors <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	v, err := svc.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Dreams) != 8 {
		t.Fatalf("lost concurrent writes: %d", len(v.Dreams))
	}
}
