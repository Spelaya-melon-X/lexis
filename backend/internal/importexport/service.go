package importexport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lexis/internal/importexport/store"
	"lexis/internal/platform/clock"
	"lexis/internal/platform/db"
	"lexis/internal/platform/httpx"
	"lexis/internal/words"
)

// WordStore — то, что импорту/экспорту нужно от модуля words (реализует *words.Service).
// Импорт списка слов идёт через те же SaveCards, что и ввод: те же правила §4 и слияние дублей.
type WordStore interface {
	List(ctx context.Context, f words.ListFilter) ([]words.Word, error)
	SaveCards(ctx context.Context, cards []words.Card) (created, merged int, err error)
}

type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	words WordStore
}

func NewService(pool *pgxpool.Pool, clk clock.Clock, ws WordStore) *Service {
	return &Service{pool: pool, clock: clk, words: ws}
}

// ImportResult — ответ POST /api/data/import. Kind говорит, что произошло:
// "words" — слова добавлены/слиты (как bulk), "backup" — данные заменены бэкапом.
type ImportResult struct {
	Kind     string            `json:"kind"`
	Created  int               `json:"created,omitempty"`
	Merged   int               `json:"merged,omitempty"`
	Errors   []words.LineError `json:"errors,omitempty"`
	Restored *RestoreCounts    `json:"restored,omitempty"`
}

type RestoreCounts struct {
	Groups       int `json:"groups"`
	Words        int `json:"words"`
	Sessions     int `json:"sessions"`
	Reviews      int `json:"reviews"`
	Achievements int `json:"achievements"`
	Days         int `json:"days"`   // 0 и для бэкапа без секции journal (v1)
	Habits       int `json:"habits"` // то же для секции habits
}

// Import определяет формат по содержимому (см. DetectFormat) и импортирует.
func (s *Service) Import(ctx context.Context, data []byte) (ImportResult, error) {
	var rows []words.Row
	var err error
	switch DetectFormat(data) {
	case FormatBackup:
		b, err := DecodeBackup(data)
		if err != nil {
			return ImportResult{}, err
		}
		counts, err := s.Restore(ctx, b)
		if err != nil {
			return ImportResult{}, err
		}
		return ImportResult{Kind: "backup", Restored: &counts}, nil
	case FormatJSONList:
		rows, err = ParseWordList(data)
	default:
		rows, err = ParseCSV(data)
	}
	if err != nil {
		return ImportResult{}, err
	}

	parsed := words.ParseRows(rows)
	created, merged, err := s.words.SaveCards(ctx, parsed.Cards)
	if err != nil {
		return ImportResult{}, fmt.Errorf("import: %w", err)
	}
	return ImportResult{Kind: "words", Created: created, Merged: merged, Errors: parsed.Errors}, nil
}

// ExportCSV пишет все слова в формате импорта.
func (s *Service) ExportCSV(ctx context.Context, w io.Writer) error {
	ws, err := s.words.List(ctx, words.ListFilter{})
	if err != nil {
		return fmt.Errorf("export csv: %w", err)
	}
	if err := WriteCSV(w, ws); err != nil {
		return fmt.Errorf("export csv: write: %w", err)
	}
	return nil
}

