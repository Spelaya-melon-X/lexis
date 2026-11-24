package ambitions

import (
	"errors"
	"testing"

	"lexis/internal/platform/httpx"
)

func ptr(v int64) *int64 { return &v }
func fixture() Snapshot {
	s := Empty()
	s.Projects = []Project{{ID: 1, Fields: Fields{Title: "Find work"}}}
	s.Steps = []Step{{ID: 2, Fields: Fields{Title: "Java"}, ProjectID: 1}, {ID: 3, Fields: Fields{Title: "CV"}, ProjectID: 1}}
	return s
}
func code(err error) string {
	var e *httpx.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Snapshot)
		want   string
	}{
		{"valid", func(*Snapshot) {}, ""},
		{"missing parent", func(s *Snapshot) { s.Steps[0].ParentID = ptr(88) }, "invalid_ambition"},
		{"tree cycle", func(s *Snapshot) { s.Steps[0].ParentID = ptr(3); s.Steps[1].ParentID = ptr(2) }, "ambition_cycle"},
		{"dependency cycle", func(s *Snapshot) { s.Dependencies = []Dependency{{2, 3}, {3, 2}} }, "ambition_cycle"},
		{"self", func(s *Snapshot) { s.Dependencies = []Dependency{{2, 2}} }, "ambition_cycle"},
		{"child waits parent", func(s *Snapshot) { s.Steps[1].ParentID = ptr(2); s.Dependencies = []Dependency{{3, 2}} }, "ambition_cycle"},
		{"inherited deadlock", func(s *Snapshot) { s.Steps[1].ParentID = ptr(2); s.Dependencies = []Dependency{{2, 3}} }, "ambition_cycle"},
		{"missing dependency", func(s *Snapshot) { s.Dependencies = []Dependency{{2, 99}} }, "invalid_ambition"},
		{"duplicate dependency", func(s *Snapshot) { s.Dependencies = []Dependency{{2, 3}, {2, 3}} }, "invalid_ambition"},
		{"cross project", func(s *Snapshot) {
			s.Projects = append(s.Projects, Project{ID: 9, Fields: Fields{Title: "Other"}})
			s.Steps[1].ProjectID = 9
			s.Dependencies = []Dependency{{2, 3}}
		}, "invalid_ambition"},
		{"blocked completion", func(s *Snapshot) { s.Dependencies = []Dependency{{2, 3}}; s.Steps[0].Done = true }, "ambition_blocked"},
		{"completed prerequisite", func(s *Snapshot) {
			s.Dependencies = []Dependency{{2, 3}}
			s.Steps[0].Done = true
			s.Steps[1].Done = true
		}, ""},
		{"parent stored done", func(s *Snapshot) { s.Steps[1].ParentID = ptr(2); s.Steps[0].Done = true }, "invalid_ambition"},
		{"invalid date", func(s *Snapshot) { d := "2026-02-30"; s.Steps[0].Deadline = &d }, "invalid_date"},
		{"duplicate global id", func(s *Snapshot) { s.Categories = []Category{{ID: 1, Title: "Travel"}} }, "invalid_ambition"},
		{"depth", func(s *Snapshot) {
			for i := int64(4); i <= 14; i++ {
				parent := i - 1
				s.Steps = append(s.Steps, Step{ID: i, Fields: Fields{Title: "Step"}, ProjectID: 1, ParentID: &parent})
			}
		}, "invalid_ambition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fixture()
			tt.change(&s)
			if err := s.Validate(); code(err) != tt.want {
				t.Fatalf("want %s, got %v", tt.want, err)
			}
		})
	}
}
func TestWeightedProgress(t *testing.T) {
	s := fixture()
	s.Steps[0].Done = true
	if got := ViewOf(s, "2026-11-09").Summaries[1].Progress; got != 50 {
		t.Fatal(got)
	}
	// Split the still open branch into 10 leaves; completing half contributes 25%, not 5/11.
	for i := int64(10); i < 20; i++ {
		s.Steps = append(s.Steps, Step{ID: i, ProjectID: 1, ParentID: ptr(3), Fields: Fields{Title: "Part"}, Done: i < 15})
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	v := ViewOf(s, "2026-11-09")
	if v.Summaries[1].Progress != 75 || v.Summaries[1].Done != 6 || v.Summaries[1].Open != 5 {
		t.Fatalf("%+v", v.Summaries[1])
	}
}
func TestInheritedReadiness(t *testing.T) {
	s := fixture()
	s.Steps = append(s.Steps, Step{ID: 4, ProjectID: 1, ParentID: ptr(2), Fields: Fields{Title: "Core"}})
	s.Dependencies = []Dependency{{2, 3}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	v := ViewOf(s, "2026-11-09")
	if v.States[4].Ready || len(v.States[4].BlockedBy) != 1 || !v.States[3].Ready {
		t.Fatalf("%+v", v.States)
	}
	s.Steps[1].Done = true
	v = ViewOf(s, "2026-11-09")
	if !v.States[4].Ready {
		t.Fatal(v.States[4])
	}
}
func TestDeleteSafety(t *testing.T) {
	s := fixture()
	s.Categories = []Category{{ID: 10, Title: "Career"}}
	s.Dreams = []Dream{{ID: 11, Fields: Fields{Title: "Work abroad"}, Status: "want", CategoryID: ptr(10), ProjectID: ptr(1)}}
	s.Steps = append(s.Steps, Step{ID: 4, ProjectID: 1, ParentID: ptr(2), Fields: Fields{Title: "Core"}})
	s.Dependencies = []Dependency{{3, 4}}
	for _, tt := range []struct {
		kind string
		id   int64
	}{{"categories", 10}, {"steps", 2}, {"projects", 1}} {
		if err := Apply(&s, tt.kind, tt.id, "DELETE", Input{}); err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Dreams) != 1 || s.Dreams[0].CategoryID != nil || s.Dreams[0].ProjectID != nil || len(s.Steps) != 0 || len(s.Dependencies) != 0 {
		t.Fatalf("%+v", s)
	}
}
func TestReopenAndMoveValidation(t *testing.T) {
	s := fixture()
	s.Steps[0].Done = true
	s.Steps[1].Done = true
	s.Dependencies = []Dependency{{2, 3}}
	s.Steps[1].Done = false
	if code(s.Validate()) != "ambition_blocked" {
		t.Fatal("reopening prerequisite must fail")
	}
	s = fixture()
	s.Steps[0].Done = true
	err := Apply(&s, "steps", 4, "POST", Input{Fields: Fields{Title: "Child"}, ProjectID: ptr(1), ParentID: ptr(2)})
	if code(err) != "ambition_blocked" {
		t.Fatal(err)
	}
}
func TestReorderAndMove(t *testing.T) {
	s := fixture()
	s.Categories = []Category{{ID: 10, Title: "A", Position: 0}, {ID: 11, Title: "B", Position: 1}}
	if err := Apply(&s, "categories", 11, "PUT", Input{Fields: Fields{Title: "B", Position: 0}}); err != nil {
		t.Fatal(err)
	}
	if s.Categories[1].Position != 0 || s.Categories[0].Position != 1 {
		t.Fatal(s.Categories)
	}
	if err := Apply(&s, "steps", 3, "PUT", Input{Fields: Fields{Title: "CV"}, ProjectID: ptr(1), ParentID: ptr(2)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Steps[1].ParentID == nil || *s.Steps[1].ParentID != 2 {
		t.Fatal(s.Steps)
	}
}
