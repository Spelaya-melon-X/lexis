// Package ambitions owns life wishes and long-running projects, independently of daily tasks.
package ambitions

import (
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

const MaxSteps = 2000
const MaxDepth = 10

type Category struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Position int32  `json:"position"`
}
type Fields struct {
	Title    string  `json:"title"`
	Note     string  `json:"note"`
	Deadline *string `json:"deadline"`
	Position int32   `json:"position"`
}
type Dream struct {
	ID int64 `json:"id"`
	Fields
	Status     string `json:"status"`
	CategoryID *int64 `json:"categoryId"`
	ProjectID  *int64 `json:"projectId"`
}
type Project struct {
	ID int64 `json:"id"`
	Fields
}
type Step struct {
	ID int64 `json:"id"`
	Fields
	ProjectID int64  `json:"projectId"`
	ParentID  *int64 `json:"parentId"`
	Done      bool   `json:"done"` // Only leaves store completion; parents are derived.
}
type Dependency struct {
	StepID     int64 `json:"stepId"`
	RequiresID int64 `json:"requiresId"`
}
type Snapshot struct {
	Timeline     *Timeline    `json:"timeline,omitempty"`
	Categories   []Category   `json:"categories"`
	Dreams       []Dream      `json:"dreams"`
	Projects     []Project    `json:"projects"`
	Steps        []Step       `json:"steps"`
	Dependencies []Dependency `json:"dependencies"`
}

func Empty() Snapshot {
	return Snapshot{Timeline: &Timeline{Years: []int32{}, Goals: []TimelineGoal{}}, Categories: []Category{}, Dreams: []Dream{}, Projects: []Project{}, Steps: []Step{}, Dependencies: []Dependency{}}
}
func invalid(message string) error { return httpx.BadRequest("invalid_ambition", message) }
func (f *Fields) normalize() error {
	f.Title = strings.TrimSpace(f.Title)
	if n := utf8.RuneCountInString(f.Title); n < 1 || n > 200 {
		return httpx.BadRequest("invalid_title", "title must contain 1..200 characters")
	}
	if utf8.RuneCountInString(f.Note) > 20000 {
		return httpx.BadRequest("text_too_long", "note exceeds 20000 characters")
	}
	if f.Deadline != nil {
		if _, err := clock.ParseDate(*f.Deadline); err != nil {
			return httpx.BadRequest("invalid_date", "invalid deadline")
		}
	}
	return nil
}

// graph includes implicit child prerequisites and inherited ancestor prerequisites.
// Thus a step cannot depend on its own descendant/ancestor in a way that deadlocks the tree.
func (s Snapshot) graph() (map[int64][]int64, map[int64][]int64) {
	children := map[int64][]int64{}
	deps := map[int64][]int64{}
	byID := map[int64]Step{}
	for _, n := range s.Steps {
		byID[n.ID] = n
		if n.ParentID != nil {
			children[*n.ParentID] = append(children[*n.ParentID], n.ID)
		}
	}
	for _, d := range s.Dependencies {
		deps[d.StepID] = append(deps[d.StepID], d.RequiresID)
	}
	edges := map[int64][]int64{}
	for _, n := range s.Steps {
		edges[n.ID] = append(edges[n.ID], children[n.ID]...)
		for cur, depth := n, 0; depth <= MaxDepth; depth++ {
			edges[n.ID] = append(edges[n.ID], deps[cur.ID]...)
			if cur.ParentID == nil {
				break
			}
			cur = byID[*cur.ParentID]
		}
	}
	return edges, children
}

