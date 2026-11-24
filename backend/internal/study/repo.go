package study

import (
	"context"
	"encoding/json"
	"fmt"

	"lexis/internal/study/store"
)

func Read(ctx context.Context, db store.DBTX) (Snapshot, error) {
	q := store.New(db)
	s := Empty()
	gs, err := q.ListGroups(ctx)
	if err != nil {
		return s, fmt.Errorf("study groups: %w", err)
	}
	cs, err := q.ListCards(ctx)
	if err != nil {
		return s, fmt.Errorf("study cards: %w", err)
	}
	for _, g := range gs {
		s.Groups = append(s.Groups, Group{ID: g.ID, Title: g.Title, ParentID: g.ParentID, Grammar: g.Grammar})
	}
	for _, c := range cs {
		card := Card{ID: c.ID, GroupID: c.GroupID, Kind: c.Kind, Front: c.Front, Back: c.Back}
		if c.GrammarContent != nil {
			if err := json.Unmarshal(c.GrammarContent, &card.Content); err != nil {
				return s, fmt.Errorf("read grammar rule %d: %w", c.ID, err)
			}
		}
		s.Cards = append(s.Cards, card)
	}
	return s, nil
}

// PreserveGrammar adds rules and topics from the current library to a pre-grammar backup.
// Restored study IDs stay intact because their reviews refer to those IDs.
func PreserveGrammar(ctx context.Context, db store.DBTX, restored Snapshot) (Snapshot, error) {
	current, err := Read(ctx, db)
	if err != nil {
		return Snapshot{}, err
	}
	used := make(map[int64]bool, len(restored.Groups)+len(restored.Cards))
	for _, g := range restored.Groups {
		used[g.ID] = true
	}
	for _, c := range restored.Cards {
		used[c.ID] = true
	}
	grammar := Empty()
	collision := false
	for _, g := range current.Groups {
		if g.Grammar {
			grammar.Groups = append(grammar.Groups, g)
			collision = collision || used[g.ID]
		}
	}
	for _, c := range current.Cards {
		if c.Kind == GrammarRule {
			grammar.Cards = append(grammar.Cards, c)
			collision = collision || used[c.ID]
		}
	}
	if len(grammar.Groups) == 0 {
		return restored, nil
	}
	if collision {
		q := store.New(db)
		if err := q.AdvanceIDs(ctx, max(current.MaxID(), restored.MaxID())); err != nil {
			return Snapshot{}, fmt.Errorf("reserve study ids: %w", err)
		}
		groupIDs := make(map[int64]int64, len(grammar.Groups))
		for i, g := range grammar.Groups {
			id, err := q.NextID(ctx)
			if err != nil {
				return Snapshot{}, fmt.Errorf("allocate grammar topic id: %w", err)
			}
			groupIDs[g.ID] = id
			grammar.Groups[i].ID = id
		}
		for i, g := range grammar.Groups {
			if g.ParentID != nil {
				id := groupIDs[*g.ParentID]
				grammar.Groups[i].ParentID = &id
			}
		}
		for i, c := range grammar.Cards {
			id, err := q.NextID(ctx)
			if err != nil {
				return Snapshot{}, fmt.Errorf("allocate grammar rule id: %w", err)
			}
			grammar.Cards[i].ID = id
			grammar.Cards[i].GroupID = groupIDs[c.GroupID]
		}
	}
	restored.Groups = append(restored.Groups, grammar.Groups...)
	restored.Cards = append(restored.Cards, grammar.Cards...)
	return restored, restored.Validate()
}

// Write participates in the caller's transaction, including full backup restore.
func Write(ctx context.Context, db store.DBTX, s Snapshot) error {
	q := store.New(db)
	gs, cs := []int64{}, []int64{}
	for _, g := range s.Groups {
		gs = append(gs, g.ID)
		if err := q.SaveGroup(ctx, store.SaveGroupParams{ID: g.ID, Title: g.Title, ParentID: g.ParentID, Grammar: g.Grammar}); err != nil {
			return fmt.Errorf("save study group: %w", err)
		}
	}
	for _, c := range s.Cards {
		cs = append(cs, c.ID)
		params := store.SaveCardParams{ID: c.ID, GroupID: c.GroupID, Kind: c.Kind, Front: c.Front, Back: c.Back}
		if c.Content != nil {
			data, err := json.Marshal(c.Content)
			if err != nil {
				return fmt.Errorf("encode grammar rule %d: %w", c.ID, err)
			}
			params.GrammarContent = data
		}
		if err := q.SaveCard(ctx, params); err != nil {
			return fmt.Errorf("save study card: %w", err)
		}
	}
	if err := q.PruneCards(ctx, cs); err != nil {
		return err
	}
	return q.PruneGroups(ctx, gs)
}
