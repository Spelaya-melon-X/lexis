//go:build integration

// Интеграционные тесты importexport на живом Postgres (запуск — SPEC §15).
package importexport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/db/dbtest"
	"lexis/internal/platform/eventbus"
	"lexis/internal/words"
)

type nopBus struct{}

func (nopBus) Publish(eventbus.Event) {}

func newTestEnv(t *testing.T) (*Service, *words.Service, dbtest.Fixtures) {
	t.Helper()
	pool := dbtest.New(t)
	clk := clock.NewFixed(testDay)
	ws := words.NewService(pool, clk, nopBus{})
	return NewService(pool, clk, ws), ws, dbtest.Fixtures{Pool: pool, Now: testDay}
}

func mustImport(t *testing.T, svc *Service, data string) ImportResult {
	t.Helper()
	res, err := svc.Import(context.Background(), []byte(data))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return res
}

func TestImport_CSV(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()

	res := mustImport(t, svc, "term,translation,example,tags\n"+
		"apple,яблоко,I eat an [apple] <> An apple a day,food;nouns\n"+
		",пусто,,\n"+
		"go_went_gone,идти,I [went] home,verbs\n")
	if res.Kind != "words" || res.Created != 2 || res.Merged != 0 || len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Fatalf("result = %+v", res)
	}

	// Повторный импорт с тем же словом в другом регистре сливается, как bulk.
	res = mustImport(t, svc, "term,translation,tags\nAPPLE,ЯБЛОКО,fruits\n")
	if res.Created != 0 || res.Merged != 1 {
		t.Errorf("merge = %+v", res)
	}

	list, err := ws.List(ctx, words.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var apple words.Word
	for _, w := range list {
		if w.Term == "apple" {
			apple = w
		}
	}
	var groups []string
	for _, g := range apple.Groups {
		groups = append(groups, g.Name)
	}
	if len(apple.Examples) != 2 || !reflect.DeepEqual(groups, []string{"food", "fruits", "nouns"}) {
		t.Errorf("apple = %+v", apple)
	}
	if n := fx.Int(t, `SELECT count(*) FROM words WHERE cardinality(forms) = 3`); n != 1 {
		t.Errorf("verb forms not imported")
	}
}

func TestImport_JSONList(t *testing.T) {
	svc, _, fx := newTestEnv(t)
	res := mustImport(t, svc, `[{"term":"a","translation":"1","tags":"g"},{"term":"b","translation":"2"},{"term":"","translation":"x"}]`)
	if res.Kind != "words" || res.Created != 2 || len(res.Errors) != 1 || res.Errors[0].Line != 3 {
		t.Errorf("result = %+v", res)
	}
	if n := fx.Int(t, `SELECT count(*) FROM word_groups`); n != 1 {
		t.Errorf("word_groups = %d", n)
	}
}

