//go:build integration

// Тренировка группы-родителя (§18.2): слова всего поддерева, каждое один раз.
package training

import (
	"context"
	"slices"
	"testing"
)

func TestService_Start_Subtree(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	travel := env.group(t, "travel")
	airport := env.group(t, "airport")
	hotel := env.group(t, "hotel")
	other := env.group(t, "other")
	env.exec(t, `UPDATE groups SET parent_id = $1 WHERE id IN ($2, $3)`, travel, airport, hotel)

	own := env.word(t, travel, "trip", 2, at(8, 9, 0))
	plane := env.word(t, airport, "plane", 2, at(8, 9, 0))
	env.exec(t, `INSERT INTO word_groups (word_id, group_id) VALUES ($1, $2)`, plane, hotel) // в двух подгруппах
	bed := env.word(t, hotel, "bed", 1, at(1, 9, 0))
	foreign := env.word(t, other, "foreign", 2, at(8, 9, 0))

	res := env.mustStart(t, StartParams{GroupID: travel, Strategy: StrategyEnRu})
	got := ids(res.Cards)
	slices.Sort(got)
	want := []int64{own, plane, bed}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("cards = %v, want %v (each word once)", got, want)
	}

	// Ответ на слово подгруппы засчитывается, на чужое — нет.
	env.mustAnswer(t, *res.SessionID, bed, AnswerGood)
	_, err := env.svc.Answer(ctx, *res.SessionID, AnswerParams{WordID: foreign, Answer: AnswerGood})
	assertHTTPError(t, err, 400, "word_not_in_session")

	// Подгруппа — только своё поддерево.
	sub := env.mustStart(t, StartParams{GroupID: airport, Strategy: StrategyEnRu, Free: true})
	if got := ids(sub.Cards); !slices.Equal(got, []int64{plane}) {
		t.Errorf("airport cards = %v, want [%d]", got, plane)
	}
}
