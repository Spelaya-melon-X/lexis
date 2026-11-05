// Напоминания внутри приложения (§17.11, Q8) — чистая логика, тесты в reminders.test.ts.
import type { Checkin, Habit } from './api/types'

/** С этого часа (локальное время) неоценённый день подсвечивается точкой на «Дне». */
export const EVENING_HOUR = 18

/** Точка на «Дне»: вечер, а сегодняшний день ещё не оценён. */
export function dayDue(now: Date, rated: boolean): boolean {
  return !rated && now.getHours() >= EVENING_HOUR
}

/**
 * Сколько привычек сегодня ещё ждут отметки: активные позитивные без отметки.
 * Негативные не считаем — у них «нет отметки» = удержался, напоминать не о чем.
 */
export function habitsDue(habits: readonly Habit[], todayCheckins: readonly Checkin[]): number {
  const checked = new Set(todayCheckins.map((c) => c.habitId))
  return habits.filter((h) => !h.archived && h.kind === 'positive' && !checked.has(h.id)).length
}
