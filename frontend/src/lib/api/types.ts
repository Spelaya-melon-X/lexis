// DTO API — зеркало Go-структур (SPEC §11). Меняется бэк — меняем здесь,
// svelte-check покажет все места, которые надо поправить.

export type Level = 1 | 2 | 3 | 4 | 5
/** Распределение слов по уровням: индекс = уровень − 1. */
export type Levels = [number, number, number, number, number]

/** Выделение в рунах (code points), полуинтервал [start, end). */
export interface Span {
  start: number
  end: number
}

export interface Example {
  text: string // разметка "I [go] home"
  sentence: string // без скобок
  highlights?: Span[]
}

export interface LineError {
  line: number
  text: string
  code: string
}

// ---------- words ----------

export interface Card {
  line: number
  term: string
  forms?: string[]
  translation: string
  examples?: Example[]
  groups?: string[]
}

export interface ParseResult {
  cards: Card[]
  errors: LineError[]
}

export interface BulkResult {
  created: number
  merged: number
  errors: LineError[]
}

export interface GroupRef {
  id: number
  name: string
}

export interface Word {
  id: number
  term: string
  forms?: string[]
  translation: string
  level: Level
  streak: number
  nextReviewAt: string
  lastReviewedAt: string | null
  createdAt: string
  examples?: Example[]
  groups?: GroupRef[]
}

export interface WordFilter {
  group?: number
  level?: Level
}

export interface WordPatch {
  term?: string
  translation?: string
  examples?: string[] // разметка; заменяет целиком
  groups?: string[] // заменяет целиком
}

// ---------- groups ----------

export interface Group {
  id: number
  name: string
  color: string | null
  createdAt: string
  wordCount: number
  due: number
  repeatedToday: boolean
  levels: Levels
}

export type Range = 'week' | 'month'

export interface DayPoint {
  day: string // YYYY-MM-DD
  sessions: number
  goodShare: number | null
}

export interface GroupDetail {
  group: Group
  analytics: {
    masteredPercent: number
    sessionsThisWeek: number
    sessionsThisMonth: number
    range: Range
    productivity: DayPoint[]
  }
  words: Word[]
}

export interface GroupPatch {
  name?: string
  color?: string | null // null — убрать цвет; поле не передано — не менять
}

// ---------- training ----------

export type Strategy = 'en_ru' | 'ru_en' | 'context'
export type Answer = 'good' | 'hard' | 'again'

export interface TrainingCard {
  wordId: number
  term: string
  forms?: string[]
  translation: string
  level: Level
  example?: Example
}

export interface StartResult {
  sessionId: number | null // null — карточек нет, сессия не создана
  cards: TrainingCard[]
}

export interface AnswerResult {
  levelBefore: Level
  levelAfter: Level
  firstInSession: boolean
  requeueAt?: number // для «Не знаю»: вернуть через столько карточек
}

export interface LevelChange {
  wordId: number
  term: string
  levelBefore: Level
  levelAfter: Level
}

export interface FinishResult {
  sessionId: number
  free: boolean
  total: number
  good: number
  hard: number
  again: number
  durationSec: number
  levelChanges: LevelChange[]
}

// ---------- stats ----------

export interface DayStats {
  day: string
  sessions: number
  words: number
  good: number
  hard: number
  again: number
  seconds: number
  goodShare: number | null
}

export interface Overview {
  day: DayStats
  groupsTrained: number
  byGroup: { id: number; name: string; words: number }[]
  streak: { current: number; best: number }
  totals: { words: number; learned: number; levels: Levels }
}

export interface Calendar {
  from: string
  to: string
  days: DayStats[]
}

export interface Trends {
  range: Range
  days: DayStats[]
}

export interface Achievement {
  code: string
  progress: number
  target: number
  unlockedAt: string | null
}

// ---------- settings / data ----------

export type Locale = 'ru' | 'en'

export interface Settings {
  language: Locale
  newWordsLimit: number
}

export type ImportResult =
  | { kind: 'words'; created?: number; merged?: number; errors?: LineError[] }
  | {
      kind: 'backup'
      restored: { groups: number; words: number; sessions: number; reviews: number; achievements: number; days: number; habits: number }
    }

