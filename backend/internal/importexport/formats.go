package importexport

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"lexis/internal/platform/httpx"
	"lexis/internal/words"
)

// Форматы импорта/экспорта списка слов (SPEC §4):
//   CSV:  term,translation,example,tags — колонки по заголовку в любом порядке;
//         примеры через "<>", теги через пробел или ";".
//   JSON: [{"term", "translation", "example", "tags"}] — те же поля строками.
// Полный бэкап — отдельный формат, см. backup.go.

type Format int

const (
	FormatCSV Format = iota
	FormatJSONList
	FormatBackup
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// DetectFormat смотрит на первый значимый символ: '[' — список слов JSON, '{' — бэкап,
// иначе CSV. Так фронту не нужно передавать формат, а пользователю — угадывать расширение.
func DetectFormat(data []byte) Format {
	data = bytes.TrimLeft(bytes.TrimPrefix(data, utf8BOM), " \t\r\n")
	switch {
	case len(data) > 0 && data[0] == '[':
		return FormatJSONList
	case len(data) > 0 && data[0] == '{':
		return FormatBackup
	}
	return FormatCSV
}

// csvColumns — допустимые имена колонок (без учёта регистра) → поле words.Row.
var csvColumns = map[string]string{
	"term": "term", "translation": "translation",
	"example": "examples", "examples": "examples",
	"tags": "tags", "groups": "tags",
}

// ParseCSV разбирает CSV со строкой заголовка. Ошибка — только если файл в целом не CSV
// или нет обязательных колонок; битые строки уходят дальше и станут LineError у парсера слов.
func ParseCSV(data []byte) ([]words.Row, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM))) // BOM — от Excel
	r.FieldsPerRecord = -1                                               // разная длина строк — не ошибка файла
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, httpx.BadRequest("empty_file", "file is empty")
	}
	if err != nil {
		return nil, httpx.BadRequest("invalid_csv", "invalid CSV: "+err.Error())
	}
	idx := map[string]int{} // поле → номер колонки
	for i, h := range header {
		if field, ok := csvColumns[strings.ToLower(strings.TrimSpace(h))]; ok {
			if _, dup := idx[field]; !dup {
				idx[field] = i
			}
		}
	}
	if _, ok := idx["term"]; !ok {
		return nil, httpx.BadRequest("csv_missing_columns", "CSV header must contain term and translation")
	}
	if _, ok := idx["translation"]; !ok {
		return nil, httpx.BadRequest("csv_missing_columns", "CSV header must contain term and translation")
	}

	var rows []words.Row
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, httpx.BadRequest("invalid_csv", "invalid CSV: "+err.Error())
		}
		if isBlank(rec) {
			continue
		}
		line, _ := r.FieldPos(0)
		get := func(field string) string {
			if i, ok := idx[field]; ok && i < len(rec) {
				return rec[i]
			}
			return ""
		}
		rows = append(rows, words.Row{
			Line: line, Raw: strings.Join(rec, ","),
			Term: get("term"), Translation: get("translation"), Examples: get("examples"), Tags: get("tags"),
		})
	}
	return rows, nil
}

func isBlank(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

// listItem — элемент JSON-списка слов. Неизвестные поля игнорируем: файл мог прийти
// из другого инструмента с лишними полями.
type listItem struct {
	Term        string `json:"term"`
	Translation string `json:"translation"`
	Example     string `json:"example"`
	Examples    string `json:"examples"`
	Tags        string `json:"tags"`
}

// ParseWordList разбирает JSON-массив слов. Line — номер элемента (с 1).
func ParseWordList(data []byte) ([]words.Row, error) {
	var items []listItem
	if err := json.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &items); err != nil {
		return nil, httpx.BadRequest("invalid_json", "invalid JSON word list: "+err.Error())
	}
	rows := make([]words.Row, len(items))
	for i, it := range items {
		ex := it.Example
		if ex == "" {
			ex = it.Examples
		}
		rows[i] = words.Row{Line: i + 1, Raw: it.Term, Term: it.Term, Translation: it.Translation, Examples: ex, Tags: it.Tags}
	}
	return rows, nil
}

// WriteCSV пишет слова в формате импорта — экспорт CSV можно сразу импортировать обратно.
// Уровни и история в CSV не попадают — для этого JSON-бэкап.
func WriteCSV(w io.Writer, ws []words.Word) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"term", "translation", "example", "tags"}); err != nil {
		return err
	}
	for _, wd := range ws {
		exs := make([]string, len(wd.Examples))
		for i, e := range wd.Examples {
			exs[i] = e.Text
		}
		tags := make([]string, len(wd.Groups))
		for i, g := range wd.Groups {
			tags[i] = g.Name
		}
		if err := cw.Write([]string{wd.Term, wd.Translation, strings.Join(exs, " <> "), strings.Join(tags, " ")}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
