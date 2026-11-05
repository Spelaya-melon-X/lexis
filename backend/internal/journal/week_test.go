package journal

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeBlocks(t *testing.T) {
	long := func(n int) string { return strings.Repeat("я", n) }
	tests := []struct {
		name     string
		in       []Block
		want     []Block
		wantCode string
	}{
		{"trim, keep inner newlines", []Block{{Topic: "  Сон   и   режим ", Conclusion: "\n ложиться до 23\nне есть на ночь \n"}},
			[]Block{{Topic: "Сон и режим", Conclusion: "ложиться до 23\nне есть на ночь"}}, ""},
		{"empty blocks dropped, order kept", []Block{{Topic: "A"}, {Topic: " ", Conclusion: "\n"}, {Conclusion: "B"}},
			[]Block{{Topic: "A"}, {Conclusion: "B"}}, ""},
		{"nil → empty", nil, []Block{}, ""},
		{"topic at limit", []Block{{Topic: long(MaxTopicLen)}}, []Block{{Topic: long(MaxTopicLen)}}, ""},
		{"topic too long", []Block{{Topic: long(MaxTopicLen + 1)}}, nil, "text_too_long"},
		{"conclusion too long", []Block{{Conclusion: long(MaxConclusionLen + 1)}}, nil, "text_too_long"},
		{"too many", make31(), nil, "too_many_items"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBlocks(tt.in)
			if code := errCode(err); code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func make31() []Block {
	bs := make([]Block, MaxBlocks+1)
	for i := range bs {
		bs[i] = Block{Topic: "x"}
	}
	return bs
}

func TestAverages(t *testing.T) {
	r := func(v int) *int { return &v }
	ds := []DaySummary{
		{Day: "2026-11-02", Rating: r(8), DopamineLevel: 4},
		{Day: "2026-11-03"},                            // пустой — не тянет средние вниз
		{Day: "2026-11-04", DopamineLevel: 2, Wins: 1}, // без оценки — только в дофамин
		{Day: "2026-11-05", Rating: r(6), DopamineLevel: 6},
		{Day: "2026-11-06", Rating: r(1), DopamineLevel: 7}, // после upTo
	}
	got, _ := json.Marshal(Averages(ds, "2026-11-05"))
	if want := `{"filled":3,"avgRating":7,"avgDopamine":4}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	got, _ = json.Marshal(Averages([]DaySummary{{Day: "2026-11-02"}}, "2026-11-05"))
	if want := `{"filled":0,"avgRating":null,"avgDopamine":null}`; string(got) != want {
		t.Errorf("empty: got %s, want %s", got, want)
	}
}
