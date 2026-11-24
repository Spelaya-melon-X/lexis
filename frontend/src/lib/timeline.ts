import type { TimelineGoal } from './api/types'
/** today is the server's civil date in LEXIS_TZ, never the browser's timezone. */
export function timelineYears(
  today: string,
  added: number[],
  goals: TimelineGoal[],
): number[] {
  const current = Number(today.slice(0, 4))
  if (!Number.isInteger(current) || current < 1 || current > 9999) return []
  return [
    ...new Set([
      ...Array.from(
        { length: Math.min(10, 10000 - current) },
        (_, i) => current + i,
      ),
      ...added,
      ...goals.map((g) => g.year),
    ]),
  ].sort((a, b) => a - b)
}
export function goalsFor(
  goals: TimelineGoal[],
  year: number,
  month: number | null,
): TimelineGoal[] {
  return goals.filter((g) => g.year === year && g.month === month)
}
