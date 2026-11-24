package study

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name, text, code string
		cards            int
	}{
		{"empty", "", "", 0},
		{"ru", "[тип: теорема]\nЗБЧ\n---\n$\\overline X \\to EX$", "", 1},
		{"en", "[type: definition]\nNorm $||x||$\n---\n$$x$$", "", 1},
		{"two", "[тип: лемма]\nA\n---\nB\n\n[тип: свойство]\nC\n---\nD", "", 2},
		{"unknown", "[тип: факт]\nA\n---\nB", "study_unknown_kind", 0},
		{"separator", "[тип: теорема]\nA\nB", "study_missing_separator", 0},
		{"empty front", "[тип: теорема]\n---\nB", "study_empty_side", 0},
		{"empty back", "[тип: теорема]\nA\n---", "study_empty_side", 0},
		{"header", "A\n---\nB", "study_missing_header", 0},
		{"code", "[type: remark]\n```text\n---\n[type: theorem]\n```\nFront\n---\nBack", "", 1},
		{"tilde", "[type: remark]\n~~~~\n---\n[type: theorem]\n~~~~\n---\nBack", "", 1},
		{"math", "[type: remark]\n$$\n---\n[type: theorem]\n$$\n---\nBack", "", 1},
		{"unclosed", "[type: remark]\nA\n---\n```\n[type: theorem]", "study_unclosed_block", 0},
		{"escaped", "[type: remark]\n\\---\n\\[type: theorem]\n---\nBack\n---\nMore", "", 1},
		{"too long", "[type: lemma]\n" + strings.Repeat("я", 20001) + "\n---\nB", "text_too_long", 0},
		{"crlf", "[type: lemma]\r\nA\r\n---\r\nB", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.text, false)
			if len(got.Cards) != tt.cards {
				t.Fatalf("cards: %+v", got)
			}
			if tt.code != "" {
				if len(got.Errors) == 0 || got.Errors[0].Code != tt.code {
					t.Fatalf("error: %+v", got)
				}
			} else if len(got.Errors) > 0 {
				t.Fatal(got.Errors)
			}
		})
	}
}
func TestParseSourceAndRecordNumbers(t *testing.T) {
	text := "[type: theorem]\n  $||x||$  \n---\n```go\n---\n```\n\n[type: invalid]\nA\n---\nB\n\n[type: lemma]\nC\n---\nD"
	got := Parse(text, false)
	if len(got.Cards) != 2 || got.Cards[0].Front != "  $||x||$  " || got.Cards[0].Back != "```go\n---\n```" || got.Cards[1].Record != 3 || len(got.Errors) != 1 || got.Errors[0].Record != 2 || got.Errors[0].Line != 8 {
		t.Fatalf("%+v", got)
	}
}

func TestParseGrammarBatch(t *testing.T) {
	source := "[тип: правило]\nShould + verb\n---\nСовет.\n\n- Пример с Markdown\n" +
		"[формула] Subject + should + base verb\n" +
		"[пример] You should study. || Тебе следует заниматься.\n" +
		"[пример] We should leave.\n" +
		"[ошибка] You should to study. || You should study.\n\n" +
		"[type: grammar_rule]\nAlthough\n---\nУступка.\n\n```text\n[formula] literal\n```"
	got := Parse(source, true)
	want := []ParsedCard{
		{Record: 1, Card: Card{
			Kind: GrammarRule, Front: "Should + verb", Back: "Совет.\n\n- Пример с Markdown",
			Content: &GrammarContent{
				Formula: "Subject + should + base verb",
				Examples: []GrammarExample{
					{Text: "You should study.", Translation: "Тебе следует заниматься."}, {Text: "We should leave."},
				},
				Mistakes: []GrammarMistake{{Incorrect: "You should to study.", Correct: "You should study."}},
			},
		}},
		{Record: 2, Card: Card{
			Kind: GrammarRule, Front: "Although", Back: "Уступка.\n\n```text\n[formula] literal\n```",
			Content: &GrammarContent{},
		}},
	}
	if len(got.Errors) != 0 || !reflect.DeepEqual(got.Cards, want) {
		t.Fatalf("grammar result = %+v", got)
	}
	if result := Parse(source, false); len(result.Cards) != 0 || len(result.Errors) != 2 {
		t.Fatalf("math parser accepted grammar: %+v", result)
	}
}

func TestParseGrammarErrors(t *testing.T) {
	out := Parse("[type: grammar_rule]\nTitle\n---\nExplanation\n[formula] first\n[formula] repeat\n"+
		"[mistake] no correction\n[type: lemma]\nMath\n---\nBack", true)
	want := []ParseError{
		{Record: 1, Line: 6, Code: "study_invalid_grammar_field"},
		{Record: 1, Line: 7, Code: "study_invalid_grammar_field"},
		{Record: 2, Line: 8, Code: "study_expected_grammar"},
	}
	if len(out.Cards) != 0 || !reflect.DeepEqual(out.Errors, want) {
		t.Fatalf("got %+v, want errors %+v", out, want)
	}
}
func TestStudyValidation(t *testing.T) {
	for _, kind := range practiceKinds {
		if err := (Snapshot{Groups: []Group{{ID: 1, Title: "Math"}}, Cards: []Card{{ID: 2, GroupID: 1, Kind: kind, Front: "$x$", Back: "**A**"}}}).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	tests := []Snapshot{
		{Groups: []Group{{ID: 1, Title: "A", ParentID: new(int64(1))}}},
		{Groups: []Group{{ID: 1, Title: "A", ParentID: new(int64(2))}}},
		{Groups: []Group{{ID: 1, Title: "A"}}, Cards: []Card{{ID: 2, GroupID: 99, Kind: "lemma", Front: "a", Back: "b"}}},
		{Groups: []Group{{ID: 1, Title: "A"}}, Cards: []Card{{ID: 2, GroupID: 1, Kind: "other", Front: "a", Back: "b"}}},
		{Groups: []Group{{ID: 1, Title: "A"}, {ID: 2, Title: "B", ParentID: new(int64(1))}, {ID: 3, Title: "C", ParentID: new(int64(2))}, {ID: 4, Title: "D", ParentID: new(int64(3))}}},
	}
	for i, s := range tests {
		if s.Validate() == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}
