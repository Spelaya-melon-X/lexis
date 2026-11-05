package importexport

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"lexis/internal/groups/tree"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
	"lexis/internal/sport"
	tasksmod "lexis/internal/tasks" // tasks — уже имя поля секции; алиас, чтобы не путать
)

// BackupVersion — версия формата. Меняем при несовместимых изменениях; старые версии
// при необходимости конвертируем при восстановлении.
//
// v2 (§17.10) = v1 + необязательные секции journal и habits. Файлы v1 восстанавливаются
// как есть: новых секций в них нет, и эти разделы при восстановлении не трогаются.
// v3 (§18.1) = v2 + groups[].parentId (дерево групп) + необязательная секция game.
// В v1/v2 их нет — все группы корневые, «Игра» при восстановлении не трогается.
const BackupVersion = 3

// Backup — полный снимок данных (§4: слова, группы, уровни, история).
// id внутри файла — только для ссылок между сущностями: при восстановлении
// записи получают новые id, ссылки переназначаются.
type Backup struct {
	Version      int                        `json:"version"`
	ExportedAt   time.Time                  `json:"exportedAt"`
	Settings     map[string]json.RawMessage `json:"settings"`
	Groups       []BackupGroup              `json:"groups"`
	Words        []BackupWord               `json:"words"`
	Sessions     []BackupSession            `json:"sessions"`
	Achievements []BackupAchievement        `json:"achievements"`
	// nil — секции нет в файле (бэкап v1): эти данные восстановление не трогает.
	Journal *BackupJournal `json:"journal,omitempty"`
	Habits  *BackupHabits  `json:"habits,omitempty"`
	Game    *BackupGame    `json:"game,omitempty"`  // v3
	Sport   *BackupSport   `json:"sport,omitempty"` // v3, с фазы C1
}

// BackupSport — раздел «Спорт» (§18.4). id упражнений — только для ссылок внутри файла.
type BackupSport struct {
	Exercises []BackupExercise `json:"exercises"`
	Workouts  []BackupWorkout  `json:"workouts"`
	Meals     []BackupMeal     `json:"meals"` // C2; в бэкапах C1 ключа нет — пусто
}

// BackupMeal — пункт еды; порядок внутри дня — порядок в файле.
type BackupMeal struct {
	Day       string    `json:"day"`
	Text      string    `json:"text"`
	Kcal      *int32    `json:"kcal"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupExercise struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupWorkout struct {
	Day         string               `json:"day"`
	Title       string               `json:"title"`
	Notes       string               `json:"notes"`
	DurationMin *int32               `json:"durationMin"`
	Kcal        *int32               `json:"kcal"`
	Exercises   []BackupWorkoutEntry `json:"exercises"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

// BackupWorkoutEntry — упражнение тренировки: подходы × повторы × вес (§18.5.7).
type BackupWorkoutEntry struct {
	ExerciseID int64    `json:"exerciseId"`
	Sets       int16    `json:"sets"`
	Reps       int32    `json:"reps"`
	Weight     *float64 `json:"weight"`
}

// BackupGame — раздел «Игра в жизнь» (§18.3). id — только для ссылок внутри файла.
type BackupGame struct {
	Types   []BackupTaskType   `json:"types"`
	Columns []BackupTaskColumn `json:"columns"`
	Tasks   []BackupTask       `json:"tasks"`
	Plans   []BackupPlanItem   `json:"plans"` // B3; в бэкапах B1/B2 ключа нет — пусто
	// B4 — фокус и награды; в бэкапах до B4 ключей нет — пусто.
	Ritual   []string             `json:"ritual"`
	Sessions []BackupFocusSession `json:"sessions"`
	Rewards  []BackupReward       `json:"rewards"`
	Ledger   []BackupPoints       `json:"ledger"`
}

type BackupFocusSession struct {
	ID            int64               `json:"id"`
	TaskID        *int64              `json:"taskId"`
	TaskTitle     string              `json:"taskTitle"`
	Day           string              `json:"day"`
	PlannedBlocks int16               `json:"plannedBlocks"`
	StartedAt     time.Time           `json:"startedAt"`
	WorkStartedAt *time.Time          `json:"workStartedAt"`
	FinishedAt    *time.Time          `json:"finishedAt"`
	Outcome       *string             `json:"outcome"`
	Breaks        []BackupFocusBreak  `json:"breaks"`
	Distractions  []BackupDistraction `json:"distractions"`
}

type BackupFocusBreak struct {
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

type BackupDistraction struct {
	At      time.Time `json:"at"`
	Trigger string    `json:"trigger"`
	GaveIn  bool      `json:"gaveIn"`
}

type BackupReward struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Price      int32      `json:"price"`
	Position   int32      `json:"position"`
	ArchivedAt *time.Time `json:"archivedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// BackupPoints — запись журнала очков (в порядке ленты).
type BackupPoints struct {
	At        time.Time `json:"at"`
	Delta     int32     `json:"delta"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	SessionID *int64    `json:"sessionId"`
	RewardID  *int64    `json:"rewardId"`
}

// BackupPlanItem — пункт плана дня. Плоским списком, порядок внутри дня — position.
type BackupPlanItem struct {
	Day      string `json:"day"`
	TaskID   int64  `json:"taskId"`
	Position int16  `json:"position"`
	Blocks   int16  `json:"blocks"`
	Start    *int16 `json:"start"` // минуты от полуночи; в бэкапах первой версии B3 нет — см. fillPlanStarts
}

// fillPlanStarts — время начала для пунктов из бэкапа без него: как в миграции 0009,
// подряд от 9:00 в порядке position внутри дня. Пункты со своим временем не трогает.
func fillPlanStarts(items []BackupPlanItem) []int16 {
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(items[a].Day, items[b].Day), cmp.Compare(items[a].Position, items[b].Position))
	})
	out := make([]int16, len(items))
	next := map[string]int16{}
	for _, i := range order {
		it := items[i]
		if it.Start != nil {
			out[i] = *it.Start
			continue
		}
		if _, ok := next[it.Day]; !ok {
			next[it.Day] = 540
		}
		out[i] = min(next[it.Day], 1410)
		next[it.Day] += it.Blocks * 30
	}
	return out
}

