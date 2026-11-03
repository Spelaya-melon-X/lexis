package words

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		cards  []Card
		errors []LineError
	}{
		{
			name:  "full line",
			text:  "apple || яблоко || I eat an [apple] || #food #nouns",
			cards: []Card{{Line: 1, Term: "apple", Translation: "яблоко", Examples: []Example{ex("I eat an [apple]", "I eat an apple", sp(9, 14))}, Groups: []string{"food", "nouns"}}},
		},
		{
			name:  "only term and translation",
			text:  "apple || яблоко",
			cards: []Card{{Line: 1, Term: "apple", Translation: "яблоко"}},
		},
		{
			name:  "spaces around separators don't matter",
			text:  "apple||яблоко||   I eat   ||#food",
			cards: []Card{{Line: 1, Term: "apple", Translation: "яблоко", Examples: []Example{ex("I eat", "I eat")}, Groups: []string{"food"}}},
		},
		{
			name:  "inner spaces in term and translation are collapsed",
			text:  "  look   up || искать   в словаре  ",
			cards: []Card{{Line: 1, Term: "look up", Translation: "искать в словаре"}},
		},
		{
			name:  "empty examples field, tags present",
			text:  "apple || яблоко || || food",
			cards: []Card{{Line: 1, Term: "apple", Translation: "яблоко", Groups: []string{"food"}}},
		},
		{
			name:  "empty tags field",
			text:  "apple || яблоко || I eat || ",
			cards: []Card{{Line: 1, Term: "apple", Translation: "яблоко", Examples: []Example{ex("I eat", "I eat")}}},
		},
		{
			name: "several examples split by <>, empty chunks skipped, ; and , are plain text",
			text: "run || бежать || I [run]; daily <> <> They, sadly, [run] fast <>",
			cards: []Card{{Line: 1, Term: "run", Translation: "бежать", Examples: []Example{
				ex("I [run]; daily", "I run; daily", sp(2, 5)),
				ex("They, sadly, [run] fast", "They, sadly, run fast", sp(13, 16)),
			}}},
		},
		{
			name: "duplicate example in one line kept once",
			text: "run || бежать || I [run] <> I [run]",
			cards: []Card{{Line: 1, Term: "run", Translation: "бежать", Examples: []Example{
				ex("I [run]", "I run", sp(2, 5)),
			}}},
		},
		{
			name: "three verb forms via underscore",
			text: "go_went_gone || идти || I [go] home <> I [went] home <> I have [gone] home",
			cards: []Card{{Line: 1, Term: "go_went_gone", Forms: []string{"go", "went", "gone"}, Translation: "идти", Examples: []Example{
				ex("I [go] home", "I go home", sp(2, 4)),
				ex("I [went] home", "I went home", sp(2, 6)),
				ex("I have [gone] home", "I have gone home", sp(7, 11)),
			}}},
		},
		{
			name:  "verb forms with spaces around underscores are normalized",
			text:  "go _ went _ gone || идти",
			cards: []Card{{Line: 1, Term: "go_went_gone", Forms: []string{"go", "went", "gone"}, Translation: "идти"}},
		},
		{
			name:  "hyphen is part of the word, not forms",
			text:  "mother-in-law || свекровь",
			cards: []Card{{Line: 1, Term: "mother-in-law", Translation: "свекровь"}},
		},
		{
			name:   "two forms is an error",
			text:   "go_went || идти",
			errors: []LineError{{Line: 1, Text: "go_went || идти", Code: CodeInvalidVerbForms}},
		},
		{
			name:   "four forms is an error",
			text:   "a_b_c_d || x",
			errors: []LineError{{Line: 1, Text: "a_b_c_d || x", Code: CodeInvalidVerbForms}},
		},
		{
			name:   "empty form is an error",
			text:   "go__gone || идти",
			errors: []LineError{{Line: 1, Text: "go__gone || идти", Code: CodeInvalidVerbForms}},
		},
		{
			name:  "empty lines and comments skipped, line numbers kept",
			text:  "// header\n\n   \n  // indented comment\napple || яблоко",
			cards: []Card{{Line: 5, Term: "apple", Translation: "яблоко"}},
		},
		{
			name:  "CRLF line endings",
			text:  "apple || яблоко\r\npear || груша\r\n",
			cards: []Card{{Line: 1, Term: "apple", Translation: "яблоко"}, {Line: 2, Term: "pear", Translation: "груша"}},
		},
		{
			name:   "empty text",
			text:   "",
			cards:  nil,
			errors: nil,
		},
		{
			name:   "no separator",
			text:   "apple",
			errors: []LineError{{Line: 1, Text: "apple", Code: CodeMissingTranslation}},
		},
		{
			name:   "too many fields",
			text:   "a || b || c || d || e",
			errors: []LineError{{Line: 1, Text: "a || b || c || d || e", Code: CodeTooManyFields}},
		},
		{
			name:   "empty term",
			text:   " || яблоко",
			errors: []LineError{{Line: 1, Text: "|| яблоко", Code: CodeEmptyTerm}},
		},
		{
			name:   "empty translation",
			text:   "apple ||  || I eat",
			errors: []LineError{{Line: 1, Text: "apple ||  || I eat", Code: CodeEmptyTranslation}},
		},
		{
			name:   "broken example fails the whole line",
			text:   "run || бежать || I [run] <> I [run",
			errors: []LineError{{Line: 1, Text: "run || бежать || I [run] <> I [run", Code: CodeUnbalancedBrackets}},
		},
		{
			name:   "empty highlight",
			text:   "run || бежать || I [ ] fast",
			errors: []LineError{{Line: 1, Text: "run || бежать || I [ ] fast", Code: CodeEmptyHighlight}},
		},
		{
			name: "bad lines don't break the others",
			text: "apple || яблоко\nbroken\npear || груша || [x\nplum || слива",
			cards: []Card{
				{Line: 1, Term: "apple", Translation: "яблоко"},
				{Line: 4, Term: "plum", Translation: "слива"},
			},
			errors: []LineError{
				{Line: 2, Text: "broken", Code: CodeMissingTranslation},
				{Line: 3, Text: "pear || груша || [x", Code: CodeUnbalancedBrackets},
			},
		},
		{
			name: "duplicates merged case-insensitively: groups and examples added, first line wins",
			text: "Apple || Яблоко || I eat an [apple] || food\n" +
				"pear || груша\n" +
				"apple || яблоко || I eat an [apple] <> An [apple] a day || food, fruits",
			cards: []Card{
				{Line: 1, Term: "Apple", Translation: "Яблоко", Examples: []Example{
					ex("I eat an [apple]", "I eat an apple", sp(9, 14)),
					ex("An [apple] a day", "An apple a day", sp(3, 8)),
				}, Groups: []string{"food", "fruits"}},
				{Line: 2, Term: "pear", Translation: "груша"},
			},
		},
		{
			name: "same term, different translation is not a duplicate",
			text: "bank || банк\nbank || берег",
			cards: []Card{
				{Line: 1, Term: "bank", Translation: "банк"},
				{Line: 2, Term: "bank", Translation: "берег"},
			},
		},
		{
			name:  "verb forms duplicates merge regardless of spaces around _",
			text:  "go_went_gone || идти || I [go]\ngo _ went _ gone || идти || I [went]",
			cards: []Card{{Line: 1, Term: "go_went_gone", Forms: []string{"go", "went", "gone"}, Translation: "идти", Examples: []Example{ex("I [go]", "I go", sp(2, 4)), ex("I [went]", "I went", sp(2, 6))}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.text)
			if !equalOrEmpty(got.Cards, tt.cards) {
				t.Errorf("cards:\n got  %+v\n want %+v", got.Cards, tt.cards)
			}
			if !equalOrEmpty(got.Errors, tt.errors) {
				t.Errorf("errors:\n got  %+v\n want %+v", got.Errors, tt.errors)
			}
		})
	}
}

