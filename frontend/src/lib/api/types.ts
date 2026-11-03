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
      restored: { groups: number; words: number; sessions: number; reviews: number; achievements: number }
    }
