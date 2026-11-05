package tasks

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"lexis/internal/platform/clock"
	"lexis/internal/platform/httpx"
)

// Распорядок дня и топ-5 (SPEC §18.3, B3). Чистые функции: формула, проверка плана, настройки.

const (
	TopN         = 5  // сколько дел предлагать на день
	MaxPlanItems = 30 // пунктов в одном дне
	LeftoverDays = 7  // «не доделал» — из планов последней недели
)

// ---------- топ-5 ----------

// Candidate — что нужно формуле от дела (невыполненный лист).
type Candidate struct {
	ID         int64
	Importance int
	Blocks     int
	Deadline   *time.Time // гражданская дата
	CreatedAt  time.Time
}

// Причины в ответе — коды для i18n: UI пишет, почему дело в топе.
const (
	ReasonOverdue      = "overdue"
	ReasonDeadlineSoon = "deadline_soon"
	ReasonImportant    = "important"
	ReasonQuick        = "quick"
)

// Веса формулы (§18.3). Важность — главный вклад, срочность — заметный, «быстрая победа» — лишь подталкивает.
const (
	weightImportance = 0.6
	weightUrgency    = 0.4
	quickBonus       = 0.05
	quickBlocks      = 2   // ≤ 1 часа
	blocksPerDay     = 6   // 3 часа — сколько реально делать одно дело за день
	urgencyHalfLife  = 7.0 // дней: через неделю «запаса» срочность вдвое меньше
	soonDays         = 3   // запас ≤ 3 дней — «скоро дедлайн»
	importantFrom    = 8
)

// Score — насколько стоит взять дело в день day (§18.3):
//
//	I = importance / 10
//	d = deadline − day, dEff = d − ⌈blocks / 6⌉   (большое дело «срочнеет» раньше: его надо начать заранее)
//	U = 0 без дедлайна; 1 при dEff ≤ 0; иначе 2^(−dEff / 7)
//	score = 0.6·I + 0.4·U + (0.05, если blocks ≤ 2)
func Score(c Candidate, day time.Time) (float64, []string) {
	reasons := []string{}
	urgency := 0.0
	if c.Deadline != nil {
		d := clock.DaysBetween(day, *c.Deadline)
		eff := d - (c.Blocks+blocksPerDay-1)/blocksPerDay
		switch {
		case eff <= 0:
			urgency = 1
		default:
			urgency = math.Pow(2, -float64(eff)/urgencyHalfLife)
		}
		switch {
		case d < 0:
			reasons = append(reasons, ReasonOverdue)
		case eff <= soonDays:
			reasons = append(reasons, ReasonDeadlineSoon)
		}
	}
	score := weightImportance*float64(c.Importance)/10 + weightUrgency*urgency
	if c.Importance >= importantFrom {
		reasons = append(reasons, ReasonImportant)
	}
	if c.Blocks <= quickBlocks {
		score += quickBonus
		reasons = append(reasons, ReasonQuick)
	}
	return score, reasons
}

// Scored — дело с оценкой.
type Scored struct {
	ID      int64
	Score   float64
	Reasons []string
}

// Top — n лучших по Score. При равенстве — раньше дедлайн (без дедлайна — после), потом старше дело, потом меньший id.
func Top(cands []Candidate, day time.Time, n int) []Scored {
	type row struct {
		c Candidate
		s Scored
	}
	rows := make([]row, len(cands))
	for i, c := range cands {
		score, reasons := Score(c, day)
		rows[i] = row{c, Scored{ID: c.ID, Score: score, Reasons: reasons}}
	}
	slices.SortFunc(rows, func(a, b row) int {
		// Сравнение с допуском: 0.1+0.2 и 0.3 в float64 различаются в последнем знаке — для «равных» дел это шум.
		if math.Abs(a.s.Score-b.s.Score) > 1e-9 {
			return cmp.Compare(b.s.Score, a.s.Score)
		}
		if c := compareDeadline(a.c.Deadline, b.c.Deadline); c != 0 {
			return c
		}
		return cmp.Or(a.c.CreatedAt.Compare(b.c.CreatedAt), cmp.Compare(a.c.ID, b.c.ID))
	})
	out := make([]Scored, 0, min(n, len(rows)))
	for _, r := range rows[:min(n, len(rows))] {
		out = append(out, r.s)
	}
	return out
}

