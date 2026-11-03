package achievements

import (
	"slices"
	"testing"
)

func reached(ps []Progress) []Code {
	var out []Code
	for _, p := range ps {
		if p.Reached {
			out = append(out, p.Code)
		}
	}
	return out
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		s    Snapshot
		want []Code
	}{
		{"nothing yet", Snapshot{}, nil},
		{"first session", Snapshot{Sessions: 1, BestStreak: 1}, []Code{FirstSession}},
		{"streak 3 exactly", Snapshot{Sessions: 3, BestStreak: 3}, []Code{FirstSession, Streak3}},
		{"streak 2 is not enough", Snapshot{Sessions: 5, BestStreak: 2}, []Code{FirstSession}},
		{"streak 30 unlocks all streaks", Snapshot{Sessions: 30, BestStreak: 31}, []Code{FirstSession, Streak3, Streak7, Streak30}},
		{"learned 9 → none", Snapshot{Learned: 9}, nil},
		{"learned 10", Snapshot{Learned: 10}, []Code{Learned10}},
		{"learned 100", Snapshot{Learned: 100}, []Code{Learned10, Learned50, Learned100}},
		{"learned 500", Snapshot{Learned: 777}, []Code{Learned10, Learned50, Learned100, Learned500}},
		{"archived group", Snapshot{ArchivedGroups: 2}, []Code{FirstArchivedGroup}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reached(Evaluate(tt.s)); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluate_Progress(t *testing.T) {
	ps := Evaluate(Snapshot{BestStreak: 5, Learned: 42})
	byCode := map[Code]Progress{}
	for _, p := range ps {
		byCode[p.Code] = p
	}
	tests := []struct {
		code             Code
		progress, target int
	}{
		{Streak3, 3, 3}, // прогресс обрезается по цели
		{Streak7, 5, 7},
		{Learned50, 42, 50},
		{Learned500, 42, 500},
		{FirstSession, 0, 1},
	}
	for _, tt := range tests {
		if p := byCode[tt.code]; p.Progress != tt.progress || p.Target != tt.target {
			t.Errorf("%s: got %d/%d, want %d/%d", tt.code, p.Progress, p.Target, tt.progress, tt.target)
		}
	}
	if len(ps) != 9 || ps[0].Code != FirstSession || ps[8].Code != FirstArchivedGroup {
		t.Errorf("order/count: %+v", ps)
	}
}

func TestCodesUnique(t *testing.T) {
	seen := map[Code]bool{}
	for _, r := range rules {
		if seen[r.code] {
			t.Errorf("duplicate code %s", r.code)
		}
		seen[r.code] = true
	}
}