// ExportBackup собирает полный снимок. Читаем в одной REPEATABLE READ транзакции:
// иначе ответ, записанный между запросами, мог бы сослаться на слово, которого нет в снимке.
func (s *Service) ExportBackup(ctx context.Context) (Backup, error) {
	b := Backup{Version: BackupVersion, ExportedAt: s.clock.Now(), Settings: map[string]json.RawMessage{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		q := store.New(tx)

		groups, err := q.ListAllGroups(ctx)
		if err != nil {
			return fmt.Errorf("groups: %w", err)
		}
		b.Groups = make([]BackupGroup, len(groups))
		for i, g := range groups {
			b.Groups[i] = BackupGroup{ID: g.ID, Name: g.Name, Color: g.Color, CreatedAt: g.CreatedAt}
		}

		ws, err := q.ListAllWords(ctx)
		if err != nil {
			return fmt.Errorf("words: %w", err)
		}
		b.Words = make([]BackupWord, len(ws))
		byWord := make(map[int64]*BackupWord, len(ws))
		for i, w := range ws {
			b.Words[i] = BackupWord{
				ID: w.ID, Term: w.Term, Forms: w.Forms, Translation: w.Translation, Level: w.Level, Streak: w.Streak,
				NextReviewAt: w.NextReviewAt, LastReviewedAt: w.LastReviewedAt, LastGoodAt: w.LastGoodAt, CreatedAt: w.CreatedAt,
				Examples: []string{}, GroupIDs: []int64{},
			}
			byWord[w.ID] = &b.Words[i]
		}
		exs, err := q.ListAllExamples(ctx)
		if err != nil {
			return fmt.Errorf("examples: %w", err)
		}
		for _, e := range exs {
			byWord[e.WordID].Examples = append(byWord[e.WordID].Examples, e.Sentence)
		}
		wgs, err := q.ListAllWordGroups(ctx)
		if err != nil {
			return fmt.Errorf("word groups: %w", err)
		}
		for _, wg := range wgs {
			byWord[wg.WordID].GroupIDs = append(byWord[wg.WordID].GroupIDs, wg.GroupID)
		}

		sessions, err := q.ListAllSessions(ctx)
		if err != nil {
			return fmt.Errorf("sessions: %w", err)
		}
		b.Sessions = make([]BackupSession, len(sessions))
		bySession := make(map[int64]*BackupSession, len(sessions))
		for i, ss := range sessions {
			b.Sessions[i] = BackupSession{
				ID: ss.ID, GroupID: ss.GroupID, Strategy: ss.Strategy, Free: ss.Free, StartedAt: ss.StartedAt,
				FinishedAt: ss.FinishedAt, Total: ss.Total, Good: ss.Good, Hard: ss.Hard, Again: ss.Again,
				DurationSec: ss.DurationSec, Reviews: []BackupReview{},
			}
			bySession[ss.ID] = &b.Sessions[i]
		}
		reviews, err := q.ListAllReviews(ctx)
		if err != nil {
			return fmt.Errorf("reviews: %w", err)
		}
		for _, r := range reviews {
			bs := bySession[r.SessionID]
			bs.Reviews = append(bs.Reviews, BackupReview{
				WordID: r.WordID, Answer: r.Answer, FirstInSession: r.FirstInSession,
				LevelBefore: r.LevelBefore, LevelAfter: r.LevelAfter, AnsweredAt: r.AnsweredAt,
			})
		}

		achs, err := q.ListAllAchievements(ctx)
		if err != nil {
			return fmt.Errorf("achievements: %w", err)
		}
		b.Achievements = make([]BackupAchievement, len(achs))
		for i, a := range achs {
			b.Achievements[i] = BackupAchievement{Code: a.Code, UnlockedAt: a.UnlockedAt}
		}

		settings, err := q.ListAllSettings(ctx)
		if err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		for _, st := range settings {
			b.Settings[st.Key] = json.RawMessage(st.Value)
		}

		if b.Journal, err = exportJournal(ctx, q); err != nil {
			return err
		}
		b.Habits, err = exportHabits(ctx, q)
		return err
	})
	if err != nil {
		return Backup{}, fmt.Errorf("export backup: %w", err)
	}
	return b, nil
}

