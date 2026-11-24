package ambitions

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/ambitions/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewService(pool *pgxpool.Pool, c clock.Clock) *Service { return &Service{pool, c} }
func (s *Service) Get(ctx context.Context) (View, error) {
	var snap Snapshot
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		snap, err = Read(ctx, tx)
		return err
	})
	if err != nil {
		return View{}, fmt.Errorf("ambitions get: %w", err)
	}
	return ViewOf(snap, clock.FormatDate(clock.TodayDate(s.clock))), nil
}

// Input replaces an entity in full (PUT). Pointer references use null to unlink.
type Input struct {
	Year    int32  `json:"year"`
	Month   *int32 `json:"month"`
	DreamID *int64 `json:"dreamId"`
	Fields
	Status     string  `json:"status"`
	CategoryID *int64  `json:"categoryId"`
	ProjectID  *int64  `json:"projectId"`
	ParentID   *int64  `json:"parentId"`
	Done       bool    `json:"done"`
	Requires   []int64 `json:"requires"`
}

func (s *Service) Change(ctx context.Context, kind string, id int64, method string, in Input) (View, error) {
	var after Snapshot
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockAmbitions(ctx); err != nil {
			return err
		}
		before, err := Read(ctx, tx)
		if err != nil {
			return err
		}
		// Apply mutates slices, so clone their backing arrays before diffing.
		after = Snapshot{Timeline: cloneTimeline(before.Timeline), Categories: slices.Clone(before.Categories), Dreams: slices.Clone(before.Dreams), Projects: slices.Clone(before.Projects), Steps: slices.Clone(before.Steps), Dependencies: slices.Clone(before.Dependencies)}
		if method == "POST" {
			id, err = q.NextID(ctx)
			if err != nil {
				return err
			}
		}
		if err := Apply(&after, kind, id, method, in); err != nil {
			return err
		}
		if err := after.Validate(); err != nil {
			return err
		}
		return Write(ctx, tx, &before, after)
	})
	if err != nil {
		return View{}, fmt.Errorf("ambitions %s %s: %w", method, kind, err)
	}
	return ViewOf(after, clock.FormatDate(clock.TodayDate(s.clock))), nil
}