func compareDeadline(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return a.Compare(*b)
}

// ---------- план дня ----------

// BlockMin — длина блока в минутах: и объём, и шаг шкалы распорядка.
const BlockMin = 30

type PlanItemInput struct {
	TaskID int64 `json:"taskId"`
	Start  int   `json:"start"` // минуты от полуночи, по получасам
	Blocks int   `json:"blocks"`
}

// PlanInput — тело PUT /api/tasks/plan/{day}: весь план дня (порядок в массиве не важен — важно время).
type PlanInput struct {
	Items []PlanItemInput `json:"items"`
}

// Normalize проверяет форму плана: дела по разу, объём 1..40, начало — по получасам и дело
// целиком в сутках, пункты не пересекаются (при перетаскивании фронт меняет их местами, §18.3).
// Что дела существуют и это невыполненные листья — проверяет сервис.
func (in PlanInput) Normalize() error {
	if len(in.Items) > MaxPlanItems {
		return httpx.BadRequest("too_many_items", fmt.Sprintf("a day plan has at most %d items", MaxPlanItems))
	}
	seen := make(map[int64]bool, len(in.Items))
	for _, it := range in.Items {
		if it.TaskID <= 0 || seen[it.TaskID] {
			return httpx.BadRequest("invalid_task", "each taskId must be a positive id and appear once")
		}
		seen[it.TaskID] = true
		if it.Blocks < 1 || it.Blocks > MaxBlocks {
			return httpx.BadRequest("invalid_blocks", fmt.Sprintf("blocks must be 1..%d", MaxBlocks))
		}
		if it.Start < 0 || it.Start%BlockMin != 0 || it.Start+it.Blocks*BlockMin > 24*60 {
			return httpx.BadRequest("invalid_start", "start must be a half-hour, and the task must fit into the day")
		}
	}
	sorted := slices.Clone(in.Items)
	slices.SortFunc(sorted, func(a, b PlanItemInput) int { return cmp.Compare(a.Start, b.Start) })
	for i := 1; i < len(sorted); i++ {
		if prev := sorted[i-1]; sorted[i].Start < prev.Start+prev.Blocks*BlockMin {
			return httpx.BadRequest("plan_overlap", "plan items must not overlap")
		}
	}
	return nil
}

// ---------- настройки раздела ----------

// Ключи в таблице settings (JSON-числа) — бэкапятся вместе с остальными настройками.
const (
	SettingDayStart = "game_day_start" // минуты от полуночи
	SettingDayEnd   = "game_day_end"
	SettingDayLimit = "game_day_limit" // блоков
)

// Settings — шкала дня и лимит (§18.3: по умолчанию 9:00–23:00 и 9 часов).
type Settings struct {
	DayStart int `json:"dayStart"`
	DayEnd   int `json:"dayEnd"`
	DayLimit int `json:"dayLimit"`
}

var DefaultSettings = Settings{DayStart: 9 * 60, DayEnd: 23 * 60, DayLimit: 18}

// SettingsPatch — тело PUT /api/tasks/settings; нет поля — не менять.
type SettingsPatch struct {
	DayStart *int `json:"dayStart"`
	DayEnd   *int `json:"dayEnd"`
	DayLimit *int `json:"dayLimit"`
}

// Apply: начало и конец — по получасам (шкала в блоках), начало раньше конца, лимит 1..48 блоков.
func (p SettingsPatch) Apply(cur Settings) (Settings, error) {
	if p.DayStart != nil {
		cur.DayStart = *p.DayStart
	}
	if p.DayEnd != nil {
		cur.DayEnd = *p.DayEnd
	}
	if p.DayLimit != nil {
		cur.DayLimit = *p.DayLimit
	}
	if cur.DayStart < 0 || cur.DayEnd > 24*60 || cur.DayStart >= cur.DayEnd || cur.DayStart%30 != 0 || cur.DayEnd%30 != 0 {
		return cur, httpx.BadRequest("invalid_day_bounds", "day start/end must be half-hours within the day, start < end")
	}
	if cur.DayLimit < 1 || cur.DayLimit > 48 {
		return cur, httpx.BadRequest("invalid_day_limit", "day limit must be 1..48 blocks")
	}
	return cur, nil
}