// Restore заменяет все данные (включая settings) содержимым бэкапа — одной транзакцией:
// упало посередине — в базе остаётся то, что было. Бэкап должен пройти validateBackup.
// Разделы v2 (journal) заменяются, только если их секция есть в файле (§17.10).
func (s *Service) Restore(ctx context.Context, b Backup) (RestoreCounts, error) {
	if err := validateBackup(b); err != nil {
		return RestoreCounts{}, err
	}
	var c RestoreCounts
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.TruncateData(ctx); err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
		if err := q.DeleteSettings(ctx); err != nil {
			return fmt.Errorf("delete settings: %w", err)
		}

		groupID := make(map[int64]int64, len(b.Groups)) // id в файле → новый id
		for _, g := range b.Groups {
			id, err := q.RestoreGroup(ctx, store.RestoreGroupParams{Name: g.Name, Color: g.Color, CreatedAt: g.CreatedAt})
			if err != nil {
				return fmt.Errorf("group %q: %w", g.Name, err)
			}
			groupID[g.ID] = id
		}

		wordID := make(map[int64]int64, len(b.Words))
		var exs []store.RestoreExamplesParams
		var wgs []store.RestoreWordGroupsParams
		for _, w := range b.Words {
			forms := w.Forms
			if forms == nil {
				forms = []string{} // NOT NULL — pgx пишет nil как NULL
			}
			id, err := q.RestoreWord(ctx, store.RestoreWordParams{
				Term: w.Term, Forms: forms, Translation: w.Translation, Level: w.Level, Streak: w.Streak,
				NextReviewAt: w.NextReviewAt, LastReviewedAt: w.LastReviewedAt, LastGoodAt: w.LastGoodAt, CreatedAt: w.CreatedAt,
			})
			if err != nil {
				return fmt.Errorf("word %q: %w", w.Term, err)
			}
			wordID[w.ID] = id
			for i, e := range w.Examples {
				exs = append(exs, store.RestoreExamplesParams{WordID: id, Position: int16(i), Sentence: e})
			}
			for _, gid := range w.GroupIDs {
				wgs = append(wgs, store.RestoreWordGroupsParams{WordID: id, GroupID: groupID[gid]})
			}
		}
		// COPY, а не INSERT по строке: ответов за год — десятки тысяч.
		if _, err := q.RestoreExamples(ctx, exs); err != nil {
			return fmt.Errorf("examples: %w", err)
		}
		if _, err := q.RestoreWordGroups(ctx, wgs); err != nil {
			return fmt.Errorf("word groups: %w", err)
		}

		var reviews []store.RestoreReviewsParams
		for _, ss := range b.Sessions {
			var gid *int64
			if ss.GroupID != nil {
				id := groupID[*ss.GroupID]
				gid = &id
			}
			sid, err := q.RestoreSession(ctx, store.RestoreSessionParams{
				GroupID: gid, Strategy: ss.Strategy, Free: ss.Free, StartedAt: ss.StartedAt, FinishedAt: ss.FinishedAt,
				Total: ss.Total, Good: ss.Good, Hard: ss.Hard, Again: ss.Again, DurationSec: ss.DurationSec,
			})
			if err != nil {
				return fmt.Errorf("session %d: %w", ss.ID, err)
			}
			for _, r := range ss.Reviews {
				reviews = append(reviews, store.RestoreReviewsParams{
					SessionID: sid, WordID: wordID[r.WordID], Answer: r.Answer, FirstInSession: r.FirstInSession,
					LevelBefore: r.LevelBefore, LevelAfter: r.LevelAfter, AnsweredAt: r.AnsweredAt,
				})
			}
		}
		if _, err := q.RestoreReviews(ctx, reviews); err != nil {
			return fmt.Errorf("reviews: %w", err)
		}

		for _, a := range b.Achievements {
			if err := q.RestoreAchievement(ctx, store.RestoreAchievementParams{Code: a.Code, UnlockedAt: a.UnlockedAt}); err != nil {
				return fmt.Errorf("achievement %s: %w", a.Code, err)
			}
		}
		for k, v := range b.Settings {
			if err := q.RestoreSetting(ctx, store.RestoreSettingParams{Key: k, Value: v}); err != nil {
				return fmt.Errorf("setting %s: %w", k, err)
			}
		}

		c = RestoreCounts{Groups: len(b.Groups), Words: len(b.Words), Sessions: len(b.Sessions), Reviews: len(reviews), Achievements: len(b.Achievements)}
		if b.Journal != nil {
			if err := restoreJournal(ctx, q, *b.Journal); err != nil {
				return fmt.Errorf("journal: %w", err)
			}
			c.Days = len(b.Journal.Days)
		}
		if b.Habits != nil {
			if err := restoreHabits(ctx, q, *b.Habits); err != nil {
				return fmt.Errorf("habits: %w", err)
			}
			c.Habits = len(b.Habits.Habits)
		}
		return nil
	})
	if err != nil {
		// Нарушен CHECK/UNIQUE (уровень 7, дубль слова в файле) — проблема файла, а не сервера.
		if db.IsIntegrityViolation(err) {
			return RestoreCounts{}, &httpx.Error{Status: http.StatusBadRequest, Code: "invalid_backup", Message: "backup violates data constraints", Err: err}
		}
		return RestoreCounts{}, fmt.Errorf("restore: %w", err)
	}
	return c, nil
}

