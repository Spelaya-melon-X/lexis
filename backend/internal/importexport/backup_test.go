package importexport

import (
	"encoding/json"
	"reflect"
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

func validGame() *BackupGame {
	typ := int64(3)
	return &BackupGame{
		Types:   []BackupTaskType{{ID: 3, Name: "Учёба"}},
		Columns: []BackupTaskColumn{{ID: 1, Name: "Входящие"}},
		Tasks:   []BackupTask{{ID: 8, Title: "Отчёт", Blocks: 2, Importance: 5, Deadline: new("2026-11-10"), TypeID: &typ, ColumnID: 1}},
		Plans:   []BackupPlanItem{{Day: "2026-11-05", TaskID: 8, Blocks: 2, Start: new(int16(540))}},
		Ritual:  []string{"Налил воды"},
		Sessions: []BackupFocusSession{{ID: 4, TaskID: new(int64(8)), TaskTitle: "Отчёт", Day: "2026-11-05", PlannedBlocks: 2,
			Breaks: []BackupFocusBreak{{}}, Distractions: []BackupDistraction{{Trigger: "телефон"}}}},
		Rewards: []BackupReward{{ID: 2, Title: "Кино", Price: 6}},
		Ledger:  []BackupPoints{{Delta: 2, Kind: "focus", Title: "Отчёт", SessionID: new(int64(4))}, {Delta: -1, Kind: "purchase", Title: "Кино", RewardID: new(int64(2))}},
	}
}

func validSport() *BackupSport {
	kg := 80.0
	return &BackupSport{
		Exercises: []BackupExercise{{ID: 3, Name: "Присед"}},
		Workouts: []BackupWorkout{{Day: "2026-11-05", Title: "Ноги",
			Exercises: []BackupWorkoutEntry{{ExerciseID: 3, Sets: 3, Reps: 5, Weight: &kg}}}},
		Meals: []BackupMeal{{Day: "2026-11-05", Text: "Каша"}},
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
		{"nested groups, parent later in file", func(b *Backup) {
			b.Groups = []BackupGroup{{ID: 11, Name: "airport", ParentID: new(int64(12))}, {ID: 12, Name: "travel"}, {ID: 10, Name: "food"}}
		}, ""},
		{"v2 with nested groups", func(b *Backup) {
			b.Version = 2
			b.Groups = append(b.Groups, BackupGroup{ID: 11, Name: "x", ParentID: new(int64(10))})
		}, "invalid_backup"},
		{"group → unknown parent", func(b *Backup) { b.Groups[0].ParentID = &unknown }, "invalid_backup"},
		{"group → parent 0", func(b *Backup) { b.Groups[0].ParentID = new(int64(0)) }, "invalid_backup"},
		{"group cycle", func(b *Backup) {
			b.Groups = []BackupGroup{{ID: 10, Name: "a", ParentID: new(int64(11))}, {ID: 11, Name: "b", ParentID: new(int64(10))}}
		}, "invalid_backup"},
		{"groups too deep", func(b *Backup) {
			b.Groups = []BackupGroup{{ID: 10, Name: "a"}, {ID: 11, Name: "b", ParentID: new(int64(10))},
				{ID: 12, Name: "c", ParentID: new(int64(11))}, {ID: 13, Name: "d", ParentID: new(int64(12))}}
		}, "invalid_backup"},
		{"v3 with game", func(b *Backup) { b.Game = validGame() }, ""},
		{"v2 with game", func(b *Backup) { b.Version = 2; b.Game = validGame() }, "invalid_backup"},
		{"task → unknown column", func(b *Backup) { b.Game = validGame(); b.Game.Tasks[0].ColumnID = 99 }, "invalid_backup"},
		{"task → unknown type", func(b *Backup) { b.Game = validGame(); b.Game.Tasks[0].TypeID = &unknown }, "invalid_backup"},
		{"task without type", func(b *Backup) { b.Game = validGame(); b.Game.Tasks[0].TypeID = nil }, ""},
		{"task step, parent later in file", func(b *Backup) {
			b.Game = validGame()
			b.Game.Tasks = append([]BackupTask{{ID: 9, ParentID: new(int64(8)), Title: "шаг", Blocks: 1, Importance: 5, ColumnID: 1}}, b.Game.Tasks...)
		}, ""},
		{"task → unknown parent", func(b *Backup) { b.Game = validGame(); b.Game.Tasks[0].ParentID = new(int64(99)) }, "invalid_backup"},
		{"task → parent 0", func(b *Backup) { b.Game = validGame(); b.Game.Tasks[0].ParentID = new(int64(0)) }, "invalid_backup"},
		{"task cycle", func(b *Backup) {
			b.Game = validGame()
			b.Game.Tasks = []BackupTask{{ID: 1, ParentID: new(int64(2)), ColumnID: 1}, {ID: 2, ParentID: new(int64(1)), ColumnID: 1}}
		}, "invalid_backup"},
		{"plan item", func(b *Backup) {
			b.Game = validGame()
			b.Game.Plans = []BackupPlanItem{{Day: "2026-11-05", TaskID: 8, Blocks: 2}}
		}, ""},
		{"plan → unknown task", func(b *Backup) {
			b.Game = validGame()
			b.Game.Plans = []BackupPlanItem{{Day: "2026-11-05", TaskID: 99, Blocks: 2}}
		}, "invalid_backup"},
		{"plan bad day", func(b *Backup) {
			b.Game = validGame()
			b.Game.Plans = []BackupPlanItem{{Day: "завтра", TaskID: 8, Blocks: 2}}
		}, "invalid_backup"},
		{"plan duplicate", func(b *Backup) {
			b.Game = validGame()
			b.Game.Plans = []BackupPlanItem{{Day: "2026-11-05", TaskID: 8, Blocks: 2}, {Day: "2026-11-05", TaskID: 8, Blocks: 1, Position: 1}}
		}, "invalid_backup"},
		{"session → unknown task", func(b *Backup) { b.Game = validGame(); b.Game.Sessions[0].TaskID = new(int64(99)) }, "invalid_backup"},
		{"session without task (deleted)", func(b *Backup) { b.Game = validGame(); b.Game.Sessions[0].TaskID = nil }, ""},
		{"session bad day", func(b *Backup) { b.Game = validGame(); b.Game.Sessions[0].Day = "x" }, "invalid_backup"},
		{"points → unknown session", func(b *Backup) { b.Game = validGame(); b.Game.Ledger[0].SessionID = new(int64(99)) }, "invalid_backup"},
		{"points → unknown reward", func(b *Backup) { b.Game = validGame(); b.Game.Ledger[1].RewardID = new(int64(99)) }, "invalid_backup"},
		{"session credited twice", func(b *Backup) { b.Game = validGame(); b.Game.Ledger[1].SessionID = new(int64(4)) }, "invalid_backup"},
		{"duplicate reward id", func(b *Backup) { b.Game = validGame(); b.Game.Rewards = append(b.Game.Rewards, b.Game.Rewards[0]) }, "invalid_backup"},
		{"v3 with sport", func(b *Backup) { b.Sport = validSport() }, ""},
		{"v2 with sport", func(b *Backup) { b.Version = 2; b.Sport = validSport() }, "invalid_backup"},
		{"workout → unknown exercise", func(b *Backup) { b.Sport = validSport(); b.Sport.Workouts[0].Exercises[0].ExerciseID = 99 }, "invalid_backup"},
		{"workout bad day", func(b *Backup) { b.Sport = validSport(); b.Sport.Workouts[0].Day = "x" }, "invalid_backup"},
		{"bodyweight entry", func(b *Backup) { b.Sport = validSport(); b.Sport.Workouts[0].Exercises[0].Weight = nil }, ""},
		{"meal bad day", func(b *Backup) { b.Sport = validSport(); b.Sport.Meals[0].Day = "x" }, "invalid_backup"},
		{"zero sets", func(b *Backup) { b.Sport = validSport(); b.Sport.Workouts[0].Exercises[0].Sets = 0 }, "invalid_backup"},
		{"duplicate exercise id", func(b *Backup) {
			b.Sport = validSport()
			b.Sport.Exercises = append(b.Sport.Exercises, b.Sport.Exercises[0])
		}, "invalid_backup"},
		{"task bad deadline", func(b *Backup) { b.Game = validGame(); b.Game.Tasks[0].Deadline = new("2026-13-01") }, "invalid_backup"},
		{"duplicate task id", func(b *Backup) { b.Game = validGame(); b.Game.Tasks = append(b.Game.Tasks, b.Game.Tasks[0]) }, "invalid_backup"},
		{"duplicate column id", func(b *Backup) {
			b.Game = validGame()
			b.Game.Columns = append(b.Game.Columns, BackupTaskColumn{ID: 1, Name: "x"})
		}, "invalid_backup"},
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

	_, err = DecodeBackup([]byte(`{"version":4}`))
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
		Habits: validHabits(), Game: validGame(), Sport: validSport()}
	raw, _ := json.Marshal(b)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for _, k := range []string{"version", "exportedAt", "settings", "groups", "words", "sessions", "achievements", "journal", "habits", "game", "sport"} {
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

	gm := m["game"].(map[string]any)
	for sec, keys := range map[string][]string{
		"types":    {"id", "name", "color", "position", "createdAt"},
		"columns":  {"id", "name", "position", "createdAt"},
		"plans":    {"day", "taskId", "position", "blocks", "start"},
		"sessions": {"id", "taskId", "taskTitle", "day", "plannedBlocks", "startedAt", "workStartedAt", "finishedAt", "outcome", "breaks", "distractions"},
		"rewards":  {"id", "title", "price", "position", "archivedAt", "createdAt"},
		"ledger":   {"at", "delta", "kind", "title", "sessionId", "rewardId"},
		"tasks":    {"id", "parentId", "title", "blocks", "importance", "deadline", "typeId", "columnId", "position", "note", "doneAt", "createdAt", "updatedAt"},
	} {
		item := gm[sec].([]any)[0].(map[string]any)
		for _, k := range keys {
			if _, ok := item[k]; !ok {
				t.Errorf("missing game.%s key %q", sec, k)
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

func TestFillPlanStarts(t *testing.T) {
	items := []BackupPlanItem{
		{Day: "2026-11-05", Position: 1, Blocks: 2},                          // после пункта с position 0 (4 блока) → 11:00
		{Day: "2026-11-05", Position: 0, Blocks: 4},                          // 9:00
		{Day: "2026-11-06", Position: 0, Blocks: 1, Start: new(int16(1200))}, // своё время не трогаем
		{Day: "2026-11-06", Position: 1, Blocks: 1},                          // другой день — с 9:00
	}
	got := fillPlanStarts(items)
	if want := []int16{660, 540, 1200, 540}; !reflect.DeepEqual(got, want) {
		t.Errorf("starts = %v, want %v", got, want)
	}
}
