// Package focus — ритуал фокуса и награды «Игры» (SPEC §18.3, B4): фокус-сессии (настройка → работа ⇄ перерыв →
// завершение), отвлечения, чек-лист ритуала, очки за работу и магазин наград.
//
// Чистые правила (таймлайн, очки, переходы, валидация) — в этом файле; БД — service.go; HTTP — module.go.
package focus

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"lexis/internal/platform/httpx"
)

const (
	WorkChunk      = 30 * time.Minute // за каждые полчаса чистой работы —
	BreakPerChunk  = 5 * time.Minute  // 5 минут перерыва и
	PointsPerChunk = 1                // 1 очко (§18.3 B6)

	DefaultSetupMin = 10
	MaxSetupMin     = 60
	MaxTriggerLen   = 200
	MaxRitualItems  = 20
	MaxRitualLen    = 100
	MaxRewardTitle  = 100
	MaxPrice        = 100000
)

// Phase — где сейчас сессия.
type Phase string

const (
	PhaseSetup    Phase = "setup"    // таймер настройки, чек-лист ритуала
	PhaseWork     Phase = "work"     // работа
	PhaseBreak    Phase = "break"    // открыт перерыв
	PhaseFinished Phase = "finished" // завершена
)

// Outcome — чем закончилась сессия.
type Outcome string

const (
	OutcomeDone      Outcome = "done"      // «Я закончил»: дело выполнено
	OutcomeStopped   Outcome = "stopped"   // «На сегодня всё»: дело остаётся, время и очки засчитаны
	OutcomeAbandoned Outcome = "abandoned" // «Отменить» в настройке: в статистику не идёт
)

func (o Outcome) Valid() bool {
	return o == OutcomeDone || o == OutcomeStopped || o == OutcomeAbandoned
}

// Times — отметки сессии (все из clock).
type Times struct {
	StartedAt     time.Time
	WorkStartedAt *time.Time
	FinishedAt    *time.Time
}

// Span — перерыв; End nil — идёт сейчас.
type Span struct {
	Start time.Time
	End   *time.Time
}

// Timeline — разбивка времени сессии в секундах. Её же (зеркально) считает фронт для тикающих таймеров.
type Timeline struct {
	Phase             Phase `json:"phase"`
	SetupSec          int   `json:"setupSec"`
	WorkSec           int   `json:"workSec"`           // чистая работа: без настройки и перерывов
	BreakSec          int   `json:"breakSec"`          // всего на перерывах
	BreakEarnedSec    int   `json:"breakEarnedSec"`    // заработано: ⌊работа / 30 мин⌋ × 5 мин
	BreakAvailableSec int   `json:"breakAvailableSec"` // осталось: заработано − потрачено, не меньше 0
	BreakOverrunSec   int   `json:"breakOverrunSec"`   // потрачено сверх заработанного (B12: можно, но видно)
}

// Compute считает Timeline на момент now (для завершённой — на момент завершения).
// Перерыв, начатый раньше работы, невозможен (переходы это запрещают), но и не ломает расчёт.
func Compute(t Times, breaks []Span, now time.Time) Timeline {
	end := now
	if t.FinishedAt != nil {
		end = *t.FinishedAt
	}
	tl := Timeline{Phase: PhaseSetup}
	switch {
	case t.FinishedAt != nil:
		tl.Phase = PhaseFinished
	case len(breaks) > 0 && breaks[len(breaks)-1].End == nil:
		tl.Phase = PhaseBreak
	case t.WorkStartedAt != nil:
		tl.Phase = PhaseWork
	}

	setupEnd := end
	if t.WorkStartedAt != nil {
		setupEnd = *t.WorkStartedAt
	}
	tl.SetupSec = seconds(t.StartedAt, setupEnd)

	for _, b := range breaks {
		bEnd := end
		if b.End != nil && b.End.Before(end) {
			bEnd = *b.End
		}
		tl.BreakSec += seconds(b.Start, bEnd)
	}
	if t.WorkStartedAt != nil {
		tl.WorkSec = max(0, seconds(*t.WorkStartedAt, end)-tl.BreakSec)
	}
	chunks := tl.WorkSec / int(WorkChunk.Seconds())
	tl.BreakEarnedSec = chunks * int(BreakPerChunk.Seconds())
	tl.BreakAvailableSec = max(0, tl.BreakEarnedSec-tl.BreakSec)
	tl.BreakOverrunSec = max(0, tl.BreakSec-tl.BreakEarnedSec)
	return tl
}

func seconds(from, to time.Time) int {
	return max(0, int(to.Sub(from)/time.Second))
}

// Points — очки за сессию (B14): +1 за каждые полные 30 минут чистой работы; остаток не переносится.
// Отменённая в настройке сессия очков не даёт (у неё и работы нет).
func Points(tl Timeline, outcome Outcome) int {
	if outcome == OutcomeAbandoned {
		return 0
	}
	return tl.WorkSec / int(WorkChunk.Seconds()) * PointsPerChunk
}

// Action — действие над сессией.
type Action string

const (
	ActionWork        Action = "work"        // «Начал делать»
	ActionBreak       Action = "break"       // «Перерыв»
	ActionResume      Action = "resume"      // «Вернулся»
	ActionDistraction Action = "distraction" // отметить отвлечение
)

