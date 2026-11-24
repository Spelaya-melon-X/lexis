package ambitions

import "testing"

func TestTimelinePeriods(t *testing.T) {
	for _, tt := range []struct {
		year  int32
		month *int32
		ok    bool
	}{{2026, nil, true}, {2036, new(int32(12)), true}, {1, new(int32(1)), true}, {9999, nil, true}, {0, nil, false}, {10000, nil, false}, {2026, new(int32(0)), false}, {2026, new(int32(13)), false}} {
		s := Empty()
		if err := Apply(&s, "goals", 1, "POST", Input{Year: tt.year, Month: tt.month, Fields: Fields{Title: "Goal"}}); err != nil {
			t.Fatal(err)
		}
		err := s.Validate()
		if (err == nil) != tt.ok {
			t.Fatalf("%+v: %v", tt, err)
		}
	}
}
func TestTimelineLinkMoveDelete(t *testing.T) {
	s := Empty()
	s.Dreams = []Dream{{ID: 1, Fields: Fields{Title: "Dream"}, Status: "fulfilled"}}
	if err := Apply(&s, "goals", 2, "POST", Input{Year: 2026, DreamID: new(int64(1))}); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Timeline.Goals[0].Title != "Dream" {
		t.Fatal(s.Timeline)
	}
	clone := cloneTimeline(s.Timeline)
	if err := Apply(&s, "goals", 2, "PUT", Input{Year: 2037, Month: new(int32(3)), DreamID: new(int64(1))}); err != nil {
		t.Fatal(err)
	}
	if s.Timeline.Goals[0].Year != 2037 || clone.Goals[0].Year != 2026 {
		t.Fatal("move mutated previous snapshot")
	}
	if err := Apply(&s, "dreams", 1, "DELETE", Input{}); err != nil {
		t.Fatal(err)
	}
	if len(s.Timeline.Goals) != 1 || s.Timeline.Goals[0].DreamID != nil || s.Timeline.Goals[0].Title != "Dream" {
		t.Fatal(s.Timeline)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.Dreams = []Dream{{ID: 1, Fields: Fields{Title: "Dream"}, Status: "fulfilled"}}
	if err := Apply(&s, "goals", 2, "DELETE", Input{}); err != nil {
		t.Fatal(err)
	}
	if len(s.Dreams) != 1 {
		t.Fatal("deleting placement deleted dream")
	}
}
func TestTimelineRejectDanglingAndDuplicate(t *testing.T) {
	s := Empty()
	s.Timeline.Goals = []TimelineGoal{{ID: 1, Year: 2026, Title: "A", DreamID: new(int64(9))}}
	if s.Validate() == nil {
		t.Fatal("dangling dream")
	}
	s.Timeline.Goals[0].DreamID = nil
	s.Timeline.Years = []int32{2036, 2036}
	if s.Validate() == nil {
		t.Fatal("duplicate year")
	}
}
