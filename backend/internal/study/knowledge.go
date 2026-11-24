package study

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"lexis/internal/groups/tree"
	"lexis/internal/platform/httpx"
	"lexis/internal/study/store"
)

type Knowledge struct {
	CardID         int64      `json:"cardId"`
	GroupID        int64      `json:"groupId"`
	Front          string     `json:"front"`
	Kind           string     `json:"kind"`
	GroupTitle     string     `json:"groupTitle"`
	Score          int64      `json:"score"`
	Total          int64      `json:"total"`
	Known          int64      `json:"known"`
	LastReviewedAt *time.Time `json:"lastReviewedAt"`
}

func ReadKnowledge(ctx context.Context, db store.DBTX) ([]Knowledge, error) {
	rows, err := store.New(db).ListKnowledge(ctx)
	if err != nil {
		return nil, fmt.Errorf("study knowledge: %w", err)
	}
	out := make([]Knowledge, 0, len(rows))
	for _, r := range rows {
		var last *time.Time
		if r.Total > 0 {
			value := r.LastReviewedAt
			last = &value
		}
		out = append(out, Knowledge{CardID: r.CardID, GroupID: r.GroupID, Front: r.Front, Kind: r.Kind, GroupTitle: r.GroupTitle, Score: r.Score, Total: r.Total, Known: r.Known, LastReviewedAt: last})
	}
	return out, nil
}
func (s *Service) Knowledge(ctx context.Context) ([]Knowledge, error) {
	return ReadKnowledge(ctx, s.pool)
}

type PracticeInput struct {
	GroupID *int64  `json:"groupId"`
	CardIDs []int64 `json:"cardIds"`
	Limit   int     `json:"limit"`
}
type PracticeCard struct {
	Card
	Score int64 `json:"score"`
}
type Selection struct {
	Cards     []PracticeCard `json:"cards"`
	Available int            `json:"available"`
}

func (in PracticeInput) Validate() error {
	if in.Limit < 1 || in.Limit > 10000 || len(in.CardIDs) > 10000 {
		return bad("invalid practice limit")
	}
	ids := map[int64]bool{}
	for _, id := range in.CardIDs {
		if id < 1 || id > 9007199254740991 || ids[id] {
			return bad("invalid card selection")
		}
		ids[id] = true
	}
	if in.GroupID != nil && (*in.GroupID < 1 || *in.GroupID > 9007199254740991) {
		return bad("invalid practice group")
	}
	return nil
}

// SelectPractice ranks the eligible subtree/selection by score, then least recently reviewed.
func SelectPractice(s Snapshot, knowledge []Knowledge, in PracticeInput) (Selection, error) {
	out := Selection{Cards: []PracticeCard{}}
	if err := in.Validate(); err != nil {
		return out, err
	}
	groups := map[int64]bool{}
	if in.GroupID != nil {
		nodes := []tree.Node{}
		found := false
		for _, g := range s.Groups {
			p := int64(0)
			if g.ParentID != nil {
				p = *g.ParentID
			}
			nodes = append(nodes, tree.Node{ID: g.ID, Parent: p})
			if g.ID == *in.GroupID {
				found = true
			}
		}
		if !found {
			return out, httpx.NotFound("practice group not found")
		}
		for _, id := range tree.New(nodes).Subtree(*in.GroupID) {
			groups[id] = true
		}
	}
	chosen := map[int64]bool{}
	for _, id := range in.CardIDs {
		chosen[id] = true
	}
	known := map[int64]Knowledge{}
	for _, k := range knowledge {
		known[k.CardID] = k
	}
	eligible := map[int64]bool{}
	for _, c := range s.Cards {
		if c.Kind == GrammarRule || (in.GroupID != nil && !groups[c.GroupID]) {
			continue
		}
		eligible[c.ID] = true
		if len(chosen) > 0 && !chosen[c.ID] {
			continue
		}
		out.Cards = append(out.Cards, PracticeCard{Card: c, Score: known[c.ID].Score})
	}
	for id := range chosen {
		if !eligible[id] {
			return Selection{}, bad("selected card is outside the group")
		}
	}
	slices.SortFunc(out.Cards, func(a, b PracticeCard) int {
		if n := cmp.Compare(a.Score, b.Score); n != 0 {
			return n
		}
		at, bt := known[a.ID].LastReviewedAt, known[b.ID].LastReviewedAt
		if at == nil && bt != nil {
			return -1
		}
		if at != nil && bt == nil {
			return 1
		}
		if at != nil && bt != nil {
			if n := at.Compare(*bt); n != 0 {
				return n
			}
		}
		return cmp.Compare(a.ID, b.ID)
	})
	out.Available = len(out.Cards)
	out.Cards = out.Cards[:min(in.Limit, len(out.Cards))]
	return out, nil
}
func (s *Service) Practice(ctx context.Context, in PracticeInput) (Selection, error) {
	if err := in.Validate(); err != nil {
		return Selection{}, err
	}
	var result Selection
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		snap, err := Read(ctx, tx)
		if err != nil {
			return err
		}
		knowledge, err := ReadKnowledge(ctx, tx)
		if err != nil {
			return err
		}
		result, err = SelectPractice(snap, knowledge, in)
		return err
	})
	if err != nil {
		return Selection{}, fmt.Errorf("study practice: %w", err)
	}
	return result, nil
}
