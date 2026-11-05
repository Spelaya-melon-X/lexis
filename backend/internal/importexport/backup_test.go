package importexport

import (
	"encoding/json"
	"strings"
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

func validHabits() *BackupHabits {
	h := int64(7)
	return &BackupHabits{
		Habits:   []BackupHabit{{ID: 7, Name: "Бег", Kind: "positive"}},
		Checkins: []BackupCheckin{{HabitID: 7, Day: "2026-11-01"}},
		Goals:    []BackupGoal{{HabitID: &h, From: "2026-11-01", To: "2026-11-09"}},
		Notes:    []BackupNote{{HabitID: &h, Kind: "insight", Body: "x"}},
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
		{"v1 without journal", func(b *Backup) { b.Version = 1 }, ""},
		{"v2 with journal", func(b *Backup) {
			b.Journal = &BackupJournal{Days: []BackupDay{{Day: "2026-11-02"}, {Day: "2026-11-03"}}}
		}, ""},
		{"v1 with journal", func(b *Backup) { b.Version = 1; b.Journal = &BackupJournal{} }, "invalid_backup"},
		{"bad journal day", func(b *Backup) { b.Journal = &BackupJournal{Days: []BackupDay{{Day: "2026-02-30"}}} }, "invalid_backup"},
		{"journal with weeks", func(b *Backup) {
			b.Journal = &BackupJournal{Weeks: []BackupWeek{{WeekStart: "2026-11-02", Blocks: []BackupBlock{{Topic: "x"}}}}}
		}, ""},
		{"bad week date", func(b *Backup) { b.Journal = &BackupJournal{Weeks: []BackupWeek{{WeekStart: "45"}}} }, "invalid_backup"},
		{"duplicate week", func(b *Backup) {
			b.Journal = &BackupJournal{Weeks: []BackupWeek{{WeekStart: "2026-11-02"}, {WeekStart: "2026-11-02"}}}
		}, "invalid_backup"},
		{"duplicate journal day", func(b *Backup) {
			b.Journal = &BackupJournal{Days: []BackupDay{{Day: "2026-11-02"}, {Day: "2026-11-02"}}}
		}, "invalid_backup"},
		{"future version", func(b *Backup) { b.Version = BackupVersion + 1 }, "unsupported_backup_version"},
		{"v2 with habits", func(b *Backup) { b.Habits = validHabits() }, ""},
		{"v1 with habits", func(b *Backup) { b.Version = 1; b.Habits = &BackupHabits{} }, "invalid_backup"},
		{"duplicate habit id", func(b *Backup) {
			b.Habits = validHabits()
			b.Habits.Habits = append(b.Habits.Habits, BackupHabit{ID: 7, Name: "x"})
		}, "invalid_backup"},
		{"checkin → unknown habit", func(b *Backup) { b.Habits = validHabits(); b.Habits.Checkins[0].HabitID = 99 }, "invalid_backup"},
		{"checkin bad day", func(b *Backup) { b.Habits = validHabits(); b.Habits.Checkins[0].Day = "вчера" }, "invalid_backup"},
		{"duplicate checkin", func(b *Backup) {
			b.Habits = validHabits()
			b.Habits.Checkins = append(b.Habits.Checkins, b.Habits.Checkins[0])
		}, "invalid_backup"},
		{"goal → unknown habit", func(b *Backup) { b.Habits = validHabits(); b.Habits.Goals[0].HabitID = &unknown }, "invalid_backup"},
		{"goal bad date", func(b *Backup) { b.Habits = validHabits(); b.Habits.Goals[0].To = "2026-02-30" }, "invalid_backup"},
		{"note → unknown habit", func(b *Backup) { b.Habits = validHabits(); b.Habits.Notes[0].HabitID = &unknown }, "invalid_backup"},
		{"general note", func(b *Backup) { b.Habits = validHabits(); b.Habits.Notes[0].HabitID = nil }, ""},
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
	b := Backup{Version: BackupVersion, ExportedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		Words: []BackupWord{{ID: 1, Examples: []string{}, GroupIDs: []int64{}}},
		Journal: &BackupJournal{
			Days:  []BackupDay{{Day: "2026-11-02", Wins: []string{}, Dopamine: []BackupDopamine{{Slot: 0, Text: "x"}}}},
			Weeks: []BackupWeek{{WeekStart: "2026-11-02", Blocks: []BackupBlock{{Topic: "Сон", Conclusion: "раньше"}}}},
		},
		Habits: validHabits()}
	raw, _ := json.Marshal(b)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"version", "exportedAt", "settings", "groups", "words", "sessions", "achievements", "journal", "habits"} {
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
	d := m["journal"].(map[string]any)["days"].([]any)[0].(map[string]any)
	for _, k := range []string{"day", "rating", "story", "wins", "dopamine", "createdAt", "updatedAt"} {
		if _, ok := d[k]; !ok {
			t.Errorf("missing journal day key %q", k)
		}
	}
	wk := m["journal"].(map[string]any)["weeks"].([]any)[0].(map[string]any)
	for _, k := range []string{"weekStart", "blocks", "createdAt", "updatedAt"} {
		if _, ok := wk[k]; !ok {
			t.Errorf("missing journal week key %q", k)
		}
	}
	if bl := wk["blocks"].([]any)[0].(map[string]any); bl["topic"] == nil || bl["conclusion"] == nil {
		t.Errorf("block keys: %v", bl)
	}
	sl := d["dopamine"].([]any)[0].(map[string]any)
	for _, k := range []string{"slot", "text", "updatedAt"} {
		if _, ok := sl[k]; !ok {
			t.Errorf("missing dopamine key %q", k)
		}
	}

	hb := m["habits"].(map[string]any)
	for sec, keys := range map[string][]string{
		"habits":   {"id", "name", "kind", "color", "trackTrigger", "trackRating", "trackAmount", "unit", "position", "archivedAt", "createdAt"},
		"checkins": {"habitId", "day", "trigger", "rating", "amount", "createdAt", "updatedAt"},
		"goals":    {"habitId", "title", "from", "to", "createdAt"},
		"notes":    {"habitId", "kind", "body", "createdAt", "updatedAt"},
	} {
		item := hb[sec].([]any)[0].(map[string]any)
		for _, k := range keys {
			if _, ok := item[k]; !ok {
				t.Errorf("missing habits.%s key %q", sec, k)
			}
		}
	}

	// Бэкап v1 (без journal) после маршалинга остаётся без ключа journal — omitempty.
	raw, _ = json.Marshal(Backup{Version: 1})
	if strings.Contains(string(raw), "journal") || strings.Contains(string(raw), "habits") {
		t.Errorf("v1 backup must not contain journal/habits: %s", raw)
	}
}

var testDay = time.Date(2026, 11, 2, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