// Parse всегда отдаёт не-nil слайсы (для JSON), а в таблице удобнее писать nil.
func TestParse_NonNilSlices(t *testing.T) {
	got := Parse("")
	if got.Cards == nil || got.Errors == nil {
		t.Fatalf("want non-nil slices, got %#v", got)
	}
}

func TestParseExample(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Example
		code ErrorCode
	}{
		{"no brackets", "I eat an apple", ex("I eat an apple", "I eat an apple"), ""},
		{"outer spaces trimmed", "  I eat  ", ex("I eat", "I eat"), ""},
		{"single highlight", "I [go] home", ex("I [go] home", "I go home", sp(2, 4)), ""},
		{"spaces inside brackets trimmed", "I [  go ] home", ex("I [go] home", "I go home", sp(2, 4)), ""},
		{"phrase inside brackets", "Please [look up] it", ex("Please [look up] it", "Please look up it", sp(7, 14)), ""},
		{"multiple highlights", "I [go] and [went]", ex("I [go] and [went]", "I go and went", sp(2, 4), sp(9, 13)), ""},
		{"highlight at start and end", "[Go] now [go]", ex("[Go] now [go]", "Go now go", sp(0, 2), sp(7, 9)), ""},
		{"adjacent highlights", "[a][b]", ex("[a][b]", "ab", sp(0, 1), sp(1, 2)), ""},
		{"offsets in runes, not bytes", "Я [иду] домой", ex("Я [иду] домой", "Я иду домой", sp(2, 5)), ""},
		{"emoji counts as one rune", "🍎 [apple]", ex("🍎 [apple]", "🍎 apple", sp(2, 7)), ""},
		{"unclosed bracket", "I [go home", Example{}, CodeUnbalancedBrackets},
		{"stray closing bracket", "I go] home", Example{}, CodeUnbalancedBrackets},
		{"nested brackets", "I [[go]] home", Example{}, CodeUnbalancedBrackets},
		{"empty brackets", "I [] home", Example{}, CodeEmptyHighlight},
		{"whitespace-only brackets", "I [   ] home", Example{}, CodeEmptyHighlight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, code := ParseExample(tt.raw)
			if code != tt.code {
				t.Fatalf("code = %q, want %q", code, tt.code)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// Text из БД должен разбираться обратно в то же самое — на этом держится хранение разметки.
func TestParseExample_Idempotent(t *testing.T) {
	for _, raw := range []string{"I [ go ] home", "[a] [ b c ] d", "plain"} {
		first, _ := ParseExample(raw)
		second, code := ParseExample(first.Text)
		if code != "" || !reflect.DeepEqual(first, second) {
			t.Errorf("%q: first %+v, second %+v (code %q)", raw, first, second, code)
		}
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"with hash, space separated", "#food #nouns", []string{"food", "nouns"}},
		{"without hash", "food nouns", []string{"food", "nouns"}},
		{"comma separated", "food,nouns", []string{"food", "nouns"}},
		{"mixed separators and extra spaces", " #food ,  nouns,,#verbs ", []string{"food", "nouns", "verbs"}},
		{"lower-case, cyrillic too", "#Food #СУЩЕСТВИТЕЛЬНЫЕ", []string{"food", "существительные"}},
		{"duplicates removed, first order kept", "b a #B A", []string{"b", "a"}},
		{"lone hash skipped", "# food ##", []string{"food"}},
		{"multiple hashes stripped", "##food", []string{"food"}},
		{"underscore kept inside tag", "#3_формы_глагола", []string{"3_формы_глагола"}},
		{"empty", "   ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTags(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func ex(text, sentence string, spans ...Span) Example {
	return Example{Text: text, Sentence: sentence, Highlights: spans}
}

func sp(start, end int) Span { return Span{Start: start, End: end} }

// equalOrEmpty считает nil и пустой слайс равными, остальное — reflect.DeepEqual.
func equalOrEmpty[T any](got, want []T) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

func TestNormalizeGroupName(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"food", "food", true},
		{"  #Food ", "food", true},
		{"##Глаголы", "глаголы", true},
		{"3_формы", "3_формы", true},
		{"", "", false},
		{" # ", "", false},
		{"food verbs", "food verbs", false},
		{"food,verbs", "food,verbs", false},
	}
	for _, tt := range tests {
		got, ok := NormalizeGroupName(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("%q: got %q %v, want %q %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseRows(t *testing.T) {
	rows := []Row{
		{Line: 2, Raw: "apple,яблоко", Term: "apple", Translation: "яблоко", Examples: "I eat an [apple] <> An apple", Tags: "#food; nouns"},
		{Line: 3, Raw: "a||b,x", Term: "a||b", Translation: "x"}, // "||" внутри поля — не разделитель
		{Line: 4, Raw: "broken", Term: "", Translation: "x"},
		{Line: 5, Raw: "APPLE,ЯБЛОКО", Term: "APPLE", Translation: "ЯБЛОКО", Tags: "fruits"}, // дубликат строки 2
		{Line: 6, Raw: "go_went,идти", Term: "go_went", Translation: "идти"},
	}
	got := ParseRows(rows)
	want := []Card{
		{Line: 2, Term: "apple", Translation: "яблоко", Examples: []Example{
			ex("I eat an [apple]", "I eat an apple", sp(9, 14)), ex("An apple", "An apple"),
		}, Groups: []string{"food", "nouns", "fruits"}},
		{Line: 3, Term: "a||b", Translation: "x"},
	}
	if !reflect.DeepEqual(got.Cards, want) {
		t.Errorf("cards:\n got  %+v\n want %+v", got.Cards, want)
	}
	wantErr := []LineError{{Line: 4, Text: "broken", Code: CodeEmptyTerm}, {Line: 6, Text: "go_went,идти", Code: CodeInvalidVerbForms}}
	if !reflect.DeepEqual(got.Errors, wantErr) {
		t.Errorf("errors = %+v, want %+v", got.Errors, wantErr)
	}
	if empty := ParseRows(nil); empty.Cards == nil || empty.Errors == nil {
		t.Errorf("want non-nil slices, got %#v", empty)
	}
}

func TestParseTags_Semicolon(t *testing.T) {
	if got := parseTags("food;nouns ; #verbs"); !reflect.DeepEqual(got, []string{"food", "nouns", "verbs"}) {
		t.Errorf("got %q", got)
	}
}
