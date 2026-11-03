package eventbus

import "time"

// Event — маркерный интерфейс. Неэкспортируемый метод «запечатывает» его:
// событием может быть только тип из этого пакета, и подписчик получает
// конечный набор вариантов для type switch.
//
// События описаны здесь, а не в модулях-издателях, чтобы achievements/stats
// не импортировали training/words (и не ловили циклы импортов).
type Event interface{ isEvent() }

// SessionFinished — завершена тренировочная сессия.
type SessionFinished struct {
	SessionID   int64
	GroupID     int64 // 0, если группа удалена
	Free        bool
	Good        int
	Hard        int
	Again       int
	DurationSec int
	FinishedAt  time.Time
}

// WordLevelChanged — SRS поменял уровень слова (в free-сессиях не публикуется).
type WordLevelChanged struct {
	WordID    int64
	FromLevel int16
	ToLevel   int16
	At        time.Time
}

// WordsAdded — сохранён пакет слов через /api/words/bulk или импорт.
type WordsAdded struct {
	Created int
	Merged  int
	At      time.Time
}

func (SessionFinished) isEvent()  {}
func (WordLevelChanged) isEvent() {}
func (WordsAdded) isEvent()       {}