// Validate is shared by commands and backup import. It never repairs a corrupt graph silently.
func (s *Snapshot) Validate() error {
	if len(s.Steps) > MaxSteps || len(s.Dreams) > 2000 || len(s.Projects) > 200 || len(s.Categories) > 100 || len(s.Dependencies) > 10000 {
		return httpx.BadRequest("too_many_items", "ambitions limit exceeded")
	}
	ids := map[int64]bool{}
	cats := map[int64]bool{}
	projects := map[int64]bool{}
	steps := map[int64]Step{}
	unique := func(id int64) error {
		if id <= 0 || id > 9007199254740991 || ids[id] {
			return invalid("invalid or duplicate id")
		}
		ids[id] = true
		return nil
	}
	for i := range s.Categories {
		c := &s.Categories[i]
		if err := unique(c.ID); err != nil {
			return err
		}
		c.Title = strings.TrimSpace(c.Title)
		if n := utf8.RuneCountInString(c.Title); n < 1 || n > 60 {
			return invalid("category title must contain 1..60 characters")
		}
		cats[c.ID] = true
	}
	for i := range s.Projects {
		p := &s.Projects[i]
		if err := unique(p.ID); err != nil {
			return err
		}
		if err := p.Fields.normalize(); err != nil {
			return err
		}
		projects[p.ID] = true
	}
	for i := range s.Dreams {
		d := &s.Dreams[i]
		if err := unique(d.ID); err != nil {
			return err
		}
		if err := d.Fields.normalize(); err != nil {
			return err
		}
		if d.Status != "want" && d.Status != "active" && d.Status != "fulfilled" {
			return invalid("invalid dream status")
		}
		if d.CategoryID != nil && !cats[*d.CategoryID] {
			return invalid("unknown category")
		}
		if d.ProjectID != nil && !projects[*d.ProjectID] {
			return invalid("unknown project")
		}
	}
	for i := range s.Steps {
		n := &s.Steps[i]
		if err := unique(n.ID); err != nil {
			return err
		}
		if err := n.Fields.normalize(); err != nil {
			return err
		}
		if !projects[n.ProjectID] {
			return invalid("unknown project")
		}
		steps[n.ID] = *n
	}
	for _, n := range s.Steps {
		seen := map[int64]bool{n.ID: true}
		cur := n
		for depth := 1; cur.ParentID != nil; depth++ {
			p, ok := steps[*cur.ParentID]
			if !ok || p.ProjectID != n.ProjectID {
				return invalid("unknown parent in project")
			}
			if seen[p.ID] {
				return httpx.BadRequest("ambition_cycle", "tree cycle")
			}
			seen[p.ID] = true
			if depth >= MaxDepth {
				return invalid("tree exceeds 10 levels")
			}
			cur = p
		}
	}
	pairs := map[Dependency]bool{}
	for _, d := range s.Dependencies {
		a, ok := steps[d.StepID]
		b, ok2 := steps[d.RequiresID]
		if !ok || !ok2 || a.ProjectID != b.ProjectID || pairs[d] {
			return invalid("invalid dependency")
		}
		pairs[d] = true
	}
	if err := s.validateTimeline(); err != nil {
		return err
	}
	edges, children := s.graph()
	colors := map[int64]int{}
	var visit func(int64) error
	visit = func(id int64) error {
		if colors[id] == 1 {
			return httpx.BadRequest("ambition_cycle", "dependency cycle")
		}
		if colors[id] == 2 {
			return nil
		}
		colors[id] = 1
		for _, x := range edges[id] {
			if err := visit(x); err != nil {
				return err
			}
		}
		colors[id] = 2
		return nil
	}
	for id := range steps {
		if err := visit(id); err != nil {
			return err
		}
	}
	done := s.completion(children)
	for _, n := range s.Steps {
		if len(children[n.ID]) > 0 && n.Done {
			return invalid("only leaves store completion")
		}
		if n.Done {
			for _, id := range edges[n.ID] {
				if !done[id] {
					return httpx.Conflict("ambition_blocked", "reopen dependent steps first or finish prerequisites")
				}
			}
		}
	}
	return nil
}
func (s Snapshot) completion(children map[int64][]int64) map[int64]bool {
	out := map[int64]bool{}
	byID := map[int64]Step{}
	for _, n := range s.Steps {
		byID[n.ID] = n
	}
	var walk func(int64) bool
	walk = func(id int64) bool {
		if len(children[id]) == 0 {
			out[id] = byID[id].Done
			return out[id]
		}
		v := true
		for _, c := range children[id] {
			v = walk(c) && v
		}
		out[id] = v
		return v
	}
	for _, n := range s.Steps {
		if n.ParentID == nil {
			walk(n.ID)
		}
	}
	return out
}

