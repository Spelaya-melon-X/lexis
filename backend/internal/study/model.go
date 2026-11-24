// Package study contains Markdown learning cards, independent of English vocabulary and SRS.
package study

import (
	"slices"
	"strings"
	"unicode/utf8"

	"lexis/internal/groups/tree"
	"lexis/internal/platform/httpx"
)

type Group struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	ParentID *int64 `json:"parentId"`
	Grammar  bool   `json:"grammar,omitempty"`
}

const GrammarRule = "grammar_rule"

type Card struct {
	ID      int64           `json:"id"`
	GroupID int64           `json:"groupId"`
	Kind    string          `json:"kind"`
	Front   string          `json:"front"` // grammar: title
	Back    string          `json:"back"`  // grammar: Markdown explanation
	Content *GrammarContent `json:"content,omitempty"`
}

type GrammarContent struct {
	Formula  string           `json:"formula"`
	Examples []GrammarExample `json:"examples,omitempty"`
	Mistakes []GrammarMistake `json:"mistakes,omitempty"`
}

type GrammarExample struct {
	Text        string `json:"text"`
	Translation string `json:"translation"`
}

type GrammarMistake struct {
	Incorrect string `json:"incorrect"`
	Correct   string `json:"correct"`
}

type Snapshot struct {
	Groups []Group `json:"groups"`
	Cards  []Card  `json:"cards"`
}

func Empty() Snapshot      { return Snapshot{Groups: []Group{}, Cards: []Card{}} }
func bad(msg string) error { return httpx.BadRequest("invalid_study", msg) }

var practiceKinds = []string{"definition", "theorem", "remark", "property", "lemma"}

func validPracticeKind(kind string) bool { return slices.Contains(practiceKinds, kind) }

func validateCard(c Card) error {
	if c.Kind != GrammarRule && !validPracticeKind(c.Kind) {
		return bad("unknown card kind")
	}
	for _, s := range []string{c.Front, c.Back} {
		if strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > 20000 {
			return bad("card sides must contain 1..20000 characters")
		}
	}
	if (c.Kind == GrammarRule) != (c.Content != nil) {
		return bad("grammar rules must have grammar content")
	}
	if c.Content != nil {
		return c.Content.validate(c.Front)
	}
	return nil
}

func (g GrammarContent) validate(title string) error {
	if utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(g.Formula) > 2000 ||
		len(g.Examples) > 20 || len(g.Mistakes) > 20 {
		return bad("grammar content exceeds its limits")
	}
	line := func(s string) bool {
		return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 2000
	}
	for _, ex := range g.Examples {
		if !line(ex.Text) || utf8.RuneCountInString(ex.Translation) > 2000 {
			return bad("invalid grammar example")
		}
	}
	for _, m := range g.Mistakes {
		if !line(m.Incorrect) || !line(m.Correct) {
			return bad("invalid grammar mistake")
		}
	}
	return nil
}

func (s Snapshot) Validate() error {
	if len(s.Groups) > 500 || len(s.Cards) > 10000 {
		return httpx.BadRequest("too_many_items", "study limit")
	}
	ids := map[int64]bool{}
	groups := map[int64]Group{}
	nodes := []tree.Node{}
	unique := func(id int64) bool {
		if id < 1 || id > 9007199254740991 || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, g := range s.Groups {
		if !unique(g.ID) || strings.TrimSpace(g.Title) == "" || utf8.RuneCountInString(g.Title) > 100 {
			return bad("invalid group")
		}
		p := int64(0)
		if g.ParentID != nil {
			p = *g.ParentID
			if p <= 0 {
				return bad("invalid parent")
			}
		}
		nodes = append(nodes, tree.Node{ID: g.ID, Parent: p, Name: g.Title})
		groups[g.ID] = g
	}
	if err := tree.New(nodes).Validate(); err != nil {
		return bad(err.Error())
	}
	for _, g := range s.Groups {
		if g.ParentID != nil && g.Grammar != groups[*g.ParentID].Grammar {
			return bad("grammar and practice topics must be in separate trees")
		}
	}
	for _, c := range s.Cards {
		group, found := groups[c.GroupID]
		if !unique(c.ID) || !found || group.Grammar != (c.Kind == GrammarRule) {
			return bad("invalid card group or id")
		}
		if err := validateCard(c); err != nil {
			return err
		}
	}
	return nil
}
func (s Snapshot) MaxID() int64 {
	var id int64
	for _, g := range s.Groups {
		id = max(id, g.ID)
	}
	for _, c := range s.Cards {
		id = max(id, c.ID)
	}
	return id
}
