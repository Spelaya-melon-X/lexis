// Чистая логика раздела «Привычки» (§17.4) — без DOM, тестируется в habits.test.ts.
import type { Checkin, CheckinInput, Goal, Habit, HabitKind } from './api/types'
import { addDays, parseDay } from './dates'

/**
 * Состояние клетки календаря (Q4):
 * done — positive отмечена (зелёная), slip — negative отмечена = сорвался (красная),
 * clean — прошедший день negative без отметки = удержался (бледно-зелёная),
 * empty — positive без отметки, before — день до создания привычки, future — ещё не наступил.
 */
export type CellState = 'done' | 'slip' | 'clean' | 'empty' | 'before' | 'future'

export function cellState(kind: HabitKind, startDay: string, day: string, checked: boolean, today: string): CellState {
  if (day > today) return 'future'
  // Отметку задним числом раньше даты создания показываем как есть — она осознанная.
  if (checked) return kind === 'positive' ? 'done' : 'slip'
  if (day < startDay) return 'before'
  return kind === 'negative' ? 'clean' : 'empty'
}

/** Понедельник недели, в которую входит день. */
export function weekStart(day: string): string {
  return addDays(day, -((parseDay(day).getDay() + 6) % 7))
}

/** Все дни [from, to] включительно. */
export function daysRange(from: string, to: string): string[] {
  const out: string[] = []
  for (let d = from; d <= to; d = addDays(d, 1)) out.push(d)
  return out
}

/** Полоса цели в окне дней: индексы колонок start..end включительно (обрезана по окну). */
export interface GoalSpan {
  goal: Goal
  start: number
  end: number
}

export function goalSpans(goals: readonly Goal[], days: readonly string[]): GoalSpan[] {
  const first = days[0]
  const last = days.at(-1)
  if (!first || !last) return []
  const out: GoalSpan[] = []
  for (const goal of goals) {
    if (goal.to < first || goal.from > last) continue
    out.push({
      goal,
      start: goal.from <= first ? 0 : days.indexOf(goal.from),
      end: goal.to >= last ? days.length - 1 : days.indexOf(goal.to),
    })
  }
  return out
}

/** 5 → "5", 5.25 → "5,25" (ru) / "5.25" (en), не больше двух знаков после запятой. */
export function formatAmount(n: number, locale: string): string {
  return n.toLocaleString(locale, { maximumFractionDigits: 2 })
}

/** "5,5" / " 5.5 " → 5.5; пусто или мусор → null. Запятая — привычный десятичный разделитель в RU. */
export function parseAmount(s: string): number | null {
  const v = s.trim().replace(',', '.')
  if (!/^\d+(\.\d+)?$/.test(v)) return null
  return Number(v)
}

/** Черновик строки формы «Отметки»: строки ввода как есть, разбор — при сохранении. */
export interface Draft {
  done: boolean
  trigger: string
  rating: number | null
  amount: string
}

export function draftOf(c: Checkin | undefined, locale = 'en'): Draft {
  return {
    done: c !== undefined,
    trigger: c?.trigger ?? '',
    rating: c?.rating ?? null,
    amount: c?.amount != null ? formatAmount(c.amount, locale).replace(/\s/g, '') : '',
  }
}

export function sameDraft(a: Draft, b: Draft): boolean {
  return a.done === b.done && a.trigger === b.trigger && a.rating === b.rating && a.amount.trim() === b.amount.trim()
}

/**
 * Черновик → тело запроса. Шлём только включённые у привычки поля (иначе бэк ответит
 * field_not_enabled). Ошибка — код для i18n errors.<code>, как у бэка.
 */
export function toInput(h: Habit, d: Draft): CheckinInput | { error: 'amount_required' | 'invalid_amount' } {
  if (!d.done) return { habitId: h.id, done: false }
  const out: CheckinInput = { habitId: h.id, done: true }
  if (h.trackTrigger && d.trigger.trim()) out.trigger = d.trigger.trim()
  if (h.trackRating && d.rating !== null) out.rating = d.rating
  if (h.trackAmount) {
    if (!d.amount.trim()) return { error: 'amount_required' }
    const a = parseAmount(d.amount)
    if (a === null) return { error: 'invalid_amount' }
    out.amount = a
  }
  return out
}
