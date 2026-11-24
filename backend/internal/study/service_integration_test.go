//go:build integration

package study

import (
	"context"
	"encoding/json"
	"lexis/internal/platform/clock"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/db/dbtest"
)

func TestGrammarBulkHTTP(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM study_cards WHERE kind = 'grammar_rule'; DELETE FROM study_groups WHERE grammar"); err != nil {
			t.Error(err)
		}
	})
	router := chi.NewRouter()
	New(pool, nil).Routes(router)
	post := func(path string, input any, status int, output any) {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body))))
		if w.Code != status {
			t.Fatalf("POST %s: got %d, want %d (%s)", path, w.Code, status, w.Body.String())
		}
		if output != nil {
			if err := json.Unmarshal(w.Body.Bytes(), output); err != nil {
				t.Fatal(err)
			}
		}
	}
	var library Snapshot
	post("/groups", Input{Title: "Grammar", Grammar: true}, 201, &library)
	grammar := library.Groups[0].ID
	post("/groups", Input{Title: "Math"}, 201, &library)
	math := library.Groups[1].ID

	source := "[type: grammar_rule]\nShould\n---\nAdvice.\n[example] You should study. || Тебе следует заниматься.\n" +
		"[type: grammar_rule]\nAlthough\n---\nConcession."
	var preview ParseResult
	post("/parse", map[string]any{"text": source, "grammar": true}, 200, &preview)
	if len(preview.Cards) != 2 || len(preview.Errors) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	post("/bulk", Input{GroupID: grammar, Text: source + "\n[mistake] missing correction"}, 400, nil)
	post("/bulk", Input{GroupID: math, Text: source}, 400, nil)
	post("/bulk", Input{GroupID: grammar, Text: source}, 201, &library)
	if len(library.Cards) != 2 {
		t.Fatalf("saved %d cards; the invalid batches must create none", len(library.Cards))
	}
	for i, card := range library.Cards {
		if card.GroupID != grammar || card.Kind != GrammarRule || card.Content == nil || card.Front != preview.Cards[i].Front {
			t.Fatalf("saved card = %+v", card)
		}
	}
	if example := library.Cards[0].Content.Examples[0]; example.Translation != "Тебе следует заниматься." {
		t.Fatalf("saved example = %+v", example)
	}
}

func TestStudyTransactions(t *testing.T) {
	pool := dbtest.New(t)
	s := NewService(pool, clock.NewFixed(time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)))
	ctx := context.Background()
	st, err := s.Change(ctx, "groups", 0, "POST", Input{Title: "Math"})
	if err != nil {
		t.Fatal(err)
	}
	group := st.Groups[0].ID
	if _, err = s.Change(ctx, "bulk", 0, "POST", Input{GroupID: group, Text: "[type: theorem]\nA\n---\nB\n[type: unknown]\nC\n---\nD"}); err == nil {
		t.Fatal("bad batch saved")
	}
	st, err = s.Get(ctx)
	if err != nil || len(st.Cards) != 0 {
		t.Fatal("partial batch")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			_, e := s.Change(ctx, "cards", 0, "POST", Input{GroupID: group, Kind: "lemma", Front: "$x$", Back: "A"})
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	st, err = s.Get(ctx)
	if err != nil || len(st.Cards) != 8 {
		t.Fatalf("concurrent cards: %v %+v", err, st)
	}
	child, err := s.Change(ctx, "groups", 0, "POST", Input{Title: "Child", ParentID: &group})
	if err != nil {
		t.Fatal(err)
	}
	var childID int64
	for _, g := range child.Groups {
		if g.ParentID != nil {
			childID = g.ID
		}
	}
	if _, err = s.Change(ctx, "groups", group, "PUT", Input{Title: "Cycle", ParentID: &childID}); err == nil {
		t.Fatal("cycle saved")
	}
	st, err = s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	card := st.Cards[0]
	_, err = s.Change(ctx, "cards", card.ID, "PUT", Input{GroupID: childID, Kind: card.Kind, Front: card.Front, Back: card.Back})
	if err != nil {
		t.Fatal(err)
	}
}
