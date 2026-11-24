package ambitions

import (
	"context"
	"slices"

	"lexis/internal/ambitions/store"
	"lexis/internal/platform/httpx"
)

type TimelineGoal struct {
	ID      int64  `json:"id"`
	Year    int32  `json:"year"`
	Month   *int32 `json:"month"`
	Title   string `json:"title"`
	Note    string `json:"note"`
	DreamID *int64 `json:"dreamId"`
}
type Timeline struct {
	Years []int32        `json:"years"`
	Goals []TimelineGoal `json:"goals"`
}

func cloneTimeline(t *Timeline) *Timeline {
	if t == nil {
		return nil
	}
	return &Timeline{Years: slices.Clone(t.Years), Goals: slices.Clone(t.Goals)}
}
func (s *Snapshot) validateTimeline() error {
	if s.Timeline == nil {
		return nil
	}
	t := s.Timeline
	ids := map[int64]bool{}
	if len(t.Goals) > 4000 || len(t.Years) > 9999 {
		return httpx.BadRequest("too_many_items", "timeline limit")
	}
	years := map[int32]bool{}
	for _, y := range t.Years {
		if y < 1 || y > 9999 || years[y] {
			return invalid("invalid timeline year")
		}
		years[y] = true
	}
	dreams := map[int64]bool{}
	for _, d := range s.Dreams {
		dreams[d.ID] = true
	}
	for i := range t.Goals {
		g := &t.Goals[i]
		if g.ID < 1 || g.ID > 9007199254740991 || ids[g.ID] {
			return invalid("invalid timeline id")
		}
		ids[g.ID] = true
		if g.Year < 1 || g.Year > 9999 || (g.Month != nil && (*g.Month < 1 || *g.Month > 12)) {
			return invalid("invalid timeline period")
		}
		f := Fields{Title: g.Title, Note: g.Note}
		if err := f.normalize(); err != nil {
			return err
		}
		g.Title = f.Title
		if g.DreamID != nil && !dreams[*g.DreamID] {
			return invalid("unknown timeline dream")
		}
	}
	return nil
}
func applyTimeline(s *Snapshot, kind string, id int64, method string, in Input) error {
	if s.Timeline == nil {
		s.Timeline = &Timeline{Years: []int32{}, Goals: []TimelineGoal{}}
	}
	t := s.Timeline
	if kind == "years" {
		if method != "POST" {
			return invalid("years can only be added")
		}
		if in.Year < 1 || in.Year > 9999 {
			return invalid("invalid year")
		}
		if !slices.Contains(t.Years, in.Year) {
			t.Years = append(t.Years, in.Year)
		}
		return nil
	}
	i := slices.IndexFunc(t.Goals, func(g TimelineGoal) bool { return g.ID == id })
	if method != "POST" && i < 0 {
		return httpx.NotFound("timeline goal not found")
	}
	if method == "DELETE" {
		t.Goals = slices.Delete(t.Goals, i, i+1)
		return nil
	}
	g := TimelineGoal{ID: id, Year: in.Year, Month: in.Month, Title: in.Title, Note: in.Note, DreamID: in.DreamID}
	if g.DreamID != nil {
		found := false
		for _, d := range s.Dreams {
			if d.ID == *g.DreamID {
				g.Title = d.Title
				found = true
				break
			}
		}
		if !found {
			return invalid("unknown dream")
		}
	}
	if i < 0 {
		t.Goals = append(t.Goals, g)
	} else {
		t.Goals[i] = g
	}
	return nil
}
func readTimeline(ctx context.Context, q *store.Queries, s *Snapshot) error {
	years, err := q.ListTimelineYears(ctx)
	if err != nil {
		return err
	}
	rows, err := q.ListTimelineGoals(ctx)
	if err != nil {
		return err
	}
	t := &Timeline{Years: years, Goals: []TimelineGoal{}}
	if t.Years == nil {
		t.Years = []int32{}
	}
	for _, r := range rows {
		t.Goals = append(t.Goals, TimelineGoal{ID: r.ID, Year: r.Year, Month: r.Month, Title: r.Title, Note: r.Note, DreamID: r.DreamID})
	}
	s.Timeline = t
	return nil
}

// nil means an old backup: preserve the timeline while other ambitions are restored.
func writeTimeline(ctx context.Context, q *store.Queries, t *Timeline) error {
	if t == nil {
		return nil
	}
	years := []int32{}
	ids := []int64{}
	for _, y := range t.Years {
		years = append(years, y)
		if err := q.SaveTimelineYear(ctx, y); err != nil {
			return err
		}
	}
	for _, g := range t.Goals {
		ids = append(ids, g.ID)
		if err := q.SaveTimelineGoal(ctx, store.SaveTimelineGoalParams{ID: g.ID, Year: g.Year, Month: g.Month, Title: g.Title, Note: g.Note, DreamID: g.DreamID}); err != nil {
			return err
		}
	}
	if err := q.PruneTimelineGoals(ctx, ids); err != nil {
		return err
	}
	return q.PruneTimelineYears(ctx, years)
}