// seed — небольшая, но полная база: группы, слова с примерами и формами, сессии с ответами,
// удалённая группа у сессии, ачивка, настройки.
func seed(t *testing.T, ws *words.Service, fx dbtest.Fixtures) {
	t.Helper()
	ctx := context.Background()
	if _, err := ws.Bulk(ctx, "apple || яблоко || I eat an [apple] <> Red [apple] || food nouns\n"+
		"go_went_gone || идти || I [go] <> I [went] || verbs\n"+
		"pear || груша || || food"); err != nil {
		t.Fatalf("Bulk: %v", err)
	}
	fx.Exec(t, `UPDATE words SET level = 4, streak = 1, last_good_at = $1, last_reviewed_at = $1 WHERE term = 'apple'`, testDay.Add(-time.Hour))
	food := fx.Int(t, `SELECT id FROM groups WHERE name = 'food'`)
	apple := fx.Int(t, `SELECT id FROM words WHERE term = 'apple'`)
	pear := fx.Int(t, `SELECT id FROM words WHERE term = 'pear'`)

	fin := testDay.Add(-30 * time.Minute)
	s1 := fx.Session(t, dbtest.Session{GroupID: &food, FinishedAt: &fin, Total: 2, Good: 1, Again: 1, DurationSec: 60})
	fx.Review(t, s1, apple, "good", true, fin)
	fx.Review(t, s1, pear, "again", true, fin)
	fx.Review(t, s1, pear, "good", false, fin)
	fx.Session(t, dbtest.Session{Free: true}) // незавершённая, без группы

	fx.Exec(t, `INSERT INTO achievements (code, unlocked_at) VALUES ('first_session', $1)`, fin)
	fx.Exec(t, `INSERT INTO settings (key, value) VALUES ('language', '"en"'), ('new_words_limit', '7')`)
	// Дерево групп (v3): verbs → food → nouns. Родитель food (verbs) стоит в файле позже —
	// восстановление не должно зависеть от порядка.
	fx.Exec(t, `UPDATE groups SET parent_id = (SELECT id FROM groups WHERE name = 'verbs') WHERE name = 'food'`)
	fx.Exec(t, `UPDATE groups SET parent_id = (SELECT id FROM groups WHERE name = 'food') WHERE name = 'nouns'`)
	// Дыра в id: удалённое слово — восстановление не должно зависеть от непрерывности id.
	fx.Exec(t, `DELETE FROM words WHERE term = 'go_went_gone'`)
	if _, err := ws.Bulk(ctx, "go_went_gone || идти || I [go] <> I [went] || verbs"); err != nil {
		t.Fatalf("Bulk: %v", err)
	}

	// Раздел «День»: день со всем, день только с дофамином, пустой день (строка есть, данных нет).
	fx.Exec(t, `INSERT INTO days (day, rating, story, created_at, updated_at) VALUES
		('2026-11-01', 8, 'история', $1, $1), ('2026-11-02', NULL, '', $1, $1), ('2026-10-01', NULL, '', $1, $1)`, fin)
	fx.Exec(t, `INSERT INTO day_wins (day, position, text) VALUES ('2026-11-01', 0, 'бег'), ('2026-11-01', 1, 'книга')`)
	fx.Exec(t, `INSERT INTO week_reviews (week_start, created_at, updated_at) VALUES ('2026-10-26', $1, $1)`, fin)
	fx.Exec(t, `INSERT INTO week_review_blocks (week_start, position, topic, conclusion) VALUES
		('2026-10-26', 0, 'Сон', 'ложиться раньше'), ('2026-10-26', 1, '', 'только вывод')`)
	fx.Exec(t, `INSERT INTO dopamine_slots (day, slot, text, updated_at) VALUES
		('2026-11-01', 0, 'кофе', $1), ('2026-11-01', 5, 'музыка', $1), ('2026-11-02', 6, 'солнце', $1)`, fin)

	// Привычки: дыра в id (удалённая привычка), архивная, общая заметка, цель без привычки.
	fx.Exec(t, `INSERT INTO habits (name, kind, color, track_amount, unit, position, created_at) VALUES
		('удалённая', 'positive', NULL, false, '', 9, $1),
		('Бег', 'positive', '#16a34a', true, 'км', 0, $1)`, fin)
	fx.Exec(t, `DELETE FROM habits WHERE name = 'удалённая'`)
	fx.Exec(t, `INSERT INTO habits (name, kind, track_trigger, position, archived_at, created_at) VALUES ('Курение', 'negative', true, 1, $1, $1)`, fin)
	run := fx.Int(t, `SELECT id FROM habits WHERE name = 'Бег'`)
	smoke := fx.Int(t, `SELECT id FROM habits WHERE name = 'Курение'`)
	fx.Exec(t, `INSERT INTO habit_checkins (habit_id, day, trigger, rating, amount, created_at, updated_at) VALUES
		($1, '2026-11-01', NULL, NULL, 5.5, $3, $3), ($2, '2026-11-01', 'стресс', NULL, NULL, $3, $3)`, run, smoke, fin)
	fx.Exec(t, `INSERT INTO habit_goals (habit_id, title, start_day, end_day, created_at) VALUES
		($1, '', '2026-11-01', '2026-11-09', $2), (NULL, 'Спать до 23', '2026-12-01', '2026-12-07', $2)`, run, fin)
	fx.Exec(t, `INSERT INTO habit_notes (habit_id, kind, body, created_at, updated_at) VALUES
		($1, 'observation', 'первая', $2, $2), (NULL, 'insight', 'вторая', $2, $2)`, run, fin)

	// «Игра»: дыра в id типа (удалённый), дело без типа, выполненное дело, вторая колонка раньше первой по порядку.
	fx.Exec(t, `INSERT INTO task_types (name, color, position, created_at) VALUES ('удалённый', NULL, 0, $1), ('Учёба', '#16a34a', 1, $1)`, fin)
	fx.Exec(t, `DELETE FROM task_types WHERE name = 'удалённый'`)
	fx.Exec(t, `INSERT INTO task_columns (name, position, created_at) VALUES ('Входящие', 1, $1), ('Сегодня', 0, $1)`, fin)
	study := fx.Int(t, `SELECT id FROM task_types WHERE name = 'Учёба'`)
	inbox := fx.Int(t, `SELECT id FROM task_columns WHERE name = 'Входящие'`)
	fx.Exec(t, `INSERT INTO tasks (title, blocks, importance, deadline, type_id, column_id, position, note, done_at, created_at, updated_at) VALUES
		('Курсовая', 12, 8, '2026-11-20', $1, $2, 0, 'глава 1', NULL, $3, $3),
		('Шкаф', 2, 3, NULL, NULL, $2, 1, '', $3, $3, $3)`, study, inbox, fin)
	// Шаг курсовой (B2): ссылка на родителя должна пережить круговой бэкап.
	fx.Exec(t, `INSERT INTO tasks (parent_id, title, blocks, importance, column_id, position, created_at, updated_at)
		SELECT id, 'Глава 1', 4, 8, column_id, 2, $1, $1 FROM tasks WHERE title = 'Курсовая'`, fin)
	// План дня (B3): пункт на шаг курсовой.
	fx.Exec(t, `INSERT INTO plan_items (day, task_id, position, blocks, start_min) SELECT '2026-11-02', id, 0, 2, 600 FROM tasks WHERE title = 'Глава 1'`)
	// Фокус и награды (B4): ритуал, завершённая сессия с перерывом и отвлечением, награда, начисление и трата.
	fx.Exec(t, `INSERT INTO ritual_items (text, position) VALUES ('Налил воды', 0), ('Не беспокоить', 1)`)
	fx.Exec(t, `INSERT INTO focus_sessions (task_id, task_title, day, planned_blocks, started_at, work_started_at, finished_at, outcome)
		SELECT id, 'Глава 1', '2026-11-02', 2, $1, $1, $1::timestamptz + interval '70 minutes', 'stopped' FROM tasks WHERE title = 'Глава 1'`, fin)
	sess := fx.Int(t, `SELECT id FROM focus_sessions`)
	fx.Exec(t, `INSERT INTO focus_breaks (session_id, started_at, ended_at) VALUES ($1, $2, $2::timestamptz + interval '5 minutes')`, sess, fin)
	fx.Exec(t, `INSERT INTO focus_distractions (session_id, at, trigger, gave_in) VALUES ($1, $2, 'телефон', true)`, sess, fin)
	fx.Exec(t, `INSERT INTO rewards (title, price, position, created_at) VALUES ('Кино', 2, 0, $1)`, fin)
	fx.Exec(t, `INSERT INTO points_ledger (at, delta, kind, title, focus_session_id) VALUES ($1, 2, 'focus', 'Глава 1', $2)`, fin, sess)
	fx.Exec(t, `INSERT INTO points_ledger (at, delta, kind, title, reward_id) SELECT $1, -2, 'purchase', 'Кино', id FROM rewards`, fin)

	// Спорт (C1): дыра в id упражнения, тренировка с упражнением на вес и упражнением на время.
	fx.Exec(t, `INSERT INTO exercises (name, created_at) VALUES ('удалённое', $1), ('Присед', $1), ('Подтягивания', $1)`, fin)
	fx.Exec(t, `DELETE FROM exercises WHERE name = 'удалённое'`)
	fx.Exec(t, `INSERT INTO workouts (day, title, notes, duration_min, kcal, created_at, updated_at) VALUES ('2026-11-01', 'Ноги', 'тяжело', 60, 420, $1, $1)`, fin)
	fx.Exec(t, `INSERT INTO workout_exercises (workout_id, exercise_id, position, sets, reps, weight) SELECT 1, id, 0, 3, 5, 82.5 FROM exercises WHERE name = 'Присед'`)
	fx.Exec(t, `INSERT INTO workout_exercises (workout_id, exercise_id, position, sets, reps, weight) SELECT 1, id, 1, 4, 8, NULL FROM exercises WHERE name = 'Подтягивания'`)
	fx.Exec(t, `INSERT INTO meals (day, position, text, kcal, created_at) VALUES ('2026-11-01', 0, 'Каша', 350, $1), ('2026-11-01', 1, 'Яблоко', NULL, $1)`, fin)
}

