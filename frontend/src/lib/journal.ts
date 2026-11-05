// Чистая логика раздела «День» (§17.2) — без DOM, тестируется в journal.test.ts.
import type { DaySummary } from './api/types'

export const DOPAMINE_SLOTS = 7
export const MAX_DOPAMINE_LEN = 140 // как journal.MaxDopamineLen на бэке
export const MAX_WIN_LEN = 200

/** Тон оценки дня для цвета: 1–3 плохо, 4–6 так себе, 7–10 хорошо. */
export function ratingTone(r: number | null): 'red' | 'amber' | 'green' | 'none' {
  if (r === null) return 'none'
  if (r <= 3) return 'red'
  if (r <= 6) return 'amber'
  return 'green'
}

/** Новый массив, где элемент i сдвинут на dir (−1 вверх, +1 вниз); за край — без изменений. */
export function move<T>(list: readonly T[], i: number, dir: -1 | 1): T[] {
  const j = i + dir
  const out = [...list]
  if (i < 0 || i >= list.length || j < 0 || j >= list.length) return out
  ;[out[i], out[j]] = [out[j]!, out[i]!]
  return out
}

/** День, в котором хоть что-то записано. */
export function isFilled(d: DaySummary): boolean {
  return d.rating !== null || d.dopamineLevel > 0 || d.wins > 0 || d.hasStory
}

export interface PeriodSummary {
  filled: number // дней с записями
  avgRating: number | null // по оценённым дням
  avgDopamine: number | null // по дням с записями: пустой день — «не вёл», а не «ноль дофамина»
}

/** Сводка периода по дням не позже upTo (будущие дни месяца не считаем). */
export function summarize(days: readonly DaySummary[], upTo: string): PeriodSummary {
  const filled = days.filter((d) => d.day <= upTo && isFilled(d))
  const rated = filled.filter((d) => d.rating !== null)
  const avg = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null)
  return {
    filled: filled.length,
    avgRating: avg(rated.map((d) => d.rating!)),
    avgDopamine: avg(filled.map((d) => d.dopamineLevel)),
  }
}

/**
 * Лучший день периода: максимальная оценка, при равенстве — больше дофамина,
 * дальше — более поздний (свежий). Без оценённых дней — null.
 */
export function bestDay(days: readonly DaySummary[]): DaySummary | null {
  let best: DaySummary | null = null
  for (const d of days) {
    if (d.rating === null) continue
    if (
      !best ||
      d.rating > best.rating! ||
      (d.rating === best.rating && (d.dopamineLevel > best.dopamineLevel || (d.dopamineLevel === best.dopamineLevel && d.day > best.day)))
    ) {
      best = d
    }
  }
  return best
}
