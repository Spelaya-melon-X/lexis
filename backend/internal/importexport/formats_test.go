package importexport

import (
	"bytes"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"lexis/internal/platform/httpx"
	"lexis/internal/words"
)

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) || he.Code != code || he.Status != http.StatusBadRequest {
		t.Fatalf("want 400 %q, got %v", code, err)
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Format
	}{
		{"csv", "term,translation\napple,яблоко", FormatCSV},
		{"json list", `[{"term":"a"}]`, FormatJSONList},
		{"json list with spaces", "\n  \t[ ]", FormatJSONList},
		{"backup", `{"version":1}`, FormatBackup},
		{"BOM + backup", "\xEF\xBB\xBF{\"version\":1}", FormatBackup},
		{"empty", "", FormatCSV},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectFormat([]byte(tt.in)); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []words.Row
	}{
		{"standard order",
			"term,translation,example,tags\napple,яблоко,I eat an [apple],food nouns\n",
			[]words.Row{{Line: 2, Raw: "apple,яблоко,I eat an [apple],food nouns", Term: "apple", Translation: "яблоко", Examples: "I eat an [apple]", Tags: "food nouns"}}},
		{"columns in any order, case-insensitive header, aliases",
			"Tags, Translation ,TERM,Examples\n#food;fruits,яблоко,apple,a <> b\n",
			[]words.Row{{Line: 2, Raw: "#food;fruits,яблоко,apple,a <> b", Term: "apple", Translation: "яблоко", Examples: "a <> b", Tags: "#food;fruits"}}},
		{"only required columns, unknown ignored",
			"term,note,translation\ngo,irregular,идти\n",
			[]words.Row{{Line: 2, Raw: "go,irregular,идти", Term: "go", Translation: "идти"}}},
		{"quoted fields with commas and newlines",
			"term,translation,example\n\"well, then\",\"ну, тогда\",\"line1\nline2\"\nnext,след,\n",
			[]words.Row{
				{Line: 2, Raw: "well, then,ну, тогда,line1\nline2", Term: "well, then", Translation: "ну, тогда", Examples: "line1\nline2"},
				{Line: 4, Raw: "next,след,", Term: "next", Translation: "след"},
			}},
		{"short rows and blank lines are fine",
			"term,translation,example,tags\napple\n,,,\n\nb,c\n",
			[]words.Row{{Line: 2, Raw: "apple", Term: "apple"}, {Line: 5, Raw: "b,c", Term: "b", Translation: "c"}}},
		{"excel BOM", "\xEF\xBB\xBFterm,translation\na,b\n",
			[]words.Row{{Line: 2, Raw: "a,b", Term: "a", Translation: "b"}}},
		{"header only", "term,translation\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCSV([]byte(tt.in))
			if err != nil {
				t.Fatalf("ParseCSV: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}

func TestParseCSV_Errors(t *testing.T) {
	tests := []struct {
		name, in, code string
	}{
		{"empty", "", "empty_file"},
		{"no translation column", "term,example\na,b\n", "csv_missing_columns"},
		{"no term column", "word,translation\na,b\n", "csv_missing_columns"},
		{"broken quotes", "term,translation\n\"a,b\n", "invalid_csv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCSV([]byte(tt.in))
			assertCode(t, err, tt.code)
		})
	}
}

func TestParseWordList(t *testing.T) {
	got, err := ParseWordList([]byte(`[
		{"term":"apple","translation":"яблоко","example":"I eat an [apple]","tags":"food"},
		{"term":"go_went_gone","translation":"идти","examples":"I [go] <> I [went]","extra":1},
		{}
	]`))
	if err != nil {
		t.Fatalf("ParseWordList: %v", err)
	}
	want := []words.Row{
		{Line: 1, Raw: "apple", Term: "apple", Translation: "яблоко", Examples: "I eat an [apple]", Tags: "food"},
		{Line: 2, Raw: "go_went_gone", Term: "go_went_gone", Translation: "идти", Examples: "I [go] <> I [went]"},
		{Line: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\n got  %+v\n want %+v", got, want)
	}

	_, err = ParseWordList([]byte(`[{"term": 5}]`))
	assertCode(t, err, "invalid_json")
}

// Экспорт CSV должен импортироваться обратно без потерь (кроме уровней — их в CSV нет).
func TestWriteCSV_RoundTrip(t *testing.T) {
	ws := []words.Word{
		{Term: "apple", Translation: "яблоко, фрукт", Examples: []words.Example{{Text: "I eat an [apple]"}, {Text: "An \"apple\""}},
			Groups: []words.GroupRef{{Name: "food"}, {Name: "nouns"}}},
		{Term: "go_went_gone", Translation: "идти"},
	}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, ws); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	if !strings.HasPrefix(buf.String(), "term,translation,example,tags\n") {
		t.Errorf("header: %q", buf.String())
	}
	rows, err := ParseCSV(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	parsed := words.ParseRows(rows)
	if len(parsed.Errors) != 0 || len(parsed.Cards) != 2 {
		t.Fatalf("parsed = %+v", parsed)
	}
	c := parsed.Cards[0]
	if c.Translation != "яблоко, фрукт" || len(c.Examples) != 2 || c.Examples[1].Text != `An "apple"` ||
		!reflect.DeepEqual(c.Groups, []string{"food", "nouns"}) {
		t.Errorf("card = %+v", c)
	}
	if !reflect.DeepEqual(parsed.Cards[1].Forms, []string{"go", "went", "gone"}) {
		t.Errorf("forms lost: %+v", parsed.Cards[1])
	}
}