type BackupTaskType struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	Position  int32     `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupTaskColumn struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Position  int32     `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupTask struct {
	ID         int64      `json:"id"`
	ParentID   *int64     `json:"parentId"` // шаг дела (B2); null — корень
	Title      string     `json:"title"`
	Blocks     int16      `json:"blocks"`
	Importance int16      `json:"importance"`
	Deadline   *string    `json:"deadline"` // YYYY-MM-DD
	TypeID     *int64     `json:"typeId"`
	ColumnID   int64      `json:"columnId"`
	Position   int32      `json:"position"`
	Note       string     `json:"note"`
	DoneAt     *time.Time `json:"doneAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// BackupHabits — раздел «Привычки». id привычек — только для ссылок внутри файла.
type BackupHabits struct {
	Habits   []BackupHabit   `json:"habits"`
	Checkins []BackupCheckin `json:"checkins"`
	Goals    []BackupGoal    `json:"goals"`
	Notes    []BackupNote    `json:"notes"` // в порядке ленты (по возрастанию id)
}

type BackupHabit struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	Color        *string    `json:"color"`
	TrackTrigger bool       `json:"trackTrigger"`
	TrackRating  bool       `json:"trackRating"`
	TrackAmount  bool       `json:"trackAmount"`
	Unit         string     `json:"unit"`
	Position     int32      `json:"position"`
	ArchivedAt   *time.Time `json:"archivedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type BackupCheckin struct {
	HabitID   int64     `json:"habitId"`
	Day       string    `json:"day"`
	Trigger   *string   `json:"trigger"`
	Rating    *int16    `json:"rating"`
	Amount    *float64  `json:"amount"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BackupGoal struct {
	HabitID   *int64    `json:"habitId"`
	Title     string    `json:"title"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	CreatedAt time.Time `json:"createdAt"`
}

