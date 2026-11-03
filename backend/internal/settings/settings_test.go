package settings

import (
	"errors"
	"testing"

	"lexis/internal/platform/httpx"
	"lexis/internal/settings/store"
	"lexis/internal/training"
)

func TestKeySharedWithTraining(t *testing.T) {
	if KeyNewWordsLimit != training.SettingNewWordsLimit {
		t.Fatalf("settings key %q != training key %q", KeyNewWordsLimit, training.SettingNewWordsLimit)
	}
	if Defaults().NewWordsLimit != training.DefaultNewLimit {
		t.Fatalf("default limit differs: %d vs %d", Defaults().NewWordsLimit, training.DefaultNewLimit)
	}
}

func TestPatch_Apply(t *testing.T) {
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }

	tests := []struct {
		name     string
		p        Patch
		want     Settings
		wantCode string
	}{
		{"empty patch keeps all", Patch{}, Settings{"ru", 10}, ""},
		{"language en", Patch{Language: str("en")}, Settings{"en", 10}, ""},
		{"limit 0 allowed", Patch{NewWordsLimit: num(0)}, Settings{"ru", 0}, ""},
		{"limit max", Patch{NewWordsLimit: num(100)}, Settings{"ru", 100}, ""},
		{"both", Patch{Language: str("en"), NewWordsLimit: num(25)}, Settings{"en", 25}, ""},
		{"bad language", Patch{Language: str("de")}, Settings{}, "invalid_language"},
		{"language is case-sensitive", Patch{Language: str("EN")}, Settings{}, "invalid_language"},
		{"negative limit", Patch{NewWordsLimit: num(-1)}, Settings{}, "invalid_new_words_limit"},
		{"limit too big", Patch{NewWordsLimit: num(101)}, Settings{}, "invalid_new_words_limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.p.Apply(Defaults())
			if tt.wantCode != "" {
				var he *httpx.Error
				if !errors.As(err, &he) || he.Code != tt.wantCode {
					t.Fatalf("want %q, got %v", tt.wantCode, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestFromRows(t *testing.T) {
	tests := []struct {
		name string
		rows []store.Setting
		want Settings
	}{
		{"no rows → defaults", nil, Defaults()},
		{"valid values", []store.Setting{{Key: "language", Value: []byte(`"en"`)}, {Key: "new_words_limit", Value: []byte(`3`)}}, Settings{"en", 3}},
		{"garbage → defaults", []store.Setting{{Key: "language", Value: []byte(`42`)}, {Key: "new_words_limit", Value: []byte(`"x"`)}}, Defaults()},
		{"out of range → defaults", []store.Setting{{Key: "language", Value: []byte(`"de"`)}, {Key: "new_words_limit", Value: []byte(`500`)}}, Defaults()},
		{"unknown keys ignored", []store.Setting{{Key: "notify_morning", Value: []byte(`"09:00"`)}}, Defaults()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fromRows(tt.rows); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRowsRoundTrip(t *testing.T) {
	s := Settings{Language: "en", NewWordsLimit: 7}
	var rows []store.Setting
	for _, r := range s.rows() {
		rows = append(rows, store.Setting{Key: r.Key, Value: r.Value})
	}
	if got := fromRows(rows); got != s {
		t.Errorf("round trip: got %+v, want %+v", got, s)
	}
}
