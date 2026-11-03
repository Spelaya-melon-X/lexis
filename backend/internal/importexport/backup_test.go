package importexport

import (
	"encoding/json"
	"testing"
	"time"
)

func validBackup() Backup {
	g := int64(10)
	return Backup{
		Version: BackupVersion,
		Groups:  []BackupGroup{{ID: 10, Name: "food"}},
		Words:   []BackupWord{{ID: 1, Term: "apple", Translation: "яблоко", Level: 2, GroupIDs: []int64{10}}},
		Sessions: []BackupSession{{ID: 5, GroupID: &g, Strategy: "en_ru",
			Reviews: []BackupReview{{WordID: 1, Answer: "good", FirstInSession: true}}}},
	}
}

func TestValidateBackup(t *testing.T) {
	unknown := int64(99)
	tests := []struct {
		name   string
		mutate func(b *Backup)
		code   string // "" — валиден
	}{
		{"valid", func(b *Backup) {}, ""},
		{"empty but versioned", func(b *Backup) { *b = Backup{Version: 1} }, ""},
		{"session without group", func(b *Backup) { b.Sessions[0].GroupID = nil }, ""},
		{"wrong version", func(b *Backup) { b.Version = 2 }, "unsupported_backup_version"},
		{"missing version", func(b *Backup) { b.Version = 0 }, "unsupported_backup_version"},
		{"duplicate group id", func(b *Backup) { b.Groups = append(b.Groups, BackupGroup{ID: 10, Name: "x"}) }, "invalid_backup"},
		{"duplicate word id", func(b *Backup) { b.Words = append(b.Words, BackupWord{ID: 1, Term: "x"}) }, "invalid_backup"},
		{"word → unknown group", func(b *Backup) { b.Words[0].GroupIDs = []int64{99} }, "invalid_backup"},
		{"session → unknown group", func(b *Backup) { b.Sessions[0].GroupID = &unknown }, "invalid_backup"},
		{"review → unknown word", func(b *Backup) { b.Sessions[0].Reviews[0].WordID = 99 }, "invalid_backup"},
		{"duplicate session id", func(b *Backup) { b.Sessions = append(b.Sessions, BackupSession{ID: 5}) }, "invalid_backup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBackup()
			tt.mutate(&b)
			err := validateBackup(b)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			assertCode(t, err, tt.code)
		})
	}
}

func TestDecodeBackup(t *testing.T) {
	raw, err := json.Marshal(validBackup())
	if err != nil {
		t.Fatal(err)
	}
	b, err := DecodeBackup(raw)
	if err != nil || len(b.Words) != 1 || b.Words[0].GroupIDs[0] != 10 {
		t.Fatalf("decode: %+v, %v", b, err)
	}

	_, err = DecodeBackup([]byte(`{"version":1,"wordz":[]}`))
	assertCode(t, err, "invalid_backup") // неизвестное поле — не наш файл

	_, err = DecodeBackup([]byte(`{"version":3}`))
	assertCode(t, err, "unsupported_backup_version")

	_, err = DecodeBackup([]byte(`{"version":1,"exportedAt":"вчера"}`))
	assertCode(t, err, "invalid_backup")
}

func TestBackupJSONShape(t *testing.T) {
	// Фиксируем имена полей: файл бэкапа — внешний контракт, переименование ломает старые бэкапы.
	b := Backup{Version: 1, ExportedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		Words: []BackupWord{{ID: 1, Examples: []string{}, GroupIDs: []int64{}}}}
	raw, _ := json.Marshal(b)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"version", "exportedAt", "settings", "groups", "words", "sessions", "achievements"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing top-level key %q in %s", k, raw)
		}
	}
	w := m["words"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "term", "forms", "translation", "level", "streak", "nextReviewAt", "lastReviewedAt", "lastGoodAt", "createdAt", "examples", "groupIds"} {
		if _, ok := w[k]; !ok {
			t.Errorf("missing word key %q", k)
		}
	}
}

var testDay = time.Date(2026, 11, 2, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