func exportJournal(ctx context.Context, q *store.Queries) (*BackupJournal, error) {
	days, err := q.ListAllDays(ctx)
	if err != nil {
		return nil, fmt.Errorf("days: %w", err)
	}
	j := &BackupJournal{Days: make([]BackupDay, len(days))}
	// Ключ — строка, а не time.Time: == у time.Time сравнивает ещё и *Location.
	byDay := make(map[string]*BackupDay, len(days))
	for i, d := range days {
		j.Days[i] = BackupDay{
			Day: clock.FormatDate(d.Day), Rating: d.Rating, Story: d.Story, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
			Wins: []string{}, Dopamine: []BackupDopamine{},
		}
		byDay[j.Days[i].Day] = &j.Days[i]
	}
	wins, err := q.ListAllWins(ctx)
	if err != nil {
		return nil, fmt.Errorf("wins: %w", err)
	}
	for _, w := range wins {
		d := byDay[clock.FormatDate(w.Day)]
		d.Wins = append(d.Wins, w.Text) // FK гарантирует, что день есть
	}
	slots, err := q.ListAllDopamine(ctx)
	if err != nil {
		return nil, fmt.Errorf("dopamine: %w", err)
	}
	for _, sl := range slots {
		d := byDay[clock.FormatDate(sl.Day)]
		d.Dopamine = append(d.Dopamine, BackupDopamine{Slot: sl.Slot, Text: sl.Text, UpdatedAt: sl.UpdatedAt})
	}
	ws, err := q.ListAllWeeks(ctx)
	if err != nil {
		return nil, fmt.Errorf("weeks: %w", err)
	}
	j.Weeks = make([]BackupWeek, len(ws))
	byWeek := make(map[string]*BackupWeek, len(ws))
	for i, w := range ws {
		j.Weeks[i] = BackupWeek{WeekStart: clock.FormatDate(w.WeekStart), Blocks: []BackupBlock{}, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
		byWeek[j.Weeks[i].WeekStart] = &j.Weeks[i]
	}
	bs, err := q.ListAllWeekBlocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("week blocks: %w", err)
	}
	for _, b := range bs {
		w := byWeek[clock.FormatDate(b.WeekStart)] // FK гарантирует, что неделя есть
		w.Blocks = append(w.Blocks, BackupBlock{Topic: b.Topic, Conclusion: b.Conclusion})
	}
	return j, nil
}

