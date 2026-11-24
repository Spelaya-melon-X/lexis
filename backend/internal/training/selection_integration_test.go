//go:build integration

package training

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"lexis/internal/importexport"
)

func TestService_SelectedWords(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	group := env.group(t, "selected")
	child := env.group(t, "child")
	env.exec(t, `UPDATE groups SET parent_id=$1 WHERE id=$2`, group, child)
	other := env.group(t, "other")
	due := env.word(t, group, "due", 2, at(8, 0, 0))
	notSelected := env.word(t, group, "not-selected", 2, at(8, 0, 0))
	future := env.word(t, child, "future", 3, at(20, 0, 0))
	archived := env.word(t, child, "archived", 5, at(1, 0, 0))
	foreign := env.word(t, other, "foreign", 2, at(8, 0, 0))

	t.Run("invalid selections create no session", func(t *testing.T) {
		before := env.count(t, `SELECT count(*) FROM training_sessions`)
		for _, selected := range [][]int64{{}, {due, foreign}, {due, 999999}, {0}, {-1}} {
			_, err := env.svc.Start(ctx, StartParams{GroupID: group, Strategy: StrategyEnRu, WordIDs: selected})
			assertHTTPError(t, err, 400, "invalid_word_selection")
		}
		if env.count(t, `SELECT count(*) FROM training_sessions`) != before {
			t.Fatal("invalid start persisted a session")
		}
	})
	t.Run("normal filters before scheduling and deduplicates", func(t *testing.T) {
		res := env.mustStart(t, StartParams{GroupID: group, Strategy: StrategyEnRu, WordIDs: []int64{due, due, future, archived}})
		if !reflect.DeepEqual(ids(res.Cards), []int64{due}) {
			t.Fatalf("cards=%v", ids(res.Cards))
		}
		env.mustAnswer(t, *res.SessionID, due, AnswerAgain)
		repeated := env.mustAnswer(t, *res.SessionID, due, AnswerGood)
		if repeated.FirstInSession || repeated.LevelBefore != repeated.LevelAfter {
			t.Fatal(repeated)
		}
		finish, err := env.svc.Finish(ctx, *res.SessionID)
		if err != nil || finish.Total != 1 || finish.Again != 1 || finish.Good != 0 {
			t.Fatalf("finish=%+v err=%v", finish, err)
		}
		if env.srs(t, notSelected).Level != 2 {
			t.Fatal("unselected SRS changed")
		}
	})
	t.Run("only not-due selected creates no session", func(t *testing.T) {
		res := env.mustStart(t, StartParams{GroupID: group, Strategy: StrategyRuEn, WordIDs: []int64{future, archived}})
		if res.SessionID != nil || len(res.Cards) != 0 {
			t.Fatal(res)
		}
	})
	t.Run("free includes all selected and never changes SRS", func(t *testing.T) {
		beforeFuture, beforeArchive := env.srs(t, future), env.srs(t, archived)
		res := env.mustStart(t, StartParams{GroupID: group, Strategy: StrategyRuEn, Free: true, WordIDs: []int64{archived, future, archived}})
		expected := []int64{future, archived}
		got := ids(res.Cards)
		slices.Sort(got)
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("cards=%v want=%v", ids(res.Cards), expected)
		}
		env.mustAnswer(t, *res.SessionID, future, AnswerGood)
		env.mustAnswer(t, *res.SessionID, archived, AnswerAgain)
		_, err := env.svc.Answer(ctx, *res.SessionID, AnswerParams{WordID: foreign, Answer: AnswerGood})
		assertHTTPError(t, err, 400, "word_not_in_session")
		finish, err := env.svc.Finish(ctx, *res.SessionID)
		if err != nil || !finish.Free || finish.Total != 2 || len(finish.LevelChanges) != 0 {
			t.Fatalf("%+v %v", finish, err)
		}
		if !reflect.DeepEqual(beforeFuture, env.srs(t, future)) || !reflect.DeepEqual(beforeArchive, env.srs(t, archived)) {
			t.Fatal("free changed SRS")
		}
		backup, err := importexport.NewService(env.pool, env.clock, nil).ExportBackup(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range backup.Sessions {
			if s.ID == *res.SessionID {
				if len(s.Reviews) != 2 {
					t.Fatal("expected only actual reviews")
				}
				raw, _ := json.Marshal(s)
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				if _, ok := fields["wordIds"]; ok {
					t.Fatal("backup contains temporary selection")
				}
			}
		}
		// A later full-group session must not inherit the selection.
		full := env.mustStart(t, StartParams{GroupID: group, Strategy: StrategyEnRu, Free: true})
		if len(ids(full.Cards)) != 4 {
			t.Fatalf("later queue=%v", ids(full.Cards))
		}
	})
}

func TestService_SelectedWordExamplesAndNewLimit(t *testing.T) {
	env := newTestEnv(t)
	group := env.group(t, "context")
	verb := env.word(t, group, "go_went_gone", 1, at(1, 0, 0), "I [go] home", "I [went] home")
	env.exec(t, `UPDATE words SET forms=ARRAY['go','went','gone'] WHERE id=$1`, verb)
	env.word(t, group, "excluded", 1, at(1, 0, 0), "An [excluded] word")
	res := env.mustStart(t, StartParams{GroupID: group, Strategy: StrategyContext, Free: true, WordIDs: []int64{verb, verb}})
	if len(res.Cards) != 1 {
		t.Fatalf("verb cards=%d", len(res.Cards))
	}
	for _, c := range res.Cards {
		if c.WordID != verb || len(c.Examples) != 2 {
			t.Fatal(c)
		}
	}
	env.exec(t, `INSERT INTO settings(key,value) VALUES('new_words_limit','0') ON CONFLICT(key) DO UPDATE SET value='0'`)
	ordinary := env.mustStart(t, StartParams{GroupID: group, Strategy: StrategyEnRu, WordIDs: []int64{verb}})
	if ordinary.SessionID != nil || len(ordinary.Cards) != 0 {
		t.Fatal("selection bypassed new-word limit")
	}
}