type StepState struct {
	Done      bool    `json:"done"`
	Leaf      bool    `json:"leaf"`
	Ready     bool    `json:"ready"`
	BlockedBy []int64 `json:"blockedBy"`
	Overdue   bool    `json:"overdue"`
	Progress  float64 `json:"progress"`
}
type Summary struct {
	Progress int     `json:"progress"`
	Done     int     `json:"done"`
	Open     int     `json:"open"`
	Overdue  int     `json:"overdue"`
	Next     []int64 `json:"next"`
}
type View struct {
	Snapshot
	Today     string              `json:"today"`
	States    map[int64]StepState `json:"states"`
	Summaries map[int64]Summary   `json:"summaries"`
}

// ViewOf splits each root's weight equally among its children, recursively.
// Refining one branch never gives it extra weight relative to another root.
func ViewOf(s Snapshot, today string) View {
	edges, children := s.graph()
	done := s.completion(children)
	out := View{Snapshot: s, Today: today, States: map[int64]StepState{}, Summaries: map[int64]Summary{}}
	byID := map[int64]Step{}
	for _, n := range s.Steps {
		byID[n.ID] = n
	}
	var progress func(int64) float64
	progress = func(id int64) float64 {
		if len(children[id]) == 0 {
			if done[id] {
				return 1
			}
			return 0
		}
		sum := 0.0
		for _, c := range children[id] {
			sum += progress(c)
		}
		return sum / float64(len(children[id]))
	}
	for _, n := range s.Steps {
		blocked := []int64{}
		for _, id := range edges[n.ID] {
			if !done[id] && !slices.Contains(children[n.ID], id) && !slices.Contains(blocked, id) {
				blocked = append(blocked, id)
			}
		}
		st := StepState{Done: done[n.ID], Leaf: len(children[n.ID]) == 0, BlockedBy: blocked, Overdue: !done[n.ID] && n.Deadline != nil && *n.Deadline < today, Progress: progress(n.ID)}
		st.Ready = st.Leaf && !st.Done && len(blocked) == 0
		out.States[n.ID] = st
	}
	for _, p := range s.Projects {
		sum := Summary{Next: []int64{}}
		roots := 0
		fraction := 0.0
		candidates := []Step{}
		for _, n := range s.Steps {
			if n.ProjectID != p.ID {
				continue
			}
			st := out.States[n.ID]
			if n.ParentID == nil {
				roots++
				fraction += st.Progress
			}
			if st.Leaf {
				if st.Done {
					sum.Done++
				} else {
					sum.Open++
				}
				if st.Ready {
					candidates = append(candidates, n)
				}
			}
			if st.Overdue {
				sum.Overdue++
			}
		}
		if roots > 0 {
			sum.Progress = int(math.Round(100 * fraction / float64(roots)))
		}
		slices.SortStableFunc(candidates, func(a, b Step) int {
			ad, bd := "9999-12-31", "9999-12-31"
			if a.Deadline != nil {
				ad = *a.Deadline
			}
			if b.Deadline != nil {
				bd = *b.Deadline
			}
			if ad != bd {
				return strings.Compare(ad, bd)
			}
			if a.Position < b.Position {
				return -1
			}
			if a.Position > b.Position {
				return 1
			}
			return int(a.ID - b.ID)
		})
		for _, n := range candidates[:min(5, len(candidates))] {
			sum.Next = append(sum.Next, n.ID)
		}
		out.Summaries[p.ID] = sum
	}
	return out
}
func (s Snapshot) MaxID() int64 {
	var m int64
	if s.Timeline != nil {
		for _, g := range s.Timeline.Goals {
			m = max(m, g.ID)
		}
	}
	for _, v := range s.Categories {
		m = max(m, v.ID)
	}
	for _, v := range s.Dreams {
		m = max(m, v.ID)
	}
	for _, v := range s.Projects {
		m = max(m, v.ID)
	}
	for _, v := range s.Steps {
		m = max(m, v.ID)
	}
	return m
}
