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

// Счётчики — по поддереву: группа + подгруппы, слова уникально (§18.2).
export interface Group {
  id: number
  name: string
  color: string | null
  parentId: number | null // null — корень
  createdAt: string
  wordCount: number
  ownWordCount: number // слова самой группы
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
  parentId?: number | null // null — в корень
}

export interface GroupCreate {
  name: string
  parentId?: number | null
  color?: string | null
}

/** Что делать с подгруппами при удалении: поднять на уровень выше или удалить вместе. */
export type GroupChildren = 'lift' | 'delete'

// ---------- training ----------

export type Answer = 'good' | 'hard' | 'again'
export type TrainingDirection = 'en_ru' | 'ru_en'

export interface TrainingCard {
  wordId: number
  term: string
  forms?: string[]
  translation: string
  level: Level
  examples?: Example[]
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
      restored: { timelineGoals: number; studyGroups: number; studyCards: number; dreams: number; projects: number; groups: number; words: number; sessions: number; reviews: number; achievements: number; days: number; habits: number }
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

// ---------- игра: дела (§18.3) ----------

export interface TaskType {
  id: number
  name: string
  color: string | null
  position: number
}

export interface TaskColumn {
  id: number
  name: string
  position: number
}

/** Blocks, done, doneAt у дела с шагами вычислены по листьям (§18.3, B2). */
export interface Task {
  id: number
  parentId: number | null
  leaf: boolean // без шагов — только такие на доске
  title: string
  blocks: number // по 30 минут; у родителя — сумма листьев
  ownBlocks: number // собственная оценка
  importance: number // 1..10
  deadline: string | null // YYYY-MM-DD
  typeId: number | null
  columnId: number
  position: number
  note: string
  done: boolean
  doneAt: string | null
  workSec: number // время в фокусе; у родителя — вместе с шагами
  createdAt: string
  updatedAt: string
}

export interface TaskInput {
  title: string
  blocks: number
  importance: number
  deadline?: string | null
  typeId?: number | null
  columnId: number
  note?: string
}

/** Нет поля — не менять; deadline/typeId: null — убрать. */
export type TaskPatch = Partial<Omit<TaskInput, 'columnId'>> & { done?: boolean }

export type TaskFilter = 'false' | 'true' | 'all'

/** Шаг при разбиении: не задано — как у родителя. */
export interface TaskStep {
  title: string
  blocks: number
  importance?: number
  deadline?: string
  typeId?: number
}

// ---------- игра: распорядок дня (§18.3, B3) ----------

export interface GameSettings {
  dayStart: number // минуты от полуночи
  dayEnd: number
  dayLimit: number // блоков
}

export interface PlanItem {
  task: Task
  start: number // минуты от полуночи, по получасам
  blocks: number
}

/** Коды причин — ключи game.plan.reason.* */
export type TopReason = 'overdue' | 'deadline_soon' | 'important' | 'quick'

export interface TopItem {
  task: Task
  score: number
  reasons: TopReason[]
}

export interface Plan {
  day: string
  editable: boolean // прошлые дни — только чтение
  items: PlanItem[]
  totalBlocks: number
  settings: GameSettings
  top: TopItem[]
  leftovers: Task[]
}

// ---------- игра: фокус и награды (§18.3, B4) ----------

export type FocusPhase = 'setup' | 'work' | 'break' | 'finished'
export type FocusOutcome = 'done' | 'stopped' | 'abandoned'
export type FocusAction = 'work' | 'break' | 'resume'

/** Разбивка времени сессии в секундах (зеркало Go focus.Compute — lib/focus.timeline). */
export interface FocusTimeline {
  phase: FocusPhase
  setupSec: number
  workSec: number
  breakSec: number
  breakEarnedSec: number
  breakAvailableSec: number
  breakOverrunSec: number
}

export interface FocusSession {
  id: number
  taskId: number | null
  taskTitle: string
  day: string
  plannedBlocks: number
  startedAt: string
  workStartedAt: string | null
  finishedAt: string | null
  outcome: FocusOutcome | null
  breaks: { startedAt: string; endedAt: string | null }[]
  distractions: { id: number; at: string; trigger: string; gaveIn: boolean }[]
  timeline: FocusTimeline
  points: number
  now: string // время сервера — для поправки часов
}

export interface Reward {
  id: number
  title: string
  price: number
  archived: boolean
}

export interface LedgerEntry {
  id: number
  at: string
  delta: number
  kind: 'focus' | 'purchase'
  title: string
}

export interface Purchase {
  balance: number
  dopamine: '' | 'added' | 'full' | 'failed'
}

// ---------- игра: статистика (§18.3, B5) ----------

export interface FocusDayStat {
  day: string
  sessions: number
  workSec: number
  setupSec: number
  breakSec: number
  tasksDone: number
  distractions: number
  gaveIn: number
}

export interface FocusStats {
  from: string
  to: string
  days: FocusDayStat[]
  totals: FocusDayStat
  triggers: { trigger: string; total: number; gaveIn: number }[]
  tasksDone: { id: number; title: string; day: string }[]
  sessions: { id: number; taskTitle: string; day: string; outcome: FocusOutcome; workSec: number; distractions: number; gaveIn: number }[]
  records: {
    totalWorkSec: number
    totalTasks: number
    bestDay: { day: string; workSec: number } | null
    bestWeek: { day: string; workSec: number } | null // day — понедельник
  }
}

// ---------- спорт (§18.4, §18.5.7) ----------

/** Упражнение в тренировке: подходы × повторы × вес (вес необязателен — подтягивания). */
export interface WorkoutEntry {
  exerciseId: number
  name: string
  sets: number
  reps: number
  weight: number | null // кг
  tonnage: number
}

export interface Workout {
  id: number
  day: string
  title: string
  notes: string
  durationMin: number | null
  kcal: number | null
  exercises: WorkoutEntry[]
  tonnage: number
  createdAt: string
  updatedAt: string
}

export interface WorkoutInput {
  day: string
  title: string
  notes: string
  durationMin: number | null
  kcal: number | null
  exercises: { name: string; sets: number; reps: number; weight: number | null }[]
}

export interface Exercise {
  id: number
  name: string
  uses: number
  lastDay: string | null
  last: { sets: number; reps: number; weight: number | null } | null // как делали в прошлый раз — значения по умолчанию
}

// ---------- спорт: еда, статистика, прогресс (§18.4, C2) ----------

export interface Meal {
  text: string
  kcal: number | null // не знаю сколько
}

export interface FoodDay {
  day: string
  meals: Meal[]
  eatenKcal: number
  burnedKcal: number // на тренировках (с часов) — показываем рядом, без «баланса»
}

export interface SportDayStat {
  day: string
  workouts: number
  burnedKcal: number
  durationMin: number
  tonnage: number
  eatenKcal: number | null // null — еду не записывали
}

export interface SportStats {
  from: string
  to: string
  days: SportDayStat[]
  totals: {
    workouts: number
    workoutDays: number
    burnedKcal: number
    durationMin: number
    tonnage: number
    eatenKcal: number
    foodDays: number
    avgBurned: number | null
    avgDuration: number | null
    avgTonnage: number | null
    avgEatenPerDay: number | null
    avgBurnedPerDay: number | null
  }
  exercises: { id: number; name: string; workouts: number; sets: number; reps: number; maxWeight: number | null; tonnage: number }[]
}

export interface ProgressPoint {
  day: string
  maxWeight: number | null
  tonnage: number
  sets: number
  reps: number
}

// Life wishes and projects (v4); daily tasks remain independent.
export interface AmbitionFields {
  title: string
  note: string
  deadline: string | null
  position: number
}
export interface DreamCategory { id: number; title: string; position: number }
export interface Dream extends AmbitionFields {
  id: number
  status: 'want' | 'active' | 'fulfilled'
  categoryId: number | null
  projectId: number | null
}
export interface BigProject extends AmbitionFields { id: number }
export interface ProjectStep extends AmbitionFields {
  id: number
  projectId: number
  parentId: number | null
  done: boolean
}
export type AmbitionKind = 'categories' | 'dreams' | 'projects' | 'steps'
export interface AmbitionInput extends AmbitionFields {
  status?: Dream['status']
  categoryId?: number | null
  projectId?: number | null
  parentId?: number | null
  done?: boolean
  requires?: number[]
}
export interface Ambitions {
 timeline?: { years: number[]; goals: TimelineGoal[] }
  categories: DreamCategory[]
  dreams: Dream[]
  projects: BigProject[]
  steps: ProjectStep[]
  dependencies: { stepId: number; requiresId: number }[]
  today: string
  states: Record<number, { done: boolean; leaf: boolean; ready: boolean; blockedBy: number[]; overdue: boolean; progress: number }>
  summaries: Record<number, { progress: number; done: number; open: number; overdue: number; next: number[] }>
}

export interface TimelineGoal { id: number; year: number; month: number | null; title: string; note: string; dreamId: number | null }
export type TimelineInput = Omit<TimelineGoal, 'id'>
export type StudyKind = 'definition' | 'theorem' | 'remark' | 'property' | 'lemma'
export interface StudyGroup { id: number; title: string; parentId: number | null; grammar?: boolean }
interface StudyCardFields { id: number; groupId: number; front: string; back: string }
export interface StudyPracticeMaterial extends StudyCardFields { kind: StudyKind }
export interface GrammarExample { text: string; translation: string }
export interface GrammarMistake { incorrect: string; correct: string }
export interface GrammarContent { formula: string; examples?: GrammarExample[]; mistakes?: GrammarMistake[] }
export interface GrammarRule extends StudyCardFields { kind: 'grammar_rule'; content: GrammarContent }
export type StudyCard = StudyPracticeMaterial | GrammarRule
export interface StudyData { groups: StudyGroup[]; cards: StudyCard[] }
export interface StudyParse { cards: (StudyCard & {record: number})[]; errors: {record: number; line: number; code: string}[] }

export interface StudyInputs {
  groups: Omit<StudyGroup, 'id'>
  cards: Omit<StudyPracticeMaterial, 'id'> | Omit<GrammarRule, 'id'>
}

export interface StudyReviewInput { id: string; cardId: number; known: boolean; reverse: boolean }
export interface StudyReviewDay { day: string; total: number; known: number }
export interface StudyStats { days: StudyReviewDay[]; total: number; known: number; activeDays: number; bestStreak: number }

export interface StudyKnowledge { cardId: number; groupId: number; front: string; kind: StudyKind; groupTitle: string; score: number; total: number; known: number; lastReviewedAt: string | null }
export interface StudyPracticeCard extends StudyPracticeMaterial { score: number }
export interface StudyPracticeInput { groupId: number | null; cardIds: number[]; limit: number }
export interface StudySelection { cards: StudyPracticeCard[]; available: number }