type BackupNote struct {
	HabitID   *int64    `json:"habitId"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// BackupJournal — раздел «День».
type BackupJournal struct {
	Days  []BackupDay  `json:"days"`
	Weeks []BackupWeek `json:"weeks"` // нет в бэкапах до фазы 4 — тогда пусто
}

type BackupWeek struct {
	WeekStart string        `json:"weekStart"` // понедельник YYYY-MM-DD
	Blocks    []BackupBlock `json:"blocks"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type BackupBlock struct {
	Topic      string `json:"topic"`
	Conclusion string `json:"conclusion"`
}

type BackupDay struct {
	Day       string           `json:"day"` // YYYY-MM-DD
	Rating    *int16           `json:"rating"`
	Story     string           `json:"story"`
	Wins      []string         `json:"wins"`
	Dopamine  []BackupDopamine `json:"dopamine"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

type BackupDopamine struct {
	Slot      int16     `json:"slot"`
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type BackupGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	ParentID  *int64    `json:"parentId"` // id в файле; null — корень (v3)
	CreatedAt time.Time `json:"createdAt"`
}

type BackupWord struct {
	ID             int64      `json:"id"`
	Term           string     `json:"term"`
	Forms          []string   `json:"forms"`
	Translation    string     `json:"translation"`
	Level          int16      `json:"level"`
	Streak         int16      `json:"streak"`
	NextReviewAt   time.Time  `json:"nextReviewAt"`
	LastReviewedAt *time.Time `json:"lastReviewedAt"`
	LastGoodAt     *time.Time `json:"lastGoodAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	Examples       []string   `json:"examples"` // разметка "I [go] home", в порядке position
	GroupIDs       []int64    `json:"groupIds"`
}

type BackupSession struct {
	ID          int64          `json:"id"`
	GroupID     *int64         `json:"groupId"`
	Strategy    string         `json:"strategy"`
	Free        bool           `json:"free"`
	StartedAt   time.Time      `json:"startedAt"`
	FinishedAt  *time.Time     `json:"finishedAt"`
	Total       int32          `json:"total"`
	Good        int32          `json:"good"`
	Hard        int32          `json:"hard"`
	Again       int32          `json:"again"`
	DurationSec int32          `json:"durationSec"`
	Reviews     []BackupReview `json:"reviews"`
}

type BackupReview struct {
	WordID         int64     `json:"wordId"`
	Answer         string    `json:"answer"`
	FirstInSession bool      `json:"firstInSession"`
	LevelBefore    int16     `json:"levelBefore"`
	LevelAfter     int16     `json:"levelAfter"`
	AnsweredAt     time.Time `json:"answeredAt"`
}

type BackupAchievement struct {
	Code       string    `json:"code"`
	UnlockedAt time.Time `json:"unlockedAt"`
}

// DecodeBackup разбирает файл бэкапа строго: неизвестное поле — скорее всего, не наш файл
// или другая версия, и восстанавливать «как получится» поверх всех данных нельзя.
func DecodeBackup(data []byte) (Backup, error) {
	var b Backup
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, utf8BOM)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Backup{}, httpx.BadRequest("invalid_backup", "invalid backup: "+err.Error())
	}
	return b, validateBackup(b)
}

// validateBackup проверяет версию и ссылки до транзакции: висячая ссылка в файле —
// ошибка 400, а не 500 от внешнего ключа посреди восстановления.
// Значения полей (уровни 1..5, непустые строки, стратегии) проверяют CHECK-и схемы.
func validateBackup(b Backup) error {
	if b.Version < 1 || b.Version > BackupVersion {
		return httpx.BadRequest("unsupported_backup_version", fmt.Sprintf("backup version %d is not supported", b.Version))
	}
	bad := func(format string, args ...any) error {
		return httpx.BadRequest("invalid_backup", fmt.Sprintf(format, args...))
	}
	if b.Version == 1 && (b.Journal != nil || b.Habits != nil) {
		return bad("version 1 backup cannot contain journal or habits")
	}

	groups := make(map[int64]bool, len(b.Groups))
	nodes := make([]tree.Node, 0, len(b.Groups))
	for _, g := range b.Groups {
		if groups[g.ID] {
			return bad("duplicate group id %d", g.ID)
		}
		groups[g.ID] = true
		if g.ParentID != nil {
			if b.Version < 3 {
				return bad("version %d backup cannot contain nested groups", b.Version)
			}
			// 0 в tree значит «корень» — такой id в файле был бы висячей ссылкой, а не корнем.
			if *g.ParentID == 0 {
				return bad("group %d refers to unknown parent 0", g.ID)
			}
			nodes = append(nodes, tree.Node{ID: g.ID, Name: g.Name, Parent: *g.ParentID})
		} else {
			nodes = append(nodes, tree.Node{ID: g.ID, Name: g.Name})
		}
	}
	if err := tree.New(nodes).Validate(); err != nil {
		return bad("group tree: %v", err)
	}
	wordIDs := make(map[int64]bool, len(b.Words))
	for _, w := range b.Words {
		if wordIDs[w.ID] {
			return bad("duplicate word id %d", w.ID)
		}
		wordIDs[w.ID] = true
		for _, gid := range w.GroupIDs {
			if !groups[gid] {
				return bad("word %d refers to unknown group %d", w.ID, gid)
			}
		}
	}
	sessions := make(map[int64]bool, len(b.Sessions))
	for _, s := range b.Sessions {
		if sessions[s.ID] {
			return bad("duplicate session id %d", s.ID)
		}
		sessions[s.ID] = true
		if s.GroupID != nil && !groups[*s.GroupID] {
			return bad("session %d refers to unknown group %d", s.ID, *s.GroupID)
		}
		for _, r := range s.Reviews {
			if !wordIDs[r.WordID] {
				return bad("review in session %d refers to unknown word %d", s.ID, r.WordID)
			}
		}
	}
	if b.Journal != nil {
		days := make(map[string]bool, len(b.Journal.Days))
		for _, d := range b.Journal.Days {
			// Дата — ключ (PK), а не просто значение: кривую ловим здесь, а не ошибкой COPY.
			if _, err := clock.ParseDate(d.Day); err != nil {
				return bad("invalid journal day %q", d.Day)
			}
			if days[d.Day] {
				return bad("duplicate journal day %s", d.Day)
			}
			days[d.Day] = true
		}
		weeks := make(map[string]bool, len(b.Journal.Weeks))
		for _, w := range b.Journal.Weeks {
			// Понедельник проверит и CHECK схемы, но кривая дата здесь — понятная 400, а не ошибка COPY.
			if _, err := clock.ParseDate(w.WeekStart); err != nil {
				return bad("invalid journal week %q", w.WeekStart)
			}
			if weeks[w.WeekStart] {
				return bad("duplicate journal week %s", w.WeekStart)
			}
			weeks[w.WeekStart] = true
		}
	}
	if b.Game != nil {
		if b.Version < 3 {
			return bad("version %d backup cannot contain game", b.Version)
		}
		if err := validateGame(*b.Game, bad); err != nil {
			return err
		}
	}
	if b.Sport != nil {
		if b.Version < 3 {
			return bad("version %d backup cannot contain sport", b.Version)
		}
		if err := validateSport(*b.Sport, bad); err != nil {
			return err
		}
	}
	if b.Habits != nil {
		return validateHabits(*b.Habits, bad)
	}
	return nil
}