func restoreJournal(ctx context.Context, q *store.Queries, j BackupJournal) error {
	if err := q.TruncateJournal(ctx); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	days := make([]store.RestoreDaysParams, len(j.Days))
	var wins []store.RestoreWinsParams
	var slots []store.RestoreDopamineParams
	for i, d := range j.Days {
		day, _ := clock.ParseDate(d.Day) // проверено в validateBackup
		days[i] = store.RestoreDaysParams{Day: day, Rating: d.Rating, Story: d.Story, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
		for pos, w := range d.Wins {
			wins = append(wins, store.RestoreWinsParams{Day: day, Position: int16(pos), Text: w})
		}
		for _, sl := range d.Dopamine {
			slots = append(slots, store.RestoreDopamineParams{Day: day, Slot: sl.Slot, Text: sl.Text, UpdatedAt: sl.UpdatedAt})
		}
	}
	// Дни — раньше пунктов и квадратиков (FK). Значения (оценка 1..10, слот 0..6, длина) проверяют CHECK-и.
	if _, err := q.RestoreDays(ctx, days); err != nil {
		return fmt.Errorf("days: %w", err)
	}
	if _, err := q.RestoreWins(ctx, wins); err != nil {
		return fmt.Errorf("wins: %w", err)
	}
	if _, err := q.RestoreDopamine(ctx, slots); err != nil {
		return fmt.Errorf("dopamine: %w", err)
	}

	weeks := make([]store.RestoreWeeksParams, len(j.Weeks))
	var blocks []store.RestoreWeekBlocksParams
	for i, w := range j.Weeks {
		monday, _ := clock.ParseDate(w.WeekStart) // проверено в validateBackup
		weeks[i] = store.RestoreWeeksParams{WeekStart: monday, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
		for pos, b := range w.Blocks {
			blocks = append(blocks, store.RestoreWeekBlocksParams{WeekStart: monday, Position: int16(pos), Topic: b.Topic, Conclusion: b.Conclusion})
		}
	}
	if _, err := q.RestoreWeeks(ctx, weeks); err != nil {
		return fmt.Errorf("weeks: %w", err)
	}
	if _, err := q.RestoreWeekBlocks(ctx, blocks); err != nil {
		return fmt.Errorf("week blocks: %w", err)
	}
	return nil
}

func exportHabits(ctx context.Context, q *store.Queries) (*BackupHabits, error) {
	hs, err := q.ListAllHabits(ctx)
	if err != nil {
		return nil, fmt.Errorf("habits: %w", err)
	}
	out := &BackupHabits{
		Habits: make([]BackupHabit, len(hs)), Checkins: []BackupCheckin{}, Goals: []BackupGoal{}, Notes: []BackupNote{},
	}
	for i, h := range hs {
		out.Habits[i] = BackupHabit{
			ID: h.ID, Name: h.Name, Kind: h.Kind, Color: h.Color, TrackTrigger: h.TrackTrigger, TrackRating: h.TrackRating,
			TrackAmount: h.TrackAmount, Unit: h.Unit, Position: h.Position, ArchivedAt: h.ArchivedAt, CreatedAt: h.CreatedAt,
		}
	}
	cs, err := q.ListAllCheckins(ctx)
	if err != nil {
		return nil, fmt.Errorf("checkins: %w", err)
	}
	for _, c := range cs {
		out.Checkins = append(out.Checkins, BackupCheckin{
			HabitID: c.HabitID, Day: clock.FormatDate(c.Day), Trigger: c.Trigger, Rating: c.Rating, Amount: c.Amount,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		})
	}
	gs, err := q.ListAllHabitGoals(ctx)
	if err != nil {
		return nil, fmt.Errorf("goals: %w", err)
	}
	for _, g := range gs {
		out.Goals = append(out.Goals, BackupGoal{
			HabitID: g.HabitID, Title: g.Title, From: clock.FormatDate(g.StartDay), To: clock.FormatDate(g.EndDay), CreatedAt: g.CreatedAt,
		})
	}
	ns, err := q.ListAllHabitNotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("notes: %w", err)
	}
	for _, n := range ns {
		out.Notes = append(out.Notes, BackupNote{HabitID: n.HabitID, Kind: n.Kind, Body: n.Body, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt})
	}
	return out, nil
}

func restoreHabits(ctx context.Context, q *store.Queries, h BackupHabits) error {
	if err := q.TruncateHabits(ctx); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	newID := make(map[int64]int64, len(h.Habits)) // id в файле → новый id
	for _, x := range h.Habits {
		id, err := q.RestoreHabit(ctx, store.RestoreHabitParams{
			Name: x.Name, Kind: x.Kind, Color: x.Color, TrackTrigger: x.TrackTrigger, TrackRating: x.TrackRating,
			TrackAmount: x.TrackAmount, Unit: x.Unit, Position: x.Position, ArchivedAt: x.ArchivedAt, CreatedAt: x.CreatedAt,
		})
		if err != nil {
			return fmt.Errorf("habit %q: %w", x.Name, err)
		}
		newID[x.ID] = id
	}
	remap := func(id *int64) *int64 {
		if id == nil {
			return nil
		}
		v := newID[*id]
		return &v
	}

	cs := make([]store.RestoreCheckinsParams, len(h.Checkins))
	for i, c := range h.Checkins {
		day, _ := clock.ParseDate(c.Day) // проверено в validateBackup
		cs[i] = store.RestoreCheckinsParams{
			HabitID: newID[c.HabitID], Day: day, Trigger: c.Trigger, Rating: c.Rating, Amount: c.Amount,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		}
	}
	if _, err := q.RestoreCheckins(ctx, cs); err != nil {
		return fmt.Errorf("checkins: %w", err)
	}
	gs := make([]store.RestoreHabitGoalsParams, len(h.Goals))
	for i, g := range h.Goals {
		from, _ := clock.ParseDate(g.From)
		to, _ := clock.ParseDate(g.To)
		gs[i] = store.RestoreHabitGoalsParams{HabitID: remap(g.HabitID), Title: g.Title, StartDay: from, EndDay: to, CreatedAt: g.CreatedAt}
	}
	if _, err := q.RestoreHabitGoals(ctx, gs); err != nil {
		return fmt.Errorf("goals: %w", err)
	}
	ns := make([]store.RestoreHabitNotesParams, len(h.Notes))
	for i, n := range h.Notes {
		ns[i] = store.RestoreHabitNotesParams{HabitID: remap(n.HabitID), Kind: n.Kind, Body: n.Body, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
	}
	if _, err := q.RestoreHabitNotes(ctx, ns); err != nil {
		return fmt.Errorf("notes: %w", err)
	}
	return nil
}

// Что очищает Reset (§17.10). Настройки не трогает ни один вариант.
const (
	ScopeEnglish = "english" // слова, группы, история, достижения — «Очистить» в настройках английского
	ScopeAll     = "all"     // + дневник и привычки — «Очистить всё» в глобальных настройках
)

// Reset очищает данные в пределах scope одной транзакцией.
func (s *Service) Reset(ctx context.Context, scope string) error {
	if scope != ScopeEnglish && scope != ScopeAll {
		return httpx.BadRequest("invalid_scope", "scope must be english or all")
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.TruncateData(ctx); err != nil {
			return fmt.Errorf("english: %w", err)
		}
		if scope != ScopeAll {
			return nil
		}
		if err := q.TruncateJournal(ctx); err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		if err := q.TruncateHabits(ctx); err != nil {
			return fmt.Errorf("habits: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reset %s: %w", scope, err)
	}
	return nil
}