// Apply is pure: identity allocation and transactions belong to the service.
func Apply(s *Snapshot, kind string, id int64, method string, in Input) error {
	if kind == "goals" || kind == "years" {
		return applyTimeline(s, kind, id, method, in)
	}
	found := false
	switch kind {
	case "categories":
		for _, x := range s.Categories {
			found = found || x.ID == id
		}
	case "dreams":
		for _, x := range s.Dreams {
			found = found || x.ID == id
		}
	case "projects":
		for _, x := range s.Projects {
			found = found || x.ID == id
		}
	case "steps":
		for _, x := range s.Steps {
			found = found || x.ID == id
		}
	default:
		return httpx.NotFound("unknown entity")
	}
	if method != "POST" && !found {
		return httpx.NotFound("entity not found")
	}
	if method == "DELETE" {
		switch kind {
		case "categories":
			s.Categories = slices.DeleteFunc(s.Categories, func(x Category) bool { return x.ID == id })
			for i := range s.Dreams {
				if equalID(s.Dreams[i].CategoryID, id) {
					s.Dreams[i].CategoryID = nil
				}
			}
		case "dreams":
			s.Dreams = slices.DeleteFunc(s.Dreams, func(x Dream) bool { return x.ID == id })
			if s.Timeline != nil {
				for i := range s.Timeline.Goals {
					g := &s.Timeline.Goals[i]
					if equalID(g.DreamID, id) {
						g.DreamID = nil
					}
				}
			}
		case "projects":
			s.Projects = slices.DeleteFunc(s.Projects, func(x Project) bool { return x.ID == id })
			for i := range s.Dreams {
				if equalID(s.Dreams[i].ProjectID, id) {
					s.Dreams[i].ProjectID = nil
				}
			}
			s.Steps = slices.DeleteFunc(s.Steps, func(x Step) bool { return x.ProjectID == id })
		case "steps":
			remove := map[int64]bool{id: true}
			for changed := true; changed; {
				changed = false
				for _, x := range s.Steps {
					if x.ParentID != nil && remove[*x.ParentID] && !remove[x.ID] {
						remove[x.ID] = true
						changed = true
					}
				}
			}
			s.Steps = slices.DeleteFunc(s.Steps, func(x Step) bool { return remove[x.ID] })
		}
		alive := map[int64]bool{}
		for _, n := range s.Steps {
			alive[n.ID] = true
		}
		s.Dependencies = slices.DeleteFunc(s.Dependencies, func(d Dependency) bool { return !alive[d.StepID] || !alive[d.RequiresID] })
		return nil
	}
	if kind == "steps" && in.ParentID != nil {
		for _, n := range s.Steps {
			if n.ID == *in.ParentID && n.Done {
				return httpx.Conflict("ambition_blocked", "reopen parent before adding children")
			}
		}
	}
	switch kind {
	case "categories":
		v := Category{id, in.Title, in.Position}
		i := slices.IndexFunc(s.Categories, func(x Category) bool { return x.ID == id })
		if i < 0 {
			s.Categories = append(s.Categories, v)
		} else {
			s.Categories[i] = v
		}
	case "dreams":
		if in.Status == "" {
			in.Status = "want"
		}
		v := Dream{ID: id, Fields: in.Fields, Status: in.Status, CategoryID: in.CategoryID, ProjectID: in.ProjectID}
		i := slices.IndexFunc(s.Dreams, func(x Dream) bool { return x.ID == id })
		if i < 0 {
			s.Dreams = append(s.Dreams, v)
		} else {
			s.Dreams[i] = v
		}
	case "projects":
		v := Project{ID: id, Fields: in.Fields}
		i := slices.IndexFunc(s.Projects, func(x Project) bool { return x.ID == id })
		if i < 0 {
			s.Projects = append(s.Projects, v)
		} else {
			s.Projects[i] = v
		}
	case "steps":
		if in.ProjectID == nil {
			return invalid("projectId required")
		}
		v := Step{ID: id, Fields: in.Fields, ProjectID: *in.ProjectID, ParentID: in.ParentID, Done: in.Done}
		i := slices.IndexFunc(s.Steps, func(x Step) bool { return x.ID == id })
		if i >= 0 && s.Steps[i].ProjectID != v.ProjectID {
			return invalid("steps cannot move between projects")
		}
		if i < 0 {
			s.Steps = append(s.Steps, v)
		} else {
			s.Steps[i] = v
		}
		s.Dependencies = slices.DeleteFunc(s.Dependencies, func(d Dependency) bool { return d.StepID == id })
		for _, r := range in.Requires {
			s.Dependencies = append(s.Dependencies, Dependency{id, r})
		}
	}

	// Position is an insertion index, not a loose rank: ties never make a move disappear.
	rows := []positionRow{}
	switch kind {
	case "categories":
		for i := range s.Categories {
			rows = append(rows, positionRow{s.Categories[i].ID, &s.Categories[i].Position})
		}
	case "dreams":
		for i := range s.Dreams {
			rows = append(rows, positionRow{s.Dreams[i].ID, &s.Dreams[i].Position})
		}
	case "projects":
		for i := range s.Projects {
			rows = append(rows, positionRow{s.Projects[i].ID, &s.Projects[i].Position})
		}
	case "steps":
		for i := range s.Steps {
			n := &s.Steps[i]
			if n.ProjectID == *in.ProjectID && sameParent(n.ParentID, in.ParentID) {
				rows = append(rows, positionRow{n.ID, &n.Position})
			}
		}
	}
	reorderPositions(rows, id, in.Position)
	return nil
}
func equalID(p *int64, id int64) bool { return p != nil && *p == id }

// Convert creates an empty project with an explicit user-supplied title, and links the wish atomically.
// Notes, dates and statuses remain independent; nothing is silently copied or kept in sync.
func (s *Service) Convert(ctx context.Context, id int64, in Fields) (View, error) {
	var after Snapshot
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockAmbitions(ctx); err != nil {
			return err
		}
		before, err := Read(ctx, tx)
		if err != nil {
			return err
		}
		after = before
		after.Dreams = slices.Clone(before.Dreams)
		after.Projects = slices.Clone(before.Projects)
		i := slices.IndexFunc(after.Dreams, func(d Dream) bool { return d.ID == id })
		if i < 0 {
			return httpx.NotFound("dream not found")
		}
		if after.Dreams[i].ProjectID != nil {
			return httpx.Conflict("ambition_linked", "dream already linked")
		}
		pid, err := q.NextID(ctx)
		if err != nil {
			return err
		}
		after.Projects = append(after.Projects, Project{ID: pid, Fields: in})
		after.Dreams[i].ProjectID = &pid
		if err := after.Validate(); err != nil {
			return err
		}
		return Write(ctx, tx, &before, after)
	})
	if err != nil {
		return View{}, fmt.Errorf("convert dream: %w", err)
	}
	return ViewOf(after, clock.FormatDate(clock.TodayDate(s.clock))), nil
}

type positionRow struct {
	id       int64
	position *int32
}

func sameParent(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func reorderPositions(rows []positionRow, id int64, index int32) {
	var target positionRow
	rest := make([]positionRow, 0, len(rows))
	for _, r := range rows {
		if r.id == id {
			target = r
		} else {
			rest = append(rest, r)
		}
	}
	if target.position == nil {
		return
	}
	slices.SortFunc(rest, func(a, b positionRow) int {
		if *a.position < *b.position {
			return -1
		}
		if *a.position > *b.position {
			return 1
		}
		if a.id < b.id {
			return -1
		}
		return 1
	})
	at := min(max(int(index), 0), len(rest))
	rest = slices.Insert(rest, at, target)
	for i, r := range rest {
		*r.position = int32(i)
	}
}