// validateSport — ссылки, даты и строки упражнений (теми же правилами, что и API).
func validateSport(sp BackupSport, bad func(string, ...any) error) error {
	names := make(map[int64]string, len(sp.Exercises))
	for _, e := range sp.Exercises {
		if _, dup := names[e.ID]; dup {
			return bad("duplicate exercise id %d", e.ID)
		}
		names[e.ID] = e.Name
	}
	for i, w := range sp.Workouts {
		if _, err := clock.ParseDate(w.Day); err != nil {
			return bad("workout %d: invalid day %q", i, w.Day)
		}
		for _, x := range w.Exercises {
			name, ok := names[x.ExerciseID]
			if !ok {
				return bad("workout %d refers to unknown exercise %d", i, x.ExerciseID)
			}
			if _, err := (sport.EntryInput{Name: name, Sets: int(x.Sets), Reps: int(x.Reps), Weight: x.Weight}).Normalize(); err != nil {
				return bad("workout %d: invalid entry of exercise %d", i, x.ExerciseID)
			}
		}
	}
	for i, m := range sp.Meals {
		if _, err := clock.ParseDate(m.Day); err != nil {
			return bad("meal %d: invalid day %q", i, m.Day)
		}
	}
	return nil
}

// validateGame — ссылки и даты секции game (значения полей проверяют CHECK-и схемы).
func validateGame(g BackupGame, bad func(string, ...any) error) error {
	types := make(map[int64]bool, len(g.Types))
	for _, t := range g.Types {
		if types[t.ID] {
			return bad("duplicate task type id %d", t.ID)
		}
		types[t.ID] = true
	}
	columns := make(map[int64]bool, len(g.Columns))
	for _, c := range g.Columns {
		if columns[c.ID] {
			return bad("duplicate column id %d", c.ID)
		}
		columns[c.ID] = true
	}
	tasks := make(map[int64]bool, len(g.Tasks))
	nodes := make([]tasksmod.Node, 0, len(g.Tasks))
	for _, t := range g.Tasks {
		if tasks[t.ID] {
			return bad("duplicate task id %d", t.ID)
		}
		tasks[t.ID] = true
		if !columns[t.ColumnID] {
			return bad("task %d refers to unknown column %d", t.ID, t.ColumnID)
		}
		if t.TypeID != nil && !types[*t.TypeID] {
			return bad("task %d refers to unknown type %d", t.ID, *t.TypeID)
		}
		if t.Deadline != nil {
			if _, err := clock.ParseDate(*t.Deadline); err != nil {
				return bad("task %d: invalid deadline %q", t.ID, *t.Deadline)
			}
		}
		n := tasksmod.Node{ID: t.ID}
		if t.ParentID != nil {
			if *t.ParentID == 0 {
				return bad("task %d refers to unknown parent 0", t.ID) // 0 в Rollup — «корень»
			}
			n.Parent = *t.ParentID
		}
		nodes = append(nodes, n)
	}
	for _, t := range g.Tasks {
		if t.ParentID != nil && !tasks[*t.ParentID] {
			return bad("task %d refers to unknown parent %d", t.ID, *t.ParentID)
		}
	}
	planned := make(map[string]bool, len(g.Plans))
	for _, p := range g.Plans {
		if _, err := clock.ParseDate(p.Day); err != nil {
			return bad("invalid plan day %q", p.Day)
		}
		if !tasks[p.TaskID] {
			return bad("plan item refers to unknown task %d", p.TaskID)
		}
		key := fmt.Sprintf("%s/%d", p.Day, p.TaskID)
		if planned[key] {
			return bad("duplicate plan item %s", key)
		}
		planned[key] = true
	}
	if err := validateFocus(g, tasks, bad); err != nil {
		return err
	}
	// Rollup не доходит до узлов на цикле (у них нет корня) — по размеру результата его и видно.
	agg := tasksmod.Rollup(nodes)
	if len(agg) != len(nodes) {
		return bad("task tree has a cycle")
	}
	for id, a := range agg {
		if a.Depth > tasksmod.MaxDepth {
			return bad("task %d is nested deeper than %d levels", id, tasksmod.MaxDepth)
		}
	}
	return nil
}

