package words

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Формат строки (SPEC §4):
//
//	term || translation || example1 <> example2 || #tag1 tag2
//
// Парсер — чистая функция без БД: тот же результат идёт и в превью (/parse),
// и в сохранение (/bulk), так что пользователь видит ровно то, что будет записано.

const (
	fieldSep   = "||"
	exampleSep = "<>"
	formsSep   = "_"
	commentPfx = "//"
)

// ErrorCode — причина, по которой строка не распарсилась. snake_case, фронт берёт текст из i18n.
type ErrorCode string

const (
	CodeTooManyFields      ErrorCode = "too_many_fields"
	CodeMissingTranslation ErrorCode = "missing_translation"
	CodeEmptyTerm          ErrorCode = "empty_term"
	CodeEmptyTranslation   ErrorCode = "empty_translation"
	CodeInvalidVerbForms   ErrorCode = "invalid_verb_forms"
	CodeUnbalancedBrackets ErrorCode = "unbalanced_brackets"
	CodeEmptyHighlight     ErrorCode = "empty_highlight"
)

type ParseResult struct {
	Cards  []Card      `json:"cards"`
	Errors []LineError `json:"errors"`
}

type Card struct {
	Line        int       `json:"line"`            // 1-based, первая строка с этой карточкой
	Term        string    `json:"term"`            // для форм — "go_went_gone"
	Forms       []string  `json:"forms,omitempty"` // ровно 3 или nil
	Translation string    `json:"translation"`
	Examples    []Example `json:"examples,omitempty"` // уникальны по Text
	Groups      []string  `json:"groups,omitempty"`   // нормализованные, без #, уникальные
}

type Example struct {
	Text       string `json:"text"`                 // нормализованная разметка "I [go] home" — пишется в БД
	Sentence   string `json:"sentence"`             // без скобок
	Highlights []Span `json:"highlights,omitempty"` // по Sentence
}

// Span — выделенный фрагмент в рунах, полуинтервал [Start, End).
// Руны, а не байты: фронт режет строку по символам, а кириллица/эмодзи в UTF-8 многобайтные.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type LineError struct {
	Line int       `json:"line"`
	Text string    `json:"text"` // исходная строка (trim), чтобы показать в превью
	Code ErrorCode `json:"code"`
}

// Parse разбирает текст из textarea. Битые строки попадают в Errors и не мешают остальным.
// Дубликаты (term+translation без учёта регистра) внутри текста сливаются в одну карточку;
// слияние с уже сохранёнными словами — задача сервиса.
func Parse(text string) ParseResult {
	acc := newAccumulator()
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw) // заодно съедает \r от CRLF
		if line == "" || strings.HasPrefix(line, commentPfx) {
			continue
		}
		card, code := parseLine(line)
		acc.add(i+1, line, card, code)
	}
	return acc.res
}

// Row — уже разбитая на поля карточка (строка CSV или объект JSON при импорте).
// Поля — как в §4: Examples через "<>", Tags через пробел/запятую/";".
type Row struct {
	Line        int    // номер строки в исходном файле — для ошибок
	Raw         string // как показать строку в ошибке
	Term        string
	Translation string
	Examples    string
	Tags        string
}

// ParseRows — то же, что Parse, но для готовых полей: те же правила и то же слияние дублей.
// Отдельно, а не «склеить в строку с || и отдать Parse»: в поле CSV может встретиться "||".
func ParseRows(rows []Row) ParseResult {
	acc := newAccumulator()
	for _, r := range rows {
		card, code := parseFields(r.Term, r.Translation, r.Examples, r.Tags)
		acc.add(r.Line, r.Raw, card, code)
	}
	return acc.res
}

// accumulator собирает результат и сливает дубликаты.
type accumulator struct {
	res   ParseResult
	byKey map[string]int // ключ дубликата → индекс в res.Cards
}

func newAccumulator() *accumulator {
	// не nil → в JSON будет [], а не null
	return &accumulator{res: ParseResult{Cards: []Card{}, Errors: []LineError{}}, byKey: make(map[string]int)}
}

func (a *accumulator) add(line int, raw string, card Card, code ErrorCode) {
	if code != "" {
		a.res.Errors = append(a.res.Errors, LineError{Line: line, Text: raw, Code: code})
		return
	}
	card.Line = line
	key := dupKey(card.Term, card.Translation)
	if idx, ok := a.byKey[key]; ok {
		mergeInto(&a.res.Cards[idx], card)
		return
	}
	a.byKey[key] = len(a.res.Cards)
	a.res.Cards = append(a.res.Cards, card)
}

// parseLine возвращает карточку или код ошибки. Код вместо error — потому что это
// не сбой, а данные для превью, и вызывающему нужен именно код, а не текст.
func parseLine(line string) (Card, ErrorCode) {
	fields := strings.Split(line, fieldSep)
	switch {
	case len(fields) > 4:
		return Card{}, CodeTooManyFields
	case len(fields) < 2:
		return Card{}, CodeMissingTranslation
	}
	fields = append(fields, "", "")[:4] // отсутствующие поля 3 и 4 = пустые
	return parseFields(fields[0], fields[1], fields[2], fields[3])
}

