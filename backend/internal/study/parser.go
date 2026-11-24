package study

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type ParseError struct {
	Record int    `json:"record"`
	Line   int    `json:"line"`
	Code   string `json:"code"`
}
type ParsedCard struct {
	Record int `json:"record"`
	Card
}
type ParseResult struct {
	Cards  []ParsedCard `json:"cards"`
	Errors []ParseError `json:"errors"`
}

var header = regexp.MustCompile(`^\[(?:тип|type):\s*([^\]]*)\]$`)
var fenceStart = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
var aliases = map[string]string{"определение": "definition", "теорема": "theorem", "замечание": "remark", "свойство": "property", "лемма": "lemma", "правило": GrammarRule}

type grammarFields struct {
	content GrammarContent
	invalid bool
}

// add reads optional field lines after the separator; all other lines belong to Markdown.
func (g *grammarFields) add(line string) (matched, valid bool) {
	marker, raw, closed := strings.Cut(line, "]")
	if !closed || (raw != "" && raw[0] != ' ' && raw[0] != '\t') {
		return false, false
	}
	value := strings.TrimSpace(raw)
	switch strings.ToLower(marker) {
	case "[формула", "[formula":
		if value == "" || g.content.Formula != "" || utf8.RuneCountInString(value) > 2000 {
			return true, false
		}
		g.content.Formula = value
	case "[пример", "[example":
		text, translation, _ := strings.Cut(value, "||")
		text, translation = strings.TrimSpace(text), strings.TrimSpace(translation)
		if text == "" || len(g.content.Examples) >= 20 || utf8.RuneCountInString(text) > 2000 || utf8.RuneCountInString(translation) > 2000 {
			return true, false
		}
		g.content.Examples = append(g.content.Examples, GrammarExample{Text: text, Translation: translation})
	case "[ошибка", "[mistake":
		incorrect, correct, _ := strings.Cut(value, "||")
		incorrect, correct = strings.TrimSpace(incorrect), strings.TrimSpace(correct)
		if incorrect == "" || correct == "" || len(g.content.Mistakes) >= 20 ||
			utf8.RuneCountInString(incorrect) > 2000 || utf8.RuneCountInString(correct) > 2000 {
			return true, false
		}
		g.content.Mistakes = append(g.content.Mistakes, GrammarMistake{Incorrect: incorrect, Correct: correct})
	default:
		return false, false
	}
	return true, true
}

// Structural lines are recognized only at column zero, outside fenced code/math.
// The first --- separates sides; later --- belongs to the back. Escapes are Markdown source.
func Parse(text string, grammar bool) ParseResult {
	out := ParseResult{Cards: []ParsedCard{}, Errors: []ParseError{}}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	record, line, split := 0, 0, -1
	kind := ""
	body := []string{}
	fence := ""
	math := ""
	preamble := false
	var fields grammarFields
	flush := func() {
		if record == 0 {
			return
		}
		code := ""
		switch {
		case grammar && kind != GrammarRule:
			code = "study_expected_grammar"
		case !grammar && !validPracticeKind(kind):
			code = "study_unknown_kind"
		case fence != "" || math != "":
			code = "study_unclosed_block"
		case split < 0:
			code = "study_missing_separator"
		default:
			front := strings.Trim(strings.Join(body[:split], "\n"), "\n")
			back := strings.Trim(strings.Join(body[split:], "\n"), "\n")
			if strings.TrimSpace(front) == "" || strings.TrimSpace(back) == "" {
				code = "study_empty_side"
			} else if utf8.RuneCountInString(front) > 20000 || utf8.RuneCountInString(back) > 20000 ||
				(grammar && utf8.RuneCountInString(front) > 200) {
				code = "text_too_long"
			} else if !fields.invalid {
				card := Card{Kind: kind, Front: front, Back: back}
				if grammar {
					content := fields.content
					card.Content = &content
				}
				out.Cards = append(out.Cards, ParsedCard{Record: record, Card: card})
			}
		}
		if code != "" {
			out.Errors = append(out.Errors, ParseError{Record: record, Line: line, Code: code})
		}
	}
	for i, s := range lines {
		if fence == "" && math == "" {
			if m := header.FindStringSubmatch(s); m != nil {
				if record == 0 && preamble {
					record = 1
				} else {
					flush()
				}
				record++
				line = i + 1
				kind = strings.ToLower(strings.TrimSpace(m[1]))
				if a, ok := aliases[kind]; ok {
					kind = a
				}
				body = []string{}
				split = -1
				fields = grammarFields{}
				continue
			}
		}
		if record == 0 {
			if strings.TrimSpace(s) != "" {
				preamble = true
				out.Errors = append(out.Errors, ParseError{Record: 1, Line: i + 1, Code: "study_missing_header"})
			}
			continue
		}
		structural := fence == "" && math == ""
		if structural && grammar && kind == GrammarRule && split >= 0 {
			if matched, valid := fields.add(s); matched {
				if !valid {
					fields.invalid = true
					out.Errors = append(out.Errors, ParseError{Record: record, Line: i + 1, Code: "study_invalid_grammar_field"})
				}
				continue
			}
		}
		if fence != "" {
			t := strings.TrimSpace(s)
			if len(t) >= len(fence) && strings.Trim(t, string(fence[0])) == "" {
				fence = ""
			}
		} else if math == "" {
			if m := fenceStart.FindStringSubmatch(s); m != nil {
				fence = m[1]
			} else if strings.TrimSpace(s) == "$$" {
				math = "$$"
			} else if strings.TrimSpace(s) == `\[` {
				math = `\]`
			}
		} else if strings.TrimSpace(s) == math {
			math = ""
		}
		if structural && s == "---" && split < 0 {
			split = len(body)
			continue
		}
		body = append(body, s)
	}
	flush()
	return out
}