// validateHabits — ссылки и даты секции habits (значения полей проверяют CHECK-и схемы).
func validateHabits(h BackupHabits, bad func(string, ...any) error) error {
	ids := make(map[int64]bool, len(h.Habits))
	for _, x := range h.Habits {
		if ids[x.ID] {
			return bad("duplicate habit id %d", x.ID)
		}
		ids[x.ID] = true
	}
	ref := func(what string, id *int64) error {
		if id != nil && !ids[*id] {
			return bad("%s refers to unknown habit %d", what, *id)
		}
		return nil
	}
	date := func(what, s string) error {
		if _, err := clock.ParseDate(s); err != nil {
			return bad("invalid %s date %q", what, s)
		}
		return nil
	}
	seen := make(map[string]bool, len(h.Checkins))
	for _, c := range h.Checkins {
		if err := ref("checkin", &c.HabitID); err != nil {
			return err
		}
		if err := date("checkin", c.Day); err != nil {
			return err
		}
		key := fmt.Sprintf("%d/%s", c.HabitID, c.Day)
		if seen[key] {
			return bad("duplicate checkin %s", key)
		}
		seen[key] = true
	}
	for _, g := range h.Goals {
		if err := ref("goal", g.HabitID); err != nil {
			return err
		}
		if err := date("goal", g.From); err != nil {
			return err
		}
		if err := date("goal", g.To); err != nil {
			return err
		}
	}
	for _, n := range h.Notes {
		if err := ref("note", n.HabitID); err != nil {
			return err
		}
	}
	return nil
}

// validateFocus — ссылки секции game, добавленные в B4: сессии → дела, журнал → сессии/награды.
// Значения (длины, CHECK исхода, одна незавершённая сессия) проверяют ограничения схемы.
func validateFocus(g BackupGame, tasks map[int64]bool, bad func(string, ...any) error) error {
	sessions := make(map[int64]bool, len(g.Sessions))
	for _, x := range g.Sessions {
		if sessions[x.ID] {
			return bad("duplicate focus session id %d", x.ID)
		}
		sessions[x.ID] = true
		if x.TaskID != nil && !tasks[*x.TaskID] {
			return bad("focus session %d refers to unknown task %d", x.ID, *x.TaskID)
		}
		if _, err := clock.ParseDate(x.Day); err != nil {
			return bad("focus session %d: invalid day %q", x.ID, x.Day)
		}
	}
	rewards := make(map[int64]bool, len(g.Rewards))
	for _, r := range g.Rewards {
		if rewards[r.ID] {
			return bad("duplicate reward id %d", r.ID)
		}
		rewards[r.ID] = true
	}
	credited := make(map[int64]bool) // начисление за сессию — одно (UNIQUE в схеме)
	for _, p := range g.Ledger {
		if p.SessionID != nil {
			if !sessions[*p.SessionID] {
				return bad("points refer to unknown focus session %d", *p.SessionID)
			}
			if credited[*p.SessionID] {
				return bad("focus session %d is credited twice", *p.SessionID)
			}
			credited[*p.SessionID] = true
		}
		if p.RewardID != nil && !rewards[*p.RewardID] {
			return bad("points refer to unknown reward %d", *p.RewardID)
		}
	}
	return nil
}
