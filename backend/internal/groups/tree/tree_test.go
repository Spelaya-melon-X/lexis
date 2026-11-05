package tree

import (
	"errors"
	"slices"
	"testing"
)

// fixture:
//
//	1 travel
//	├── 2 airport
//	│   └── 3 gates
//	└── 4 hotel
//	5 verbs
func fixture() *Tree {
	return New([]Node{
		{ID: 1, Name: "travel"},
		{ID: 2, Name: "airport", Parent: 1},
		{ID: 3, Name: "gates", Parent: 2},
		{ID: 4, Name: "hotel", Parent: 1},
		{ID: 5, Name: "verbs"},
	})
}

func TestDepthHeight(t *testing.T) {
	tr := fixture()
	tests := []struct {
		id            int64
		depth, height int
	}{
		{0, 0, 3}, // виртуальный корень: высота считается по детям 0 (1 и 5) + 1 — не используется, но не паникует
		{1, 1, 3},
		{2, 2, 2},
		{3, 3, 1},
		{5, 1, 1},
	}
	for _, tt := range tests {
		if got := tr.Depth(tt.id); got != tt.depth {
			t.Errorf("Depth(%d) = %d, want %d", tt.id, got, tt.depth)
		}
		if tt.id != 0 {
			if got := tr.Height(tt.id); got != tt.height {
				t.Errorf("Height(%d) = %d, want %d", tt.id, got, tt.height)
			}
		}
	}
}

func TestSubtree(t *testing.T) {
	tr := fixture()
	got := tr.Subtree(1)
	slices.Sort(got)
	if want := []int64{1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Errorf("Subtree(1) = %v, want %v", got, want)
	}
	if got := tr.Subtree(5); !slices.Equal(got, []int64{5}) {
		t.Errorf("Subtree(5) = %v", got)
	}
}

func TestCheckMove(t *testing.T) {
	tests := []struct {
		name      string
		id, to    int64
		wantError error
	}{
		{"leaf to root", 3, 0, nil},
		{"subtree of height 2 under depth-1 group", 2, 5, nil},
		{"same parent again", 2, 1, nil},
		{"into itself", 2, 2, ErrCycle},
		{"into own descendant", 1, 3, ErrCycle},
		{"into own child", 1, 2, ErrCycle},
		{"height 3 under a root group → depth 4", 1, 5, ErrTooDeep},
		{"height 2 under depth-2 group → depth 4", 2, 4, ErrTooDeep},
		{"leaf under depth-2 group", 5, 4, nil},
		{"leaf under depth-3 group", 5, 3, ErrTooDeep},
		{"unknown group", 99, 0, ErrNotFound},
		{"unknown parent", 5, 99, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := fixture().CheckMove(tt.id, tt.to); !errors.Is(err, tt.wantError) {
				t.Errorf("CheckMove(%d, %d) = %v, want %v", tt.id, tt.to, err, tt.wantError)
			}
		})
	}
}

func TestCheckNew(t *testing.T) {
	tr := fixture()
	for _, tt := range []struct {
		parent int64
		want   error
	}{{0, nil}, {1, nil}, {2, nil}, {3, ErrTooDeep}, {42, ErrNotFound}} {
		if err := tr.CheckNew(tt.parent); !errors.Is(err, tt.want) {
			t.Errorf("CheckNew(%d) = %v, want %v", tt.parent, err, tt.want)
		}
	}
}

func TestCheckPath(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  error
	}{
		{"single existing", []string{"airport"}, nil},
		{"single new", []string{"food"}, nil},
		{"existing chain", []string{"travel", "airport"}, nil},
		{"existing chain not from root", []string{"airport", "gates"}, nil},
		{"new child under existing", []string{"travel", "beach"}, nil},
		{"all new, max depth", []string{"a", "b", "c"}, nil},
		{"new under depth-3 group", []string{"gates", "door"}, ErrTooDeep},
		{"new chain under depth-2 group", []string{"airport", "x", "y"}, ErrTooDeep},
		{"existing under other parent", []string{"hotel", "airport"}, ErrPathConflict},
		{"existing root group under a new one", []string{"food", "verbs"}, ErrPathConflict},
		{"existing child under a new one", []string{"food", "gates"}, ErrPathConflict},
		{"existing root as child of existing", []string{"travel", "verbs"}, ErrPathConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := fixture().CheckPath(tt.parts); !errors.Is(err, tt.want) {
				t.Errorf("CheckPath(%v) = %v, want %v", tt.parts, err, tt.want)
			}
		})
	}
}

func TestCheckPaths(t *testing.T) {
	tests := []struct {
		name  string
		paths [][]string
		want  error
	}{
		{"no paths", nil, nil},
		{"independent paths", [][]string{{"travel", "beach"}, {"food"}}, nil},
		{"same new path twice", [][]string{{"x", "y"}, {"x", "y"}}, nil},
		{"second path continues the first", [][]string{{"x", "y"}, {"y", "z"}}, nil},
		{"same new child under two new parents", [][]string{{"x", "y"}, {"z", "y"}}, ErrPathConflict},
		{"new chain then too deep", [][]string{{"x", "y", "z"}, {"z", "w"}}, ErrTooDeep},
		{"first path fails", [][]string{{"hotel", "airport"}, {"food"}}, ErrPathConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := fixture()
			if err := tr.CheckPaths(tt.paths); !errors.Is(err, tt.want) {
				t.Errorf("CheckPaths(%v) = %v, want %v", tt.paths, err, tt.want)
			}
			if _, ok := tr.ByName("x"); ok {
				t.Error("CheckPaths must not modify the tree")
			}
		})
	}
}

func TestDepth_CycleInDataDoesNotHang(t *testing.T) {
	tr := New([]Node{{ID: 1, Name: "a", Parent: 2}, {ID: 2, Name: "b", Parent: 1}})
	if d := tr.Depth(1); d > 3 {
		t.Errorf("Depth on a cycle = %d, want a small bounded number", d)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		nodes []Node
		want  error
	}{
		{"fixture", nil, nil},
		{"empty", []Node{}, nil},
		{"dangling parent", []Node{{ID: 1, Name: "a", Parent: 7}}, ErrNotFound},
		{"self-loop", []Node{{ID: 1, Name: "a", Parent: 1}}, ErrCycle},
		{"two-node cycle", []Node{{ID: 1, Name: "a", Parent: 2}, {ID: 2, Name: "b", Parent: 1}}, ErrCycle},
		{"cycle hanging below a chain", []Node{{ID: 1, Name: "a", Parent: 2}, {ID: 2, Name: "b", Parent: 3}, {ID: 3, Name: "c", Parent: 2}}, ErrCycle},
		{"too deep", []Node{{ID: 1, Name: "a"}, {ID: 2, Name: "b", Parent: 1}, {ID: 3, Name: "c", Parent: 2}, {ID: 4, Name: "d", Parent: 3}}, ErrTooDeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := fixture()
			if tt.nodes != nil {
				tr = New(tt.nodes)
			}
			if err := tr.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}