// ---------- journal (§17.2) ----------

/** Запись дня. Дня без записи нет — бэк отдаёт пустой. */
export interface Day {
  day: string // YYYY-MM-DD
  rating: number | null // 1..10
  story: string
  wins: string[]
  dopamine: (string | null)[] // ровно 7, null — пустой квадратик
  dopamineLevel: number // 0..7
}

/** PATCH: нет поля — не менять; rating: null — снять оценку; wins заменяет список целиком. */
export interface DayPatch {
  rating?: number | null
  story?: string
  wins?: string[]
}

export interface DaySummary {
  day: string
  rating: number | null
  dopamineLevel: number
  wins: number
  hasStory: boolean
}

/** Средние за период (по дням не позже сегодня). */
export interface PeriodAverages {
  filled: number // дней с записями
  avgRating: number | null // по оценённым дням
  avgDopamine: number | null // по дням с записями
}

export interface WeekBlock {
  topic: string // «О чём»
  conclusion: string // «Вывод»
}

export interface WeekSummary extends PeriodAverages {
  days: DaySummary[] // 7 дней, Пн..Вс
  habits: HabitStat[]
  goals: Goal[]
}

/** Недельный анализ. Недели без анализа нет — пустые блоки, updatedAt: null. */
export interface Week {
  weekStart: string // понедельник
  weekEnd: string
  blocks: WeekBlock[]
  updatedAt: string | null
  summary: WeekSummary
}

export interface JournalDays {
  from: string
  to: string
  days: DaySummary[]
}

// ---------- habits (§17.4) ----------

export type HabitKind = 'positive' | 'negative'

export interface Habit {
  id: number
  name: string
  kind: HabitKind
  color: string | null
  trackTrigger: boolean
  trackRating: boolean
  trackAmount: boolean
  unit: string
  archived: boolean
  startDay: string // дата создания (YYYY-MM-DD, TZ пользователя)
  createdAt: string
}

/** Тело POST /habits. */
export type HabitFields = Pick<Habit, 'name' | 'kind' | 'color' | 'trackTrigger' | 'trackRating' | 'trackAmount' | 'unit'>

/** PATCH: нет поля — не менять; color: null — без цвета. Тип не меняется. */
export type HabitPatch = Partial<Omit<HabitFields, 'kind'>> & { archived?: boolean }

/** Отметка: есть — привычка отмечена (positive — сделал, negative — сорвался). */
export interface Checkin {
  habitId: number
  day: string
  trigger: string | null
  rating: number | null
  amount: number | null
}

/** Строка формы дня; done = false — снять отметку. Поля — только включённые у привычки. */
export interface CheckinInput {
  habitId: number
  done: boolean
  trigger?: string
  rating?: number
  amount?: number
}

export interface GoalProgress {
  done: number // positive — дней с отметкой, negative — дней без срыва
  elapsed: number
  total: number
}

export interface Goal {
  id: number
  habitId: number | null
  title: string
  from: string
  to: string
  progress: GoalProgress | null
}

export interface GoalInput {
  habitId: number | null
  title: string
  from: string
  to: string
}

export type NoteKind = 'observation' | 'insight'

export interface Note {
  id: number
  habitId: number | null
  kind: NoteKind
  body: string
  createdAt: string
  updatedAt: string
}

export interface NoteInput {
  habitId: number | null
  kind: NoteKind
  body: string
}

export interface NoteFilter {
  habit?: number | 'general'
  kind?: NoteKind
  before?: number
  limit?: number
}

/** Выполнение привычки за период. */
export interface HabitStat {
  habitId: number
  name: string
  kind: HabitKind
  color: string | null
  unit: string
  trackAmount: boolean
  trackRating: boolean
  archived: boolean
  days: number // учитываемые дни: с даты создания и не позже сегодня
  checked: number // positive — сделал, negative — сорвался
  amount: number | null
  avgRating: number | null
  bestClean: number // negative: самая длинная серия без срыва
}