// canonical заменяет id порядковыми номерами (ссылки — тоже), чтобы сравнивать снимки
// до и после восстановления: новые id не обязаны совпадать со старыми.
func canonical(t *testing.T, b Backup) string {
	t.Helper()
	b.ExportedAt = time.Time{}
	gIdx, wIdx := map[int64]int64{}, map[int64]int64{}
	for i := range b.Groups {
		gIdx[b.Groups[i].ID] = int64(i)
		b.Groups[i].ID = int64(i)
	}
	for i := range b.Groups { // отдельным проходом: родитель может стоять позже ребёнка
		if p := b.Groups[i].ParentID; p != nil {
			v := gIdx[*p]
			b.Groups[i].ParentID = &v
		}
	}
	for i := range b.Words {
		wIdx[b.Words[i].ID] = int64(i)
		b.Words[i].ID = int64(i)
		for j, g := range b.Words[i].GroupIDs {
			b.Words[i].GroupIDs[j] = gIdx[g]
		}
	}
	if b.Habits != nil {
		hIdx := map[int64]int64{}
		for i := range b.Habits.Habits {
			hIdx[b.Habits.Habits[i].ID] = int64(i)
			b.Habits.Habits[i].ID = int64(i)
		}
		remap := func(id *int64) *int64 {
			if id == nil {
				return nil
			}
			v := hIdx[*id]
			return &v
		}
		for i := range b.Habits.Checkins {
			b.Habits.Checkins[i].HabitID = hIdx[b.Habits.Checkins[i].HabitID]
		}
		for i := range b.Habits.Goals {
			b.Habits.Goals[i].HabitID = remap(b.Habits.Goals[i].HabitID)
		}
		for i := range b.Habits.Notes {
			b.Habits.Notes[i].HabitID = remap(b.Habits.Notes[i].HabitID)
		}
	}
	if b.Game != nil {
		tIdx, cIdx := map[int64]int64{}, map[int64]int64{}
		for i := range b.Game.Types {
			tIdx[b.Game.Types[i].ID] = int64(i)
			b.Game.Types[i].ID = int64(i)
		}
		for i := range b.Game.Columns {
			cIdx[b.Game.Columns[i].ID] = int64(i)
			b.Game.Columns[i].ID = int64(i)
		}
		taskIdx := map[int64]int64{} // до замены id: родитель может стоять позже шага
		for i, x := range b.Game.Tasks {
			taskIdx[x.ID] = int64(i)
		}
		for i := range b.Game.Tasks {
			x := &b.Game.Tasks[i]
			x.ID, x.ColumnID = int64(i), cIdx[x.ColumnID]
			if x.TypeID != nil {
				x.TypeID = new(tIdx[*x.TypeID])
			}
			if x.ParentID != nil {
				x.ParentID = new(taskIdx[*x.ParentID])
			}
		}
		for i := range b.Game.Plans {
			b.Game.Plans[i].TaskID = taskIdx[b.Game.Plans[i].TaskID]
		}
		sIdx, rIdx := map[int64]int64{}, map[int64]int64{}
		for i := range b.Game.Sessions {
			x := &b.Game.Sessions[i]
			sIdx[x.ID], x.ID = int64(i), int64(i)
			if x.TaskID != nil {
				x.TaskID = new(taskIdx[*x.TaskID])
			}
		}
		for i := range b.Game.Rewards {
			rIdx[b.Game.Rewards[i].ID], b.Game.Rewards[i].ID = int64(i), int64(i)
		}
		for i := range b.Game.Ledger {
			x := &b.Game.Ledger[i]
			if x.SessionID != nil {
				x.SessionID = new(sIdx[*x.SessionID])
			}
			if x.RewardID != nil {
				x.RewardID = new(rIdx[*x.RewardID])
			}
		}
	}
	if b.Sport != nil {
		eIdx := map[int64]int64{}
		for i := range b.Sport.Exercises {
			eIdx[b.Sport.Exercises[i].ID], b.Sport.Exercises[i].ID = int64(i), int64(i)
		}
		for i := range b.Sport.Workouts {
			for j := range b.Sport.Workouts[i].Exercises {
				x := &b.Sport.Workouts[i].Exercises[j]
				x.ExerciseID = eIdx[x.ExerciseID]
			}
		}
	}
	for i := range b.Sessions {
		b.Sessions[i].ID = int64(i)
		if g := b.Sessions[i].GroupID; g != nil {
			v := gIdx[*g]
			b.Sessions[i].GroupID = &v
		}
		for j := range b.Sessions[i].Reviews {
			b.Sessions[i].Reviews[j].WordID = wIdx[b.Sessions[i].Reviews[j].WordID]
		}
	}
	raw, err := json.MarshalIndent(b, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// groupParents — имя группы → имя родителя ("" — корень).
func groupParents(b Backup) map[string]string {
	names := map[int64]string{}
	for _, g := range b.Groups {
		names[g.ID] = g.Name
	}
	m := map[string]string{}
	for _, g := range b.Groups {
		m[g.Name] = ""
		if g.ParentID != nil {
			m[g.Name] = names[*g.ParentID]
		}
	}
	return m
}

func TestBackup_RoundTrip(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()
	seed(t, ws, fx)

	b1, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatalf("ExportBackup: %v", err)
	}
	if b1.Version != BackupVersion || b1.Journal == nil || len(b1.Journal.Days) != 3 {
		t.Fatalf("journal section = %+v", b1.Journal)
	}
	if w := b1.Journal.Weeks; len(w) != 1 || len(w[0].Blocks) != 2 || w[0].Blocks[1].Conclusion != "только вывод" {
		t.Fatalf("journal weeks = %+v", w)
	}
	if d := b1.Journal.Days[1]; d.Day != "2026-11-01" || *d.Rating != 8 || !reflect.DeepEqual(d.Wins, []string{"бег", "книга"}) || len(d.Dopamine) != 2 {
		t.Fatalf("journal day = %+v", d)
	}
	if h := b1.Habits; h == nil || len(h.Habits) != 2 || len(h.Checkins) != 2 || len(h.Goals) != 2 || len(h.Notes) != 2 || h.Habits[1].ArchivedAt == nil {
		t.Fatalf("habits section = %+v", b1.Habits)
	}
	if g := b1.Game; g == nil || len(g.Types) != 1 || len(g.Columns) != 2 || len(g.Tasks) != 3 || g.Tasks[1].DoneAt == nil || g.Tasks[2].ParentID == nil || *g.Tasks[0].Deadline != "2026-11-20" ||
		len(g.Plans) != 1 || len(g.Ritual) != 2 || len(g.Sessions) != 1 || len(g.Sessions[0].Breaks) != 1 ||
		len(g.Sessions[0].Distractions) != 1 || len(g.Rewards) != 1 || len(g.Ledger) != 2 {
		t.Fatalf("game section = %+v", b1.Game)
	}
	if sp := b1.Sport; sp == nil || len(sp.Exercises) != 2 || len(sp.Workouts) != 1 || len(sp.Workouts[0].Exercises) != 2 ||
		sp.Workouts[0].Exercises[0].Sets != 3 || *sp.Workouts[0].Exercises[0].Weight != 82.5 || sp.Workouts[0].Exercises[1].Weight != nil ||
		len(sp.Meals) != 2 || sp.Meals[1].Text != "Яблоко" || sp.Meals[1].Kcal != nil {
		t.Fatalf("sport section = %+v", b1.Sport)
	}
	if parents := groupParents(b1); parents["food"] != "verbs" || parents["nouns"] != "food" || parents["verbs"] != "" {
		t.Fatalf("group tree in backup = %v", parents)
	}
	if len(b1.Words) != 3 || len(b1.Groups) != 3 || len(b1.Sessions) != 2 || len(b1.Achievements) != 1 || len(b1.Settings) != 2 {
		t.Fatalf("snapshot = %d words %d groups %d sessions %d ach %d settings",
			len(b1.Words), len(b1.Groups), len(b1.Sessions), len(b1.Achievements), len(b1.Settings))
	}

	// Меняем базу, потом восстанавливаем через Import (как с фронта: JSON-файл).
	if _, err := ws.Bulk(ctx, "extra || лишнее || || junk"); err != nil {
		t.Fatal(err)
	}
	fx.Exec(t, `UPDATE settings SET value = '"ru"' WHERE key = 'language'`)
	fx.Exec(t, `DELETE FROM dopamine_slots WHERE slot = 0`)
	fx.Exec(t, `INSERT INTO days (day, created_at, updated_at) VALUES ('2026-09-01', $1, $1)`, testDay)
	fx.Exec(t, `DELETE FROM habit_notes`)
	fx.Exec(t, `DELETE FROM week_reviews`)
	fx.Exec(t, `INSERT INTO habits (name, kind, position, created_at) VALUES ('лишняя', 'positive', 5, $1)`, testDay)
	fx.Exec(t, `DELETE FROM tasks WHERE title = 'Шкаф'`)
	raw, _ := json.Marshal(b1)
	res := mustImport(t, svc, string(raw))
	if res.Kind != "backup" || res.Restored == nil || *res.Restored != (RestoreCounts{Groups: 3, Words: 3, Sessions: 2, Reviews: 3, Achievements: 1, Days: 3, Habits: 2, Tasks: 3, Workouts: 1}) {
		t.Fatalf("restore result = %+v", res)
	}

	b2, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatalf("ExportBackup after restore: %v", err)
	}
	if c1, c2 := canonical(t, b1), canonical(t, b2); c1 != c2 {
		t.Errorf("round trip differs:\n--- before\n%s\n--- after\n%s", c1, c2)
	}
	if parents := groupParents(b2); parents["food"] != "verbs" || parents["nouns"] != "food" {
		t.Errorf("group tree after restore = %v", parents)
	}
	// После восстановления обычная работа продолжается: новые id не конфликтуют.
	if _, err := ws.Bulk(ctx, "new || новое || || food"); err != nil {
		t.Errorf("Bulk after restore: %v", err)
	}
}

