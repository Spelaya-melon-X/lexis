package training

import (
	"reflect"
	"testing"
)

func TestNormalizeSelection(t *testing.T) {
	for _, tt := range []struct {
		name      string
		ids, want []int64
		bad       bool
	}{
		{"omitted", nil, nil, false},
		{"empty", []int64{}, nil, true},
		{"duplicates", []int64{3, 1, 3, 1, 2}, []int64{3, 1, 2}, false},
		{"zero", []int64{1, 0}, nil, true},
		{"negative", []int64{-1}, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeSelection(tt.ids)
			if (err != nil) != tt.bad || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}