// CheckAction — разрешено ли действие в фазе (иначе 409 invalid_phase). Завершение проверяет CheckFinish.
func CheckAction(p Phase, a Action) error {
	ok := false
	switch a {
	case ActionWork:
		ok = p == PhaseSetup
	case ActionBreak:
		ok = p == PhaseWork
	case ActionResume:
		ok = p == PhaseBreak
	case ActionDistraction:
		ok = p == PhaseWork || p == PhaseBreak
	}
	if !ok {
		return invalidPhase(p, string(a))
	}
	return nil
}

// CheckFinish: «отменить» — только в настройке (B11), «закончил»/«на сегодня всё» — после начала работы
// (во время перерыва тоже: перерыв закроется). Неизвестный исход — 400.
func CheckFinish(p Phase, o Outcome) error {
	switch o {
	case OutcomeAbandoned:
		if p != PhaseSetup {
			return invalidPhase(p, string(o))
		}
	case OutcomeDone, OutcomeStopped:
		if p != PhaseWork && p != PhaseBreak {
			return invalidPhase(p, string(o))
		}
	default:
		return httpx.BadRequest("invalid_outcome", "outcome must be done, stopped or abandoned")
	}
	return nil
}

func invalidPhase(p Phase, what string) error {
	if p == PhaseFinished {
		return httpx.Conflict("focus_finished", "the focus session is already finished")
	}
	return httpx.Conflict("invalid_phase", fmt.Sprintf("%s is not possible in phase %s", what, p))
}

// ---------- ввод ----------

// DistractionInput — тело POST /sessions/{id}/distractions.
type DistractionInput struct {
	Trigger string `json:"trigger"`
	GaveIn  bool   `json:"gaveIn"`
}

func (in DistractionInput) Normalize() (DistractionInput, error) {
	in.Trigger = strings.Join(strings.Fields(in.Trigger), " ")
	if in.Trigger == "" {
		return in, httpx.BadRequest("empty_text", "trigger is empty")
	}
	if utf8.RuneCountInString(in.Trigger) > MaxTriggerLen {
		return in, httpx.BadRequest("text_too_long", fmt.Sprintf("trigger is longer than %d characters", MaxTriggerLen))
	}
	return in, nil
}

// NormalizeRitual — пункты чек-листа: trim, пустые выбрасываются, не больше 20, каждый ≤ 100.
func NormalizeRitual(items []string) ([]string, error) {
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.Join(strings.Fields(it), " ")
		if it == "" {
			continue
		}
		if utf8.RuneCountInString(it) > MaxRitualLen {
			return nil, httpx.BadRequest("text_too_long", fmt.Sprintf("a ritual item is longer than %d characters", MaxRitualLen))
		}
		out = append(out, it)
	}
	if len(out) > MaxRitualItems {
		return nil, httpx.BadRequest("too_many_items", fmt.Sprintf("a ritual has at most %d items", MaxRitualItems))
	}
	return out, nil
}

// RewardInput — тело POST /rewards; база для PATCH.
type RewardInput struct {
	Title string `json:"title"`
	Price int    `json:"price"`
}

func (in RewardInput) Normalize() (RewardInput, error) {
	in.Title = strings.Join(strings.Fields(in.Title), " ")
	if in.Title == "" || utf8.RuneCountInString(in.Title) > MaxRewardTitle {
		return in, httpx.BadRequest("invalid_title", fmt.Sprintf("title must be 1..%d characters", MaxRewardTitle))
	}
	if in.Price < 1 || in.Price > MaxPrice {
		return in, httpx.BadRequest("invalid_price", fmt.Sprintf("price must be 1..%d", MaxPrice))
	}
	return in, nil
}

// RewardPatch — тело PATCH /rewards/{id}. Нет поля — не менять; archived: true — в архив (из магазина пропадает).
type RewardPatch struct {
	Title    *string `json:"title"`
	Price    *int    `json:"price"`
	Archived *bool   `json:"archived"`
}

func (p RewardPatch) Apply(cur RewardInput, archivedAt *time.Time, now time.Time) (RewardInput, *time.Time, error) {
	if p.Title != nil {
		cur.Title = *p.Title
	}
	if p.Price != nil {
		cur.Price = *p.Price
	}
	if p.Archived != nil {
		switch {
		case !*p.Archived:
			archivedAt = nil
		case archivedAt == nil:
			archivedAt = &now
		}
	}
	cur, err := cur.Normalize()
	return cur, archivedAt, err
}

// ---------- настройки ----------

// SettingSetupMin — длительность таймера настройки, минуты (JSON-число в settings).
const SettingSetupMin = "focus_setup_min"

type Settings struct {
	SetupMin int `json:"setupMin"`
}

type SettingsPatch struct {
	SetupMin *int `json:"setupMin"`
}

func (p SettingsPatch) Apply(cur Settings) (Settings, error) {
	if p.SetupMin != nil {
		cur.SetupMin = *p.SetupMin
	}
	if cur.SetupMin < 1 || cur.SetupMin > MaxSetupMin {
		return cur, httpx.BadRequest("invalid_setup_min", fmt.Sprintf("setup must be 1..%d minutes", MaxSetupMin))
	}
	return cur, nil
}