// Бэкап v1 (до раздела «День») восстанавливает английский и не трогает дневник (§17.10).
func TestRestore_V1KeepsJournal(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()
	seed(t, ws, fx)
	b, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b.Version, b.Journal, b.Habits, b.Game, b.Sport, b.Ambitions, b.Study, b.StudyHistory = 1, nil, nil, nil, nil, nil, nil, nil
	b.Words = b.Words[:1]
	b.Sessions = nil
	for i := range b.Groups { // в настоящем v1-файле поля parentId нет
		b.Groups[i].ParentID = nil
	}

	raw, _ := json.Marshal(b)
	res := mustImport(t, svc, string(raw))
	if res.Restored == nil || res.Restored.Words != 1 || res.Restored.Days != 0 || res.Restored.Tasks != 0 {
		t.Fatalf("restored = %+v", res.Restored)
	}
	if n := fx.Int(t, `SELECT count(*) FROM tasks`); n != 3 {
		t.Errorf("game must survive v1 restore, tasks = %d", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM workout_exercises`); n != 2 {
		t.Errorf("sport must survive v1 restore, entries = %d", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM words`); n != 1 {
		t.Errorf("words = %d, want 1", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM groups WHERE parent_id IS NOT NULL`); n != 0 {
		t.Errorf("v1 restore: all groups must be roots, %d nested", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM days`) + fx.Int(t, `SELECT count(*) FROM day_wins`) + fx.Int(t, `SELECT count(*) FROM dopamine_slots`); n != 3+2+3 {
		t.Errorf("journal must survive v1 restore, rows = %d", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM habits`) + fx.Int(t, `SELECT count(*) FROM habit_checkins`) + fx.Int(t, `SELECT count(*) FROM habit_notes`); n != 2+2+2 {
		t.Errorf("habits must survive v1 restore, rows = %d", n)
	}
}

// Битая секция journal откатывает всё восстановление, английский в том числе.
func TestRestore_InvalidJournalRollsBack(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()
	seed(t, ws, fx)
	b, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b.Words[0].Term = "changed"
	b.Journal.Days[0].Dopamine = []BackupDopamine{{Slot: 9, Text: "x"}} // CHECK slot 0..6

	_, err = svc.Restore(ctx, b)
	assertCode(t, err, "invalid_backup")
	if n := fx.Int(t, `SELECT count(*) FROM words WHERE term = 'changed'`); n != 0 {
		t.Errorf("words must be untouched")
	}
	if n := fx.Int(t, `SELECT count(*) FROM dopamine_slots`); n != 3 {
		t.Errorf("journal must be untouched, dopamine rows = %d", n)
	}
}

func TestRestore_InvalidRollsBack(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	ctx := context.Background()
	seed(t, ws, fx)
	b, err := svc.ExportBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Ссылки целы, но уровень 9 нарушает CHECK — падает посреди транзакции.
	b.Words[len(b.Words)-1].Level = 9
	b.Words[0].Term = "changed"
	_, err = svc.Restore(ctx, b)
	assertCode(t, err, "invalid_backup")

	if n := fx.Int(t, `SELECT count(*) FROM words WHERE term = 'apple'`); n != 1 {
		t.Errorf("data must be untouched after failed restore")
	}
	if n := fx.Int(t, `SELECT count(*) FROM settings`); n != 2 {
		t.Errorf("settings must be untouched, got %d", n)
	}
}

// «Очистить всё»: английский + дневник + привычки, настройки остаются.
func TestReset_All(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	seed(t, ws, fx)
	if err := svc.Reset(context.Background(), ScopeAll); err != nil {
		t.Fatalf("Reset all: %v", err)
	}
	for _, table := range []string{"words", "groups", "training_sessions", "achievements", "days", "day_wins", "dopamine_slots",
		"week_reviews", "week_review_blocks", "habits", "habit_checkins", "habit_goals", "habit_notes",
		"task_types", "task_columns", "tasks", "plan_items", "ritual_items", "focus_sessions", "focus_breaks",
		"focus_distractions", "rewards", "points_ledger", "exercises", "workouts", "workout_exercises", "meals"} {
		if n := fx.Int(t, `SELECT count(*) FROM `+table); n != 0 {
			t.Errorf("%s not empty: %d", table, n)
		}
	}
	if n := fx.Int(t, `SELECT count(*) FROM settings`); n != 2 {
		t.Errorf("settings must survive, got %d", n)
	}
}

func TestReset(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	seed(t, ws, fx)
	if err := svc.Reset(context.Background(), ScopeEnglish); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for _, table := range []string{"words", "word_examples", "groups", "word_groups", "training_sessions", "reviews", "achievements"} {
		if n := fx.Int(t, `SELECT count(*) FROM `+table); n != 0 {
			t.Errorf("%s not empty: %d", table, n)
		}
	}
	if n := fx.Int(t, `SELECT count(*) FROM settings`); n != 2 {
		t.Errorf("settings must survive reset, got %d", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM days`); n != 3 {
		t.Errorf("reset of English must keep journal, days = %d", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM habits`); n != 2 {
		t.Errorf("reset of English must keep habits, got %d", n)
	}
	if n := fx.Int(t, `SELECT count(*) FROM tasks`); n != 3 {
		t.Errorf("reset of English must keep game, got %d", n)
	}
	if id := fx.Group(t, "fresh"); id != 1 {
		t.Errorf("ids must restart, got %d", id)
	}
}

func TestModule_HTTP(t *testing.T) {
	svc, ws, fx := newTestEnv(t)
	seed(t, ws, fx)
	r := chi.NewRouter()
	r.Route("/api/data", (&Module{svc: svc}).Routes)
	do := func(method, path string, body *strings.Reader, ct string) *httptest.ResponseRecorder {
		hr := httptest.NewRequest(method, path, body)
		if ct != "" {
			hr.Header.Set("Content-Type", ct)
		}
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, hr)
		return rr
	}

	rr := do(http.MethodGet, "/api/data/export?format=csv", strings.NewReader(""), "")
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(rr.Header().Get("Content-Disposition"), "lexis-words-2026-11-02.csv") ||
		!strings.Contains(rr.Body.String(), "apple,яблоко,I eat an [apple] <> Red [apple],food nouns") {
		t.Errorf("export csv: %d %v\n%s", rr.Code, rr.Header(), rr.Body)
	}

	rr = do(http.MethodGet, "/api/data/export", strings.NewReader(""), "")
	if rr.Code != 200 || !strings.Contains(rr.Header().Get("Content-Disposition"), "lexis-backup-2026-11-02.json") {
		t.Fatalf("export json: %d %v", rr.Code, rr.Header())
	}
	backup := rr.Body.String()

	if rr = do(http.MethodPost, "/api/data/reset", strings.NewReader(`{"confirm":true}`), "application/json"); rr.Code != 204 {
		t.Fatalf("reset: %d %s", rr.Code, rr.Body)
	}

	body, ct := multipartBody(t, "file", backup)
	rr = do(http.MethodPost, "/api/data/import", strings.NewReader(body.String()), ct)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"kind":"backup"`) {
		t.Errorf("import backup: %d %s", rr.Code, rr.Body)
	}
	if n := fx.Int(t, `SELECT count(*) FROM words`); n != 3 {
		t.Errorf("words after HTTP restore = %d", n)
	}
}
