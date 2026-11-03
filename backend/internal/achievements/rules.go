package achievements

// Достижения (SPEC §8). Правила — чистая функция от «снимка» состояния: так один и тот же
// код работает и в подписчике eventbus, и при чтении списка, и проверяется table-driven тестом.

type Code string

const (
	FirstSession       Code = "first_session"
	Streak3            Code = "streak_3"
	Streak7            Code = "streak_7"
	Streak30           Code = "streak_30"
	Learned10          Code = "learned_10"
	Learned50          Code = "learned_50"
	Learned100         Code = "learned_100"
	Learned500         Code = "learned_500"
	FirstArchivedGroup Code = "first_archived_group"
)

// Snapshot — всё, от чего зависят достижения.
type Snapshot struct {
	Sessions       int // завершённых сессий (обычных и free)
	BestStreak     int // лучшая серия дней — не текущая: однажды достигнутое не «отзывается»
	Learned        int // слов на уровне ≥ 4 («Мастер» + «Архив»)
	ArchivedGroups int // непустых групп, где все слова в «Архиве»
}

type rule struct {
	code   Code
	target int
	metric func(Snapshot) int
}

func sessions(s Snapshot) int { return s.Sessions }
func streak(s Snapshot) int   { return s.BestStreak }
func learned(s Snapshot) int  { return s.Learned }
func archived(s Snapshot) int { return s.ArchivedGroups }

// rules — порядок = порядок показа в UI.
var rules = []rule{
	{FirstSession, 1, sessions},
	{Streak3, 3, streak},
	{Streak7, 7, streak},
	{Streak30, 30, streak},
	{Learned10, 10, learned},
	{Learned50, 50, learned},
	{Learned100, 100, learned},
	{Learned500, 500, learned},
	{FirstArchivedGroup, 1, archived},
}

// Progress — состояние одного достижения по снимку.
type Progress struct {
	Code     Code
	Progress int // min(метрика, Target) — для прогресс-бара
	Target   int
	Reached  bool
}

// Evaluate возвращает прогресс по всем достижениям в порядке показа.
func Evaluate(s Snapshot) []Progress {
	out := make([]Progress, len(rules))
	for i, r := range rules {
		v := r.metric(s)
		out[i] = Progress{Code: r.code, Progress: min(v, r.target), Target: r.target, Reached: v >= r.target}
	}
	return out
}
