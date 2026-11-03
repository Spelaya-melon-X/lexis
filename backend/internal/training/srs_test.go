package training

import (
	"fmt"
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*60*60)

// at — момент в МСК: день ноября 2026 и время.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 11, day, hour, minute, 0, 0, msk)
}

func ptr(t time.Time) *time.Time { return &t }

func TestApply(t *testing.T) {
	now := at(10, 12, 0)

	tests := []struct {
		name   string
		in     State
		answer Answer
		want   State
	}{
		// Уровень 1: любой первый ответ → 2, серия 0.
		{"new + good → learning, streak not counted",
			State{Level: 1}, AnswerGood,
			State{Level: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now)}},
		{"new + hard → learning",
			State{Level: 1}, AnswerHard,
			State{Level: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now)}},
		{"new + again → learning (not below 2)",
			State{Level: 1}, AnswerAgain,
			State{Level: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now)}},

		// Уровень 2: 3 «Знаю» с шагом ≥ 1 день.
		{"learning: first good of series always counts",
			State{Level: 2}, AnswerGood,
			State{Level: 2, Streak: 1, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},
		{"learning: good next day counts",
			State{Level: 2, Streak: 1, LastGoodAt: ptr(at(9, 20, 0))}, AnswerGood,
			State{Level: 2, Streak: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},
		{"learning: second good same day not counted, not reset",
			State{Level: 2, Streak: 1, LastGoodAt: ptr(at(10, 8, 0))}, AnswerGood,
			State{Level: 2, Streak: 1, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(at(10, 8, 0))}},
		{"learning: third counted good → known, streak reset, interval 3 days",
			State{Level: 2, Streak: 2, LastGoodAt: ptr(at(9, 12, 0))}, AnswerGood,
			State{Level: 3, NextReviewAt: at(13, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},
		{"learning: third good same day → no promotion",
			State{Level: 2, Streak: 2, LastGoodAt: ptr(at(10, 9, 0))}, AnswerGood,
			State{Level: 2, Streak: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(at(10, 9, 0))}},
		{"learning: after hard (streak 0) good counts even same day",
			State{Level: 2, Streak: 0, LastGoodAt: ptr(at(10, 9, 0))}, AnswerGood,
			State{Level: 2, Streak: 1, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},

		// Календарные дни, а не 24 часа.
		{"midnight: 23:59 → 00:01 is a different day",
			State{Level: 2, Streak: 1, LastGoodAt: ptr(at(9, 23, 59))}, AnswerGood,
			State{Level: 2, Streak: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},
		{"timezone: last good stored in UTC is compared in user TZ",
			// 2026-11-09 22:30 UTC = 10 ноября 01:30 МСК — тот же день, что и now.
			State{Level: 2, Streak: 1, LastGoodAt: ptr(time.Date(2026, 11, 9, 22, 30, 0, 0, time.UTC))}, AnswerGood,
			State{Level: 2, Streak: 1, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(time.Date(2026, 11, 9, 22, 30, 0, 0, time.UTC))}},

		// Уровень 3: 3 «Знаю» с шагом ≥ 3 дня.
		{"known: good 2 days after previous not counted",
			State{Level: 3, Streak: 1, LastGoodAt: ptr(at(8, 12, 0))}, AnswerGood,
			State{Level: 3, Streak: 1, NextReviewAt: at(13, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(at(8, 12, 0))}},
		{"known: good 3 days after previous counted",
			State{Level: 3, Streak: 1, LastGoodAt: ptr(at(7, 12, 0))}, AnswerGood,
			State{Level: 3, Streak: 2, NextReviewAt: at(13, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},
		{"known: third counted good → master, interval 7 days",
			State{Level: 3, Streak: 2, LastGoodAt: ptr(at(7, 12, 0))}, AnswerGood,
			State{Level: 4, NextReviewAt: at(17, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},

		// Уровень 4: 2 «Знаю» с шагом ≥ 7 дней.
		{"master: good 6 days after previous not counted",
			State{Level: 4, Streak: 1, LastGoodAt: ptr(at(4, 12, 0))}, AnswerGood,
			State{Level: 4, Streak: 1, NextReviewAt: at(17, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(at(4, 12, 0))}},
		{"master: second counted good → archive",
			State{Level: 4, Streak: 1, LastGoodAt: ptr(at(3, 12, 0))}, AnswerGood,
			State{Level: 5, NextReviewAt: now, LastReviewedAt: ptr(now), LastGoodAt: ptr(now)}},

		// «Затрудняюсь»: серия 0, уровень тот же, завтра.
		{"hard resets streak, keeps level, tomorrow",
			State{Level: 3, Streak: 2, LastGoodAt: ptr(at(7, 12, 0))}, AnswerHard,
			State{Level: 3, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(at(7, 12, 0))}},

		// «Не знаю»: −1 уровень, не ниже 2, серия 0, завтра.
		{"again on master → known",
			State{Level: 4, Streak: 1, LastGoodAt: ptr(at(3, 12, 0))}, AnswerAgain,
			State{Level: 3, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now), LastGoodAt: ptr(at(3, 12, 0))}},
		{"again on known → learning",
			State{Level: 3, Streak: 1}, AnswerAgain,
			State{Level: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now)}},
		{"again on learning stays learning",
			State{Level: 2, Streak: 2}, AnswerAgain,
			State{Level: 2, NextReviewAt: at(11, 12, 0), LastReviewedAt: ptr(now)}},

		// Архив не тренируется — состояние не меняется вовсе.
		{"archive is untouched",
			State{Level: 5, NextReviewAt: at(1, 0, 0)}, AnswerAgain,
			State{Level: 5, NextReviewAt: at(1, 0, 0)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.in, tt.answer, now)
			if !stateEqual(got, tt.want) {
				t.Errorf("\n got  %s\n want %s", fmtState(got), fmtState(tt.want))
			}
		})
	}
}

// DST: интервал — календарные дни (AddDate), а не 24h×N.
func TestApply_IntervalIsCalendarDays(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, berlin) // 25 октября — переход на зимнее время
	got := Apply(State{Level: 3, Streak: 1}, AnswerHard, now)
	if want := time.Date(2026, 10, 25, 12, 0, 0, 0, berlin); !got.NextReviewAt.Equal(want) {
		t.Errorf("next = %v, want %v", got.NextReviewAt, want)
	}
}

func TestAnswer_Valid(t *testing.T) {
	for _, a := range []Answer{AnswerGood, AnswerHard, AnswerAgain} {
		if !a.Valid() {
			t.Errorf("%q must be valid", a)
		}
	}
	for _, a := range []Answer{"", "GOOD", "easy"} {
		if a.Valid() {
			t.Errorf("%q must be invalid", a)
		}
	}
}

func stateEqual(a, b State) bool {
	return a.Level == b.Level && a.Streak == b.Streak && a.NextReviewAt.Equal(b.NextReviewAt) &&
		timePtrEqual(a.LastReviewedAt, b.LastReviewedAt) && timePtrEqual(a.LastGoodAt, b.LastGoodAt)
}

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func fmtState(s State) string {
	f := func(t *time.Time) string {
		if t == nil {
			return "nil"
		}
		return t.Format("02 15:04 MST")
	}
	return fmt.Sprintf("level=%d streak=%d next=%s reviewed=%s good=%s",
		s.Level, s.Streak, s.NextReviewAt.Format("02 15:04 MST"), f(s.LastReviewedAt), f(s.LastGoodAt))
}
