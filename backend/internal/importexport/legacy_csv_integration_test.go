//go:build integration

package importexport

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"slices"
	"testing"

	"lexis/internal/words"
)

// The owner's export stays outside the repository. dbtest uses a disposable database.
func TestLegacyCSVBackup(t *testing.T) {
	path := os.Getenv("LEXIS_TEST_WORDS_CSV")
	if path == "" {
		t.Skip("LEXIS_TEST_WORDS_CSV not set")
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc, _, _ := newTestEnv(t)
	want := csvCards(t, original)

	result := mustImport(t, svc, string(original))
	if len(result.Errors) != 0 || result.Created != len(want) || result.Merged != 0 {
		t.Fatalf("imported %d words, merged %d, errors %d; want %d words", result.Created, result.Merged, len(result.Errors), len(want))
	}
	var exported bytes.Buffer
	if err := svc.ExportCSV(context.Background(), &exported); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(exported.Bytes(), []byte("term,translation,example,tags\n")) {
		t.Fatal("CSV export changed its header")
	}
	got := csvCards(t, exported.Bytes())
	if len(got) != len(want) {
		t.Fatalf("re-exported %d words, want %d", len(got), len(want))
	}
	for key, card := range want {
		if !reflect.DeepEqual(got[key], card) {
			t.Fatal("a word, its forms, examples or topic memberships changed in the CSV round trip")
		}
	}
	t.Logf("%d words retained their forms, translations, examples and topics", len(got))
}

func csvCards(t *testing.T, data []byte) map[[2]string]words.Card {
	t.Helper()
	rows, err := ParseCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	result := words.ParseRows(rows)
	if len(result.Errors) != 0 {
		t.Fatalf("%d invalid CSV records", len(result.Errors))
	}
	cards := make(map[[2]string]words.Card, len(result.Cards))
	for _, card := range result.Cards {
		card.Line = 0
		slices.Sort(card.Groups)
		cards[[2]string{card.Term, card.Translation}] = card
	}
	return cards
}
