package focus

import (
	"errors"
	"strings"
	"testing"
	"time"

	"lexis/internal/platform/httpx"
)

func code(err error) string {
	var he *httpx.Error
	if errors.As(err, &he) {
		return he.Code
	}
	if err != nil {
		return "unexpected: " + err.Error()
	}
	return ""
}

var t0 = time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

// at — момент через m минут после t0.
func at(m float64) time.Time     { return t0.Add(time.Duration(m * float64(time.Minute))) }
func ptr(t time.Time) *time.Time { return &t }

func TestCompute(t *testing.T) {
	tests := []struct {
		name   string
		times  Times
		breaks []Span
		now    time.Time
		want   Timeline
	}{
		{
			"setup, 4 min in", Times{StartedAt: t0}, nil, at(4),
			Timeline{Phase: PhaseSetup, SetupSec: 240},
		},
		{
			"setup overrun is just longer setup", Times{StartedAt: t0}, nil, at(15),
			Timeline{Phase: PhaseSetup, SetupSec: 900},
		},
		{
			"work 29 min — no break earned yet", Times{StartedAt: t0, WorkStartedAt: ptr(at(10))}, nil, at(39),
			Timeline{Phase: PhaseWork, SetupSec: 600, WorkSec: 29 * 60},
		},
		{
			"work 65 min — 10 min earned", Times{StartedAt: t0, WorkStartedAt: ptr(at(10))}, nil, at(75),
			Timeline{Phase: PhaseWork, SetupSec: 600, WorkSec: 65 * 60, BreakEarnedSec: 600, BreakAvailableSec: 600},
		},
		{
			"on a break: break time is not work", Times{StartedAt: t0, WorkStartedAt: ptr(at(10))},
			[]Span{{Start: at(70)}}, at(73),
			Timeline{Phase: PhaseBreak, SetupSec: 600, WorkSec: 60 * 60, BreakSec: 180, BreakEarnedSec: 600, BreakAvailableSec: 420},
		},
		{
			"break overrun", Times{StartedAt: t0, WorkStartedAt: ptr(at(0))},
			[]Span{{Start: at(30), End: ptr(at(42))}}, at(50),
			// работа 50 − 12 = 38 мин → заработано 5 мин, потрачено 12 → перерасход 7
			Timeline{Phase: PhaseWork, WorkSec: 38 * 60, BreakSec: 720, BreakEarnedSec: 300, BreakOverrunSec: 420},
		},
		{
			"finished: counted up to finish, not now", Times{StartedAt: t0, WorkStartedAt: ptr(at(10)), FinishedAt: ptr(at(100))},
			[]Span{{Start: at(40), End: ptr(at(45))}}, at(500),
			Timeline{Phase: PhaseFinished, SetupSec: 600, WorkSec: 85 * 60, BreakSec: 300, BreakEarnedSec: 600, BreakAvailableSec: 300},
		},
		{
			"finished during setup (abandoned)", Times{StartedAt: t0, FinishedAt: ptr(at(3))}, nil, at(100),
			Timeline{Phase: PhaseFinished, SetupSec: 180},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compute(tt.times, tt.breaks, tt.now); got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestPoints(t *testing.T) {
	tests := []struct {
		work    int
		outcome Outcome
		want    int
	}{
		{29 * 60, OutcomeDone, 0},
		{30 * 60, OutcomeDone, 1},
		{89 * 60, OutcomeStopped, 2},
		{180 * 60, OutcomeDone, 6},
		{60 * 60, OutcomeAbandoned, 0},
	}
	for _, tt := range tests {
		if got := Points(Timeline{WorkSec: tt.work}, tt.outcome); got != tt.want {
			t.Errorf("Points(%d min, %s) = %d, want %d", tt.work/60, tt.outcome, got, tt.want)
		}
	}
}

func TestCheckAction(t *testing.T) {
	tests := []struct {
		p    Phase
		a    Action
		code string
	}{
		{PhaseSetup, ActionWork, ""},
		{PhaseWork, ActionWork, "invalid_phase"},
		{PhaseWork, ActionBreak, ""},
		{PhaseSetup, ActionBreak, "invalid_phase"},
		{PhaseBreak, ActionBreak, "invalid_phase"},
		{PhaseBreak, ActionResume, ""},
		{PhaseWork, ActionResume, "invalid_phase"},
		{PhaseWork, ActionDistraction, ""},
		{PhaseBreak, ActionDistraction, ""},
		{PhaseSetup, ActionDistraction, "invalid_phase"},
		{PhaseFinished, ActionWork, "focus_finished"},
	}
	for _, tt := range tests {
		if got := code(CheckAction(tt.p, tt.a)); got != tt.code {
			t.Errorf("%s in %s: %q, want %q", tt.a, tt.p, got, tt.code)
		}
	}
}

func TestCheckFinish(t *testing.T) {
	tests := []struct {
		p    Phase
		o    Outcome
		code string
	}{
		{PhaseSetup, OutcomeAbandoned, ""},
		{PhaseWork, OutcomeAbandoned, "invalid_phase"},
		{PhaseWork, OutcomeDone, ""},
		{PhaseBreak, OutcomeStopped, ""},
		{PhaseSetup, OutcomeDone, "invalid_phase"},
		{PhaseFinished, OutcomeDone, "focus_finished"},
		{PhaseWork, "maybe", "invalid_outcome"},
	}
	for _, tt := range tests {
		if got := code(CheckFinish(tt.p, tt.o)); got != tt.code {
			t.Errorf("%s in %s: %q, want %q", tt.o, tt.p, got, tt.code)
		}
	}
}

func TestDistractionInput_Normalize(t *testing.T) {
	got, err := DistractionInput{Trigger: "  телефон   завибрировал ", GaveIn: true}.Normalize()
	if err != nil || got.Trigger != "телефон завибрировал" || !got.GaveIn {
		t.Errorf("got %+v %v", got, err)
	}
	if _, err := (DistractionInput{Trigger: " "}).Normalize(); code(err) != "empty_text" {
		t.Errorf("empty: %v", err)
	}
	if _, err := (DistractionInput{Trigger: strings.Repeat("я", 201)}).Normalize(); code(err) != "text_too_long" {
		t.Errorf("long: %v", err)
	}
}

func TestNormalizeRitual(t *testing.T) {
	got, err := NormalizeRitual([]string{" Налил  воды ", "", "  ", "Не беспокоить"})
	if err != nil || len(got) != 2 || got[0] != "Налил воды" {
		t.Errorf("got %v %v", got, err)
	}
	if got, _ := NormalizeRitual(nil); got == nil || len(got) != 0 {
		t.Errorf("nil → %#v, want empty non-nil", got)
	}
	many := make([]string, MaxRitualItems+1)
	for i := range many {
		many[i] = "x"
	}
	if _, err := NormalizeRitual(many); code(err) != "too_many_items" {
		t.Errorf("many: %v", err)
	}
	if _, err := NormalizeRitual([]string{strings.Repeat("я", 101)}); code(err) != "text_too_long" {
		t.Errorf("long: %v", err)
	}
}

func TestReward(t *testing.T) {
	if got, err := (RewardInput{Title: "  Кино ", Price: 6}).Normalize(); err != nil || got.Title != "Кино" {
		t.Errorf("ok: %+v %v", got, err)
	}
	for _, in := range []RewardInput{{Title: "", Price: 1}, {Title: strings.Repeat("я", 101), Price: 1}} {
		if _, err := in.Normalize(); code(err) != "invalid_title" {
			t.Errorf("%+v: %v", in, err)
		}
	}
	for _, price := range []int{0, -5, MaxPrice + 1} {
		if _, err := (RewardInput{Title: "x", Price: price}).Normalize(); code(err) != "invalid_price" {
			t.Errorf("price %d: %v", price, err)
		}
	}

	now := at(0)
	earlier := at(-60)
	cur := RewardInput{Title: "Кино", Price: 6}
	got, arch, err := RewardPatch{Price: new(8), Archived: new(true)}.Apply(cur, nil, now)
	if err != nil || got.Price != 8 || arch == nil || !arch.Equal(now) {
		t.Errorf("archive: %+v %v %v", got, arch, err)
	}
	if _, arch, _ := (RewardPatch{Archived: new(true)}).Apply(cur, &earlier, now); !arch.Equal(earlier) {
		t.Errorf("re-archive moved the date: %v", arch)
	}
	if _, arch, _ := (RewardPatch{Archived: new(false)}).Apply(cur, &earlier, now); arch != nil {
		t.Errorf("unarchive: %v", arch)
	}
}

func TestSettingsPatch(t *testing.T) {
	def := Settings{SetupMin: DefaultSetupMin}
	if got, err := (SettingsPatch{SetupMin: new(5)}).Apply(def); err != nil || got.SetupMin != 5 {
		t.Errorf("ok: %+v %v", got, err)
	}
	for _, m := range []int{0, 61} {
		if _, err := (SettingsPatch{SetupMin: new(m)}).Apply(def); code(err) != "invalid_setup_min" {
			t.Errorf("%d: %v", m, err)
		}
	}
}
