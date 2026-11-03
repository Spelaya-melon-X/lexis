package training

import (
	"time"

	"lexis/internal/platform/clock"
)

// SRS — 5 уровней (SPEC §6). Чистые функции: «сейчас» приходит параметром.

type Answer string

const (
	AnswerGood  Answer = "good"  // «Знаю»
	AnswerHard  Answer = "hard"  // «Затрудняюсь»
	AnswerAgain Answer = "again" // «Не знаю»
)

func (a Answer) Valid() bool {
	return a == AnswerGood || a == AnswerHard || a == AnswerAgain
}

const (
	LevelNew      int16 = 1
	LevelLearning int16 = 2
	LevelKnown    int16 = 3
	LevelMaster   int16 = 4
	LevelArchive  int16 = 5
)

// levelRule — что нужно на уровне, чтобы подняться выше.
type levelRule struct {
	intervalDays   int   // через сколько дней показать после «Знаю»
	goodsToPromote int16 // сколько засчитанных «Знаю» подряд нужно для перехода
	minGapDays     int   // минимум календарных дней между засчитанными «Знаю»
}

// Уровень 2: «минимум в 2 разных дня» реализовано как шаг ≥ 1 день между «Знаю» —
// одно правило на все уровни (решено 2026-11-02, SPEC §6).
var rules = map[int16]levelRule{
	LevelLearning: {intervalDays: 1, goodsToPromote: 3, minGapDays: 1},
	LevelKnown:    {intervalDays: 3, goodsToPromote: 3, minGapDays: 3},
	LevelMaster:   {intervalDays: 7, goodsToPromote: 2, minGapDays: 7},
}

// После «Затрудняюсь»/«Не знаю» слово возвращается завтра, независимо от уровня.
const retryDays = 1

// State — SRS-поля слова.
type State struct {
	Level          int16
	Streak         int16
	NextReviewAt   time.Time
	LastReviewedAt *time.Time
	LastGoodAt     *time.Time // последнее ЗАСЧИТАННОЕ в серию «Знаю»
}

// Apply применяет ответ к состоянию. Вызывается только для первого ответа на слово
// в обычной сессии — повторные ответы и free-сессии SRS не трогают (это решает сервис).
func Apply(s State, a Answer, now time.Time) State {
	if s.Level >= LevelArchive { // архив в обычные сессии не попадает; на всякий случай — без изменений
		return s
	}
	s.LastReviewedAt = &now

	if s.Level == LevelNew {
		// Любой первый ответ → «Изучаю», серия с нуля (знакомство в серию не идёт).
		s.Level, s.Streak = LevelLearning, 0
		s.NextReviewAt = now.AddDate(0, 0, rules[LevelLearning].intervalDays)
		return s
	}

	switch a {
	case AnswerGood:
		r := rules[s.Level]
		// Первый «Знаю» серии засчитывается всегда, следующие — только через minGapDays.
		// .In(now.Location()): DaysBetween считает даты в TZ первого аргумента, а время из БД
		// приходит в UTC — без приведения 01:00 МСК оказалось бы «вчера».
		if s.Streak == 0 || s.LastGoodAt == nil ||
			clock.DaysBetween(s.LastGoodAt.In(now.Location()), now) >= r.minGapDays {
			s.Streak++
			s.LastGoodAt = &now
		}
		if s.Streak >= r.goodsToPromote {
			s.Level++
			s.Streak = 0 // «ещё N Знаю» на новом уровне — серия заново
		}
		if next, ok := rules[s.Level]; ok {
			s.NextReviewAt = now.AddDate(0, 0, next.intervalDays)
		} else {
			s.NextReviewAt = now // архив: не due по уровню, дата не важна
		}
	case AnswerHard:
		s.Streak = 0
		s.NextReviewAt = now.AddDate(0, 0, retryDays)
	case AnswerAgain:
		s.Level = max(s.Level-1, LevelLearning)
		s.Streak = 0
		s.NextReviewAt = now.AddDate(0, 0, retryDays)
	}
	return s
}
