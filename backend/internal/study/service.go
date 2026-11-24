package study

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
	"lexis/internal/study/store"
)

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewService(pool *pgxpool.Pool, c clock.Clock) *Service { return &Service{pool: pool, clock: c} }
func (s *Service) Get(ctx context.Context) (Snapshot, error) {
	var out Snapshot
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		out, err = Read(ctx, tx)
		return err
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("study get: %w", err)
	}
	return out, nil
}

type Input struct {
	Title    string          `json:"title"`
	ParentID *int64          `json:"parentId"`
	GroupID  int64           `json:"groupId"`
	Kind     string          `json:"kind"`
	Front    string          `json:"front"`
	Back     string          `json:"back"`
	Text     string          `json:"text"`
	Grammar  bool            `json:"grammar"`
	Content  *GrammarContent `json:"content"`
}

func (s *Service) Change(ctx context.Context, kind string, id int64, method string, in Input) (Snapshot, error) {
	var out Snapshot
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockStudy(ctx); err != nil {
			return err
		}
		var err error
		out, err = Read(ctx, tx)
		if err != nil {
			return err
		}
		if kind == "bulk" {
			group := slices.IndexFunc(out.Groups, func(g Group) bool { return g.ID == in.GroupID })
			if group < 0 {
				return bad("invalid card group")
			}
			parsed := Parse(in.Text, out.Groups[group].Grammar)
			if len(parsed.Errors) > 0 || len(parsed.Cards) == 0 {
				return bad("fix the preview errors before saving")
			}
			for _, p := range parsed.Cards {
				id, e := q.NextID(ctx)
				if e != nil {
					return e
				}
				c := p.Card
				c.ID = id
				c.GroupID = in.GroupID
				out.Cards = append(out.Cards, c)
			}
		} else {
			if method == "POST" {
				id, err = q.NextID(ctx)
				if err != nil {
					return err
				}
			}
			if err := Apply(&out, kind, id, method, in); err != nil {
				return err
			}
		}
		if err := out.Validate(); err != nil {
			return err
		}
		slices.SortFunc(out.Groups, func(a, b Group) int {
			if n := strings.Compare(a.Title, b.Title); n != 0 {
				return n
			}
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		return Write(ctx, tx, out)
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("study %s: %w", kind, err)
	}
	return out, nil
}
func Apply(s *Snapshot, kind string, id int64, method string, in Input) error {
	switch kind {
	case "groups":
		i := slices.IndexFunc(s.Groups, func(g Group) bool { return g.ID == id })
		if method != "POST" && i < 0 {
			return httpx.NotFound("study group not found")
		}
		if method == "DELETE" {
			for _, g := range s.Groups {
				if g.ParentID != nil && *g.ParentID == id {
					return httpx.Conflict("study_group_not_empty", "move children first")
				}
			}
			for _, c := range s.Cards {
				if c.GroupID == id {
					return httpx.Conflict("study_group_not_empty", "move or delete cards first")
				}
			}
			s.Groups = slices.Delete(s.Groups, i, i+1)
		} else {
			g := Group{ID: id, Title: strings.TrimSpace(in.Title), ParentID: in.ParentID, Grammar: in.Grammar}
			if i < 0 {
				s.Groups = append(s.Groups, g)
			} else {
				g.Grammar = s.Groups[i].Grammar
				s.Groups[i] = g
			}
		}
	case "cards":
		i := slices.IndexFunc(s.Cards, func(c Card) bool { return c.ID == id })
		if method != "POST" && i < 0 {
			return httpx.NotFound("study card not found")
		}
		if method == "DELETE" {
			s.Cards = slices.Delete(s.Cards, i, i+1)
		} else {
			c := Card{ID: id, GroupID: in.GroupID, Kind: in.Kind, Front: in.Front, Back: in.Back, Content: in.Content}
			if i < 0 {
				s.Cards = append(s.Cards, c)
			} else {
				if (s.Cards[i].Kind == GrammarRule) != (c.Kind == GrammarRule) {
					return bad("cannot change the study material type")
				}
				s.Cards[i] = c
			}
		}
	default:
		return httpx.NotFound("study entity not found")
	}
	return nil
}