// parseFields применяет правила §4 к отдельным полям карточки.
func parseFields(rawTerm, rawTranslation, rawExamples, rawTags string) (Card, ErrorCode) {
	term, forms, code := normalizeTerm(rawTerm)
	if code != "" {
		return Card{}, code
	}
	translation, code := normalizeTranslation(rawTranslation)
	if code != "" {
		return Card{}, code
	}
	card := Card{Term: term, Forms: forms, Translation: translation}

	for _, raw := range strings.Split(rawExamples, exampleSep) {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		ex, code := ParseExample(raw)
		if code != "" {
			return Card{}, code
		}
		card.Examples = appendExample(card.Examples, ex)
	}
	card.Groups = parseTags(rawTags)
	return card, ""
}

// normalizeTerm применяет правила §4 к term: trim, схлопывание пробелов, 3 формы через "_".
// Общая для парсера и PATCH, чтобы правка слова проверялась так же, как ввод.
func normalizeTerm(raw string) (term string, forms []string, code ErrorCode) {
	term = collapseSpaces(raw)
	if term == "" {
		return "", nil, CodeEmptyTerm
	}
	if !strings.Contains(term, formsSep) {
		return term, nil, ""
	}
	forms, ok := parseForms(term)
	if !ok {
		return "", nil, CodeInvalidVerbForms
	}
	return strings.Join(forms, formsSep), forms, "" // "go _ went _ gone" → "go_went_gone"
}

func normalizeTranslation(raw string) (string, ErrorCode) {
	t := collapseSpaces(raw)
	if t == "" {
		return "", CodeEmptyTranslation
	}
	return t, ""
}

// parseForms: "go_went_gone" → [go went gone]. Ровно 3 непустые части, иначе ok=false.
func parseForms(term string) ([]string, bool) {
	parts := strings.Split(term, formsSep)
	if len(parts) != 3 {
		return nil, false
	}
	for i, p := range parts {
		parts[i] = collapseSpaces(p)
		if parts[i] == "" {
			return nil, false
		}
	}
	return parts, true
}

// ParseExample разбирает разметку примера: "I [ go ] home" →
// Text "I [go] home", Sentence "I go home", Highlights [{2 4}].
// Экспортируется, потому что сервис хранит в БД только Text и при чтении
// восстанавливает Sentence и Highlights этой же функцией.
func ParseExample(raw string) (Example, ErrorCode) {
	raw = strings.TrimSpace(raw)

	var text, sentence, inner strings.Builder
	var spans []Span
	inBracket := false
	pos := 0 // длина sentence в рунах — отсюда начинается следующий спан

	for _, r := range raw {
		switch {
		case r == '[':
			if inBracket { // вложенная скобка
				return Example{}, CodeUnbalancedBrackets
			}
			inBracket = true
			inner.Reset()
		case r == ']':
			if !inBracket {
				return Example{}, CodeUnbalancedBrackets
			}
			inBracket = false
			word := strings.TrimSpace(inner.String())
			if word == "" {
				return Example{}, CodeEmptyHighlight
			}
			n := utf8.RuneCountInString(word)
			spans = append(spans, Span{Start: pos, End: pos + n})
			pos += n
			text.WriteString("[" + word + "]")
			sentence.WriteString(word)
		case inBracket:
			inner.WriteRune(r)
		default:
			text.WriteRune(r)
			sentence.WriteRune(r)
			pos++
		}
	}
	if inBracket {
		return Example{}, CodeUnbalancedBrackets
	}
	return Example{Text: text.String(), Sentence: sentence.String(), Highlights: spans}, ""
}

// parseTags: "#Food, verbs; #food" → [food verbs]. Разделитель — пробел, запятая или ";".
func parseTags(s string) []string {
	var tags []string
	for _, f := range strings.FieldsFunc(s, isTagSep) {
		if tag := normalizeTag(f); tag != "" {
			tags = appendUnique(tags, tag)
		}
	}
	return tags
}

// ';' — для CSV-импорта (§4: теги через пробел или ';'); во вводе тоже работает.
func isTagSep(r rune) bool { return r == ',' || r == ';' || unicode.IsSpace(r) }

// NormalizeGroupName приводит имя группы к виду тега (§4): "#Food" → "food".
// ok=false — пусто или несколько тегов ("food verbs"): такое имя нельзя было бы
// ввести тегом, поэтому и переименовать в него нельзя.
func NormalizeGroupName(s string) (name string, ok bool) {
	name = normalizeTag(s)
	return name, name != "" && !strings.ContainsFunc(name, isTagSep)
}

// normalizeTag: "#Food" → "food". Пустая строка — тега нет.
func normalizeTag(s string) string {
	return strings.ToLower(strings.TrimLeft(strings.TrimSpace(s), "#"))
}

func mergeInto(dst *Card, src Card) {
	for _, ex := range src.Examples {
		dst.Examples = appendExample(dst.Examples, ex)
	}
	for _, g := range src.Groups {
		dst.Groups = appendUnique(dst.Groups, g)
	}
}

// Списки маленькие (единицы элементов), поэтому линейный поиск проще и быстрее map.

func appendUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

func appendExample(list []Example, ex Example) []Example {
	for _, v := range list {
		if v.Text == ex.Text { // как UNIQUE (word_id, sentence) в БД — точное совпадение
			return list
		}
	}
	return append(list, ex)
}

// dupKey повторяет UNIQUE (lower(term), lower(translation)). \x00 не встречается во вводе,
// поэтому склейка однозначна ("a b"+"c" ≠ "a"+"b c").
func dupKey(term, translation string) string {
	return strings.ToLower(term) + "\x00" + strings.ToLower(translation)
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
