// Тренировки (§18.4, §18.5.7): черновик формы — упражнение одной строкой «подходы × повторы × вес»,
// значения по умолчанию — «как в прошлый раз»; итоги недели. Поля ввода — строки, разбор — при сохранении.

import type { Exercise, Meal, Workout, WorkoutInput } from './api/types'
import { parseAmount } from './habits'

export interface EntryDraft {
  name: string
  sets: string
  reps: string
  weight: string // пусто — без веса
  touched: boolean // числа меняли руками — выбор знакомого упражнения их уже не перезапишет
}

/** Новая строка: значения по умолчанию, которые обычно и правят. */
export function newEntry(): EntryDraft {
  return { name: '', sets: '3', reps: '10', weight: '', touched: false }
}

/** Найти упражнение справочника по имени без учёта регистра и лишних пробелов. */
export function findExercise(list: Exercise[], name: string): Exercise | undefined {
  const key = name.trim().replace(/\s+/g, ' ').toLowerCase()
  return key ? list.find((e) => e.name.toLowerCase() === key) : undefined
}

/** Выбрали знакомое упражнение — подставить «как в прошлый раз» (если числа ещё не трогали). */
export function withLast(d: EntryDraft, ex: Exercise | undefined): EntryDraft {
  if (!ex?.last || d.touched) return d
  return { ...d, sets: String(ex.last.sets), reps: String(ex.last.reps), weight: ex.last.weight === null ? '' : String(ex.last.weight) }
}

const int = (s: string): number | null => (/^\d+$/.test(s.trim()) ? Number(s.trim()) : null)

/** Черновик → тело API. Пустые строки (без имени) пропускаются; ошибка — номер строки, которую бэк не примет. */
export function toInput(
  base: { day: string; title: string; notes: string; durationMin: string; kcal: string },
  entries: EntryDraft[],
): WorkoutInput | { error: 'invalid_set' | 'invalid_duration' | 'invalid_kcal'; entry?: number } {
  const durationMin = base.durationMin.trim() === '' ? null : int(base.durationMin)
  if (base.durationMin.trim() !== '' && durationMin === null) return { error: 'invalid_duration' }
  const kcal = base.kcal.trim() === '' ? null : int(base.kcal)
  if (base.kcal.trim() !== '' && kcal === null) return { error: 'invalid_kcal' }

  const exercises: WorkoutInput['exercises'] = []
  for (const [i, e] of entries.entries()) {
    if (!e.name.trim()) continue
    const sets = int(e.sets)
    const reps = int(e.reps)
    const weight = e.weight.trim() === '' ? null : parseAmount(e.weight)
    if (!sets || !reps || (e.weight.trim() !== '' && weight === null)) return { error: 'invalid_set', entry: i }
    exercises.push({ name: e.name.trim(), sets, reps, weight })
  }
  return { day: base.day, title: base.title, notes: base.notes, durationMin, kcal, exercises }
}

/** Тренировка → строки формы. */
export function draftOf(w: Workout): EntryDraft[] {
  return w.exercises.map((e) => ({
    name: e.name,
    sets: String(e.sets),
    reps: String(e.reps),
    weight: e.weight === null ? '' : String(e.weight),
    touched: true,
  }))
}

/** Тоннаж строки: подходы × повторы × вес (без веса — 0). */
export function entryTonnage(e: Pick<EntryDraft, 'sets' | 'reps' | 'weight'>): number {
  const sets = int(e.sets)
  const reps = int(e.reps)
  const kg = parseAmount(e.weight)
  return sets && reps && kg !== null ? sets * reps * kg : 0
}

/** Тонны с одним знаком от тонны и выше, иначе килограммы — «820 кг», «1,2 т». */
export function weightParts(kg: number): { value: number; unit: 'kg' | 't' } {
  return kg >= 1000 ? { value: Math.round(kg / 100) / 10, unit: 't' } : { value: Math.round(kg), unit: 'kg' }
}

export interface DayRow {
  day: string
  workouts: number
  kcal: number
  durationMin: number
  tonnage: number
}

export interface ExerciseRow {
  name: string
  times: number // в скольких тренировках
  sets: number
  reps: number // всего повторов: Σ подходы × повторы
  maxWeight: number | null
  tonnage: number
}

export interface WeekSummary {
  workouts: number
  kcal: number
  avgKcal: number | null // на тренировку — только по тем, где ккал записаны
  durationMin: number
  avgDuration: number | null
  tonnage: number
  avgTonnage: number | null // на тренировку с весами
  days: DayRow[] // все дни недели, включая пустые
  exercises: ExerciseRow[] // по тоннажу, потом по числу раз
}

const avg = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null)

/** Итоги недели (§18.5.7): сколько потратил, какие упражнения, сколько поднято, средние, таблица по дням. */
export function weekSummary(workouts: Workout[], days: string[]): WeekSummary {
  const rows = new Map<string, DayRow>(days.map((d) => [d, { day: d, workouts: 0, kcal: 0, durationMin: 0, tonnage: 0 }]))
  const ex = new Map<string, ExerciseRow>()
  for (const w of workouts) {
    const r = rows.get(w.day)
    if (r) {
      r.workouts++
      r.kcal += w.kcal ?? 0
      r.durationMin += w.durationMin ?? 0
      r.tonnage += w.tonnage
    }
    for (const e of w.exercises) {
      const row = ex.get(e.name) ?? { name: e.name, times: 0, sets: 0, reps: 0, maxWeight: null, tonnage: 0 }
      row.times++
      row.sets += e.sets
      row.reps += e.sets * e.reps
      row.tonnage += e.tonnage
      if (e.weight !== null) row.maxWeight = Math.max(row.maxWeight ?? 0, e.weight)
      ex.set(e.name, row)
    }
  }
  const withKcal = workouts.filter((w) => w.kcal !== null).map((w) => w.kcal!)
  const withDuration = workouts.filter((w) => w.durationMin !== null).map((w) => w.durationMin!)
  const withTonnage = workouts.filter((w) => w.tonnage > 0).map((w) => w.tonnage)
  return {
    workouts: workouts.length,
    kcal: withKcal.reduce((a, b) => a + b, 0),
    avgKcal: avg(withKcal),
    durationMin: withDuration.reduce((a, b) => a + b, 0),
    avgDuration: avg(withDuration),
    tonnage: workouts.reduce((a, w) => a + w.tonnage, 0),
    avgTonnage: avg(withTonnage),
    days: [...rows.values()],
    exercises: [...ex.values()].sort((a, b) => b.tonnage - a.tonnage || b.times - a.times || a.name.localeCompare(b.name)),
  }
}

/** Подсказка еды по тексту (без учёта регистра и пробелов) — чтобы подставить её ккал. */
export function findMeal(suggestions: Meal[], text: string): Meal | undefined {
  const key = text.trim().replace(/\s+/g, ' ').toLowerCase()
  return key ? suggestions.find((m) => m.text.toLowerCase() === key) : undefined
}

/** Сумма известных ккал (пункты без ккал не считаются) — как sport.SumKcal на бэке. */
export function sumKcal(meals: Meal[]): number {
  return meals.reduce((sum, m) => sum + (m.kcal ?? 0), 0)
}
