package tasks

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestFirstFree(t *testing.T) {
	plan := []PlanItemInput{{TaskID: 1, Start: 540, Blocks: 2}, {TaskID: 2, Start: 660, Blocks: 2}}
	for _, tc := range []struct {
		name             string
		items            []PlanItemInput
		blocks, from, to int
		want             int
		found            bool
	}{
		{"fits in gap", plan, 2, 540, 1380, 600, true},
		{"too long for gap", plan, 3, 540, 1380, 720, true},
		{"respects earliest start", plan, 1, 630, 1380, 630, true},
		{"half hour in gap", plan, 1, 540, 1380, 600, true},
		{"after configured end", plan, 4, 540, 780, 720, true},
		{"empty plan", nil, 2, 540, 1380, 540, true},
		{"hits midnight", []PlanItemInput{{Start: 1200, Blocks: 8}}, 2, 1200, 1380, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, ok := firstFree(tc.items, tc.blocks, tc.from, tc.to)
			if ok != tc.found || ok && start != tc.want {
				t.Errorf("firstFree = %d, %v; want %d, %v", start, ok, tc.want, tc.found)
			}
		})
	}
}

func date(s string) *time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &d
}

// Примеры из SPEC §18.3 (день — понедельник 2026-11-02).
func TestScore(t *testing.T) {
	day := *date("2026-11-02")
	tests := []struct {
		name    string
		c       Candidate
		want    float64
		reasons []string
	}{
		{"отчёт: завтра, 2 блока", Candidate{Importance: 5, Blocks: 2, Deadline: date("2026-11-03")}, 0.75, []string{ReasonDeadlineSoon, ReasonQuick}},
		{"просроченная мелочь", Candidate{Importance: 3, Blocks: 1, Deadline: date("2026-11-01")}, 0.63, []string{ReasonOverdue, ReasonQuick}},
		{"курсовая через 14 дней", Candidate{Importance: 7, Blocks: 12, Deadline: date("2026-11-16")}, 0.42 + 0.4*math.Pow(2, -12.0/7), []string{}},
		{"мечта без срока", Candidate{Importance: 9, Blocks: 8}, 0.54, []string{ReasonImportant}},
		{"шкаф через 30 дней", Candidate{Importance: 2, Blocks: 4, Deadline: date("2026-12-02")}, 0.12 + 0.4*math.Pow(2, -29.0/7), []string{}},
		{"дедлайн сегодня", Candidate{Importance: 1, Blocks: 3, Deadline: date("2026-11-02")}, 0.06 + 0.4, []string{ReasonDeadlineSoon}},
		// 12 блоков = 2 дня работы: при дедлайне через 2 дня запаса уже нет — срочность 1.
		{"большое дело с малым запасом", Candidate{Importance: 5, Blocks: 12, Deadline: date("2026-11-04")}, 0.3 + 0.4, []string{ReasonDeadlineSoon}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons := Score(tt.c, day)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("score = %.4f, want %.4f", got, tt.want)
			}
			if !reflect.DeepEqual(reasons, tt.reasons) {
				t.Errorf("reasons = %v, want %v", reasons, tt.reasons)
			}
		})
	}
}

func TestTop(t *testing.T) {
	day := *date("2026-11-02")
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	cands := []Candidate{
		{ID: 1, Importance: 2, Blocks: 4, CreatedAt: t0},                               // 0.12
		{ID: 2, Importance: 5, Blocks: 2, Deadline: date("2026-11-03"), CreatedAt: t0}, // 0.75
		{ID: 3, Importance: 9, Blocks: 8, CreatedAt: t0},                               // 0.54
		{ID: 4, Importance: 5, Blocks: 4, Deadline: date("2026-12-31"), CreatedAt: t0}, // 0.30 + ε
		{ID: 5, Importance: 5, Blocks: 4, CreatedAt: t0.Add(time.Hour)},                // 0.30 — младше 6
		{ID: 6, Importance: 5, Blocks: 4, CreatedAt: t0},                               // 0.30
		{ID: 7, Importance: 5, Blocks: 4, Deadline: date("2027-06-01"), CreatedAt: t0}, // 0.30 + почти 0
	}
	got := Top(cands, day, 5)
	ids := make([]int64, len(got))
	for i, s := range got {
		ids[i] = s.ID
	}
	// 4 и 7 чуть выше 0.30 из-за срочности; среди ровно 0.30 — старше раньше (6, потом 5), но в топ-5 влезает только 6.
	if want := []int64{2, 3, 4, 7, 6}; !reflect.DeepEqual(ids, want) {
		t.Errorf("top = %v, want %v", ids, want)
	}
	if len(Top(cands[:2], day, 5)) != 2 || len(Top(nil, day, 5)) != 0 {
		t.Error("top of short list must return all")
	}
}

