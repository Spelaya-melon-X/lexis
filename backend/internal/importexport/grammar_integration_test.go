//go:build integration

package importexport

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"lexis/internal/platform/clock"
	"lexis/internal/study"
	"lexis/migrations"
)

func TestGrammarUpgradeAndBackup(t *testing.T) {
	backupService, wordService, fx := newTestEnv(t)
	ctx := context.Background()
	sqlDB := stdlib.OpenDBFromPool(fx.Pool)
	defer sqlDB.Close()
	migrator, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrator.DownTo(ctx, 16); err != nil {
		t.Fatal(err)
	}

	seed(t, wordService, fx)
	fx.Exec(t, `UPDATE training_sessions SET strategy = 'context' WHERE finished_at IS NOT NULL`)
	fx.Exec(t, `INSERT INTO study_groups (id, title) VALUES (nextval('study_ids'), 'Math')`)
	fx.Exec(t, `INSERT INTO study_cards (id, group_id, kind, front, back)
	            VALUES (nextval('study_ids'), 1, 'definition', 'Definition', 'Explanation')`)
	fx.Exec(t, `INSERT INTO study_reviews (id, card_id, reviewed_at, known, reverse)
	            VALUES ('a1111111-1111-4111-8111-111111111111', 2, $1, true, false)`, testDay)
	legacy := func() string {
		t.Helper()
		var rows string
		err := fx.Pool.QueryRow(ctx, `SELECT jsonb_build_array(
			(SELECT jsonb_agg(w ORDER BY w.id) FROM words w),
			(SELECT jsonb_agg(e ORDER BY e.id) FROM word_examples e),
			(SELECT jsonb_agg(g ORDER BY g.id) FROM groups g),
			(SELECT jsonb_agg(wg ORDER BY wg.word_id, wg.group_id) FROM word_groups wg),
			(SELECT jsonb_agg(s ORDER BY s.id) FROM training_sessions s),
			(SELECT jsonb_agg(r ORDER BY r.id) FROM reviews r),
			(SELECT jsonb_agg(sr ORDER BY sr.id) FROM study_reviews sr)
		)::text`).Scan(&rows)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := legacy()
	if _, err := migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if legacy() != before {
		t.Fatal("migration changed vocabulary, groups, scheduling data or review history")
	}

	service := study.NewService(fx.Pool, clock.NewFixed(testDay))
	library, err := service.Get(ctx)
	if err != nil || len(library.Groups) != 1 || library.Groups[0].Grammar || len(library.Cards) != 1 ||
		library.Cards[0] != (study.Card{ID: 2, GroupID: 1, Kind: "definition", Front: "Definition", Back: "Explanation"}) {
		t.Fatalf("legacy study library = %+v, %v", library, err)
	}

	change := func(entity string, id int64, method string, input study.Input) study.Snapshot {
		t.Helper()
		next, err := service.Change(ctx, entity, id, method, input)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	topics := change("groups", 0, http.MethodPost, study.Input{Title: "Modal verbs", Grammar: true})
	t.Cleanup(func() {
		if err := backupService.Reset(ctx, ScopeAll); err != nil {
			t.Errorf("clean test data for migration rollback: %v", err)
		}
	})
	topicID := topics.Groups[1].ID
	updated := change("groups", topicID, http.MethodPut, study.Input{Title: "Modal constructions"})
	if !updated.Groups[1].Grammar {
		t.Fatal("renaming a topic discarded its grammar type")
	}
	content := &study.GrammarContent{
		Formula:  "Subject + should + base verb",
		Examples: []study.GrammarExample{{Text: "You should study.", Translation: "Тебе следует заниматься."}},
		Mistakes: []study.GrammarMistake{{Incorrect: "You should to study.", Correct: "You should study."}},
	}
	rule := study.Input{GroupID: topicID, Kind: study.GrammarRule, Front: "Should + verb", Back: "**Advice**\n\n- Item", Content: content}
	wrongTopic := rule
	wrongTopic.GroupID = 1
	if _, err := service.Change(ctx, "cards", 0, http.MethodPost, wrongTopic); err == nil {
		t.Fatal("accepted a grammar rule in a math group")
	}
	created := change("cards", 0, http.MethodPost, rule)
	ruleID := created.Cards[1].ID
	rule.Back = "**Advice**\n\n1. Use the base form."
	change("cards", ruleID, http.MethodPut, rule)
	if _, err := service.Change(ctx, "groups", topicID, http.MethodDelete, study.Input{}); err == nil {
		t.Fatal("deleted a topic with a rule")
	}
	batch, err := service.Practice(ctx, study.PracticeInput{Limit: 10})
	if err != nil || len(batch.Cards) != 1 || batch.Cards[0].Kind != "definition" || batch.Cards[0].Score != 1 {
		t.Fatalf("math practice = %+v, %v", batch, err)
	}
	knowledge, err := service.Knowledge(ctx)
	if err != nil || len(knowledge) != 1 || knowledge[0].Total != 1 {
		t.Fatalf("math knowledge = %+v, %v", knowledge, err)
	}
	if err := service.Record(ctx, study.ReviewInput{ID: "a2222222-2222-4222-8222-222222222222", CardID: ruleID, Known: true}); err == nil {
		t.Fatal("recorded an SRS-style answer for a reading card")
	}
	if _, err := migrator.DownTo(ctx, 16); err == nil {
		t.Fatal("downgrade discarded grammar data")
	}
	exported, err := backupService.ExportBackup(ctx)
	if err != nil || exported.Version != 7 || exported.Study == nil || len(exported.Study.Cards) != 2 {
		t.Fatalf("grammar backup: version=%d, err=%v", exported.Version, err)
	}
	if got := exported.Study.Cards[1]; got.Back != rule.Back || !reflect.DeepEqual(got.Content, content) {
		t.Fatalf("saved rule = %+v", got)
	}
	if len(exported.Sessions) != 2 || exported.Sessions[0].Strategy != "context" {
		t.Fatal("legacy training strategy was not retained in the backup")
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBackup(raw)
	if err != nil {
		t.Fatal(err)
	}
	change("cards", ruleID, http.MethodDelete, study.Input{})
	change("groups", topicID, http.MethodDelete, study.Input{})
	if _, err := backupService.Restore(ctx, decoded); err != nil {
		t.Fatal(err)
	}
	library, err = service.Get(ctx)
	if err != nil || !reflect.DeepEqual(library, *exported.Study) {
		t.Fatalf("study restore = %+v, %v", library, err)
	}

	legacyBackup := exported
	legacyBackup.Version = 6
	legacyBackup.Study = &study.Snapshot{
		Groups: []study.Group{exported.Study.Groups[0]},
		Cards: []study.Card{
			exported.Study.Cards[0],
			{ID: ruleID, GroupID: 1, Kind: "lemma", Front: "Legacy math card", Back: "Explanation"},
		},
	}
	if _, err := backupService.Restore(ctx, legacyBackup); err != nil {
		t.Fatal(err)
	}
	library, err = service.Get(ctx)
	if err != nil || len(library.Cards) != 3 || library.Cards[1] != legacyBackup.Study.Cards[1] {
		t.Fatalf("legacy study restore = %+v, %v", library, err)
	}
	preserved := library.Cards[2]
	if preserved.ID == ruleID || preserved.Back != rule.Back || preserved.GroupID != library.Groups[1].ID ||
		!reflect.DeepEqual(preserved.Content, content) {
		t.Fatalf("grammar after restoring old backup = %+v", preserved)
	}
}
