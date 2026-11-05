package httpx

import (
	"encoding/json"
	"testing"
)

func TestOptional(t *testing.T) {
	type body struct {
		N Optional[int] `json:"n"`
	}
	tests := []struct {
		name    string
		in      string
		wantSet bool
		want    *int
		wantErr bool
	}{
		{"absent", `{}`, false, nil, false},
		{"null", `{"n": null}`, true, nil, false},
		{"value", `{"n": 7}`, true, ptr(7), false},
		{"zero is a value", `{"n": 0}`, true, ptr(0), false},
		{"wrong type", `{"n": "7"}`, false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b body
			err := json.Unmarshal([]byte(tt.in), &b)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if b.N.Set != tt.wantSet {
				t.Errorf("Set = %v, want %v", b.N.Set, tt.wantSet)
			}
			if (b.N.Value == nil) != (tt.want == nil) || (tt.want != nil && *b.N.Value != *tt.want) {
				t.Errorf("Value = %v, want %v", b.N.Value, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