func TestTop_TieBreakByDeadline(t *testing.T) {
	day := *date("2026-11-02")
	// Обе просрочены → U = 1, одинаковая важность и объём: раньше дедлайн — выше.
	cands := []Candidate{
		{ID: 1, Importance: 5, Blocks: 4, Deadline: date("2026-10-30")},
		{ID: 2, Importance: 5, Blocks: 4, Deadline: date("2026-10-20")},
	}
	if got := Top(cands, day, 5); got[0].ID != 2 {
		t.Errorf("top = %+v, want id 2 first", got)
	}
}

func TestPlanInput_Normalize(t *testing.T) {
	many := make([]PlanItemInput, MaxPlanItems+1)
	for i := range many {
		many[i] = PlanItemInput{TaskID: int64(i + 1), Start: i * BlockMin, Blocks: 1}
	}
	tests := []struct {
		name  string
		items []PlanItemInput
		code  string
	}{
		{"empty plan is fine", nil, ""},
		{"ok, any order", []PlanItemInput{{TaskID: 1, Start: 900, Blocks: 2}, {TaskID: 2, Start: 0, Blocks: 30}}, ""},
		{"back to back", []PlanItemInput{{TaskID: 1, Start: 540, Blocks: 2}, {TaskID: 2, Start: 600, Blocks: 1}}, ""},
		{"ends at midnight", []PlanItemInput{{TaskID: 1, Start: 1380, Blocks: 2}}, ""},
		{"too many", many, "too_many_items"},
		{"duplicate task", []PlanItemInput{{TaskID: 1, Blocks: 1}, {TaskID: 1, Start: 60, Blocks: 2}}, "invalid_task"},
		{"bad task id", []PlanItemInput{{TaskID: 0, Blocks: 1}}, "invalid_task"},
		{"zero blocks", []PlanItemInput{{TaskID: 1, Blocks: 0}}, "invalid_blocks"},
		{"41 blocks", []PlanItemInput{{TaskID: 1, Blocks: 41}}, "invalid_blocks"},
		{"not a half-hour", []PlanItemInput{{TaskID: 1, Start: 545, Blocks: 1}}, "invalid_start"},
		{"negative", []PlanItemInput{{TaskID: 1, Start: -30, Blocks: 1}}, "invalid_start"},
		{"past midnight", []PlanItemInput{{TaskID: 1, Start: 1410, Blocks: 2}}, "invalid_start"},
		{"overlap", []PlanItemInput{{TaskID: 1, Start: 600, Blocks: 1}, {TaskID: 2, Start: 540, Blocks: 3}}, "plan_overlap"},
	}
	for _, tt := range tests {
		if got := code(PlanInput{Items: tt.items}.Normalize()); got != tt.code {
			t.Errorf("%s: code = %q, want %q", tt.name, got, tt.code)
		}
	}
}

func TestSettingsPatch_Apply(t *testing.T) {
	tests := []struct {
		name string
		p    SettingsPatch
		want Settings
		code string
	}{
		{"empty keeps defaults", SettingsPatch{}, DefaultSettings, ""},
		{"move day", SettingsPatch{DayStart: new(8 * 60), DayEnd: new(22*60 + 30)}, Settings{480, 1350, 18}, ""},
		{"whole day", SettingsPatch{DayStart: new(0), DayEnd: new(24 * 60)}, Settings{0, 1440, 18}, ""},
		{"limit", SettingsPatch{DayLimit: new(12)}, Settings{540, 1380, 12}, ""},
		{"start after end", SettingsPatch{DayStart: new(23 * 60)}, Settings{}, "invalid_day_bounds"},
		{"not a half-hour", SettingsPatch{DayStart: new(9*60 + 15)}, Settings{}, "invalid_day_bounds"},
		{"past midnight", SettingsPatch{DayEnd: new(24*60 + 30)}, Settings{}, "invalid_day_bounds"},
		{"negative", SettingsPatch{DayStart: new(-30)}, Settings{}, "invalid_day_bounds"},
		{"zero limit", SettingsPatch{DayLimit: new(0)}, Settings{}, "invalid_day_limit"},
		{"limit over a day", SettingsPatch{DayLimit: new(49)}, Settings{}, "invalid_day_limit"},
	}
	for _, tt := range tests {
		got, err := tt.p.Apply(DefaultSettings)
		if code(err) != tt.code {
			t.Errorf("%s: code = %q, want %q", tt.name, code(err), tt.code)
			continue
		}
		if tt.code == "" && got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
