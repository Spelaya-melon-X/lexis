// Раскладка календаря активности (§8, heatmap): недели — колонки, дни Пн..Вс — строки.
import type { DayStats } from './api/types'
import { parseDay } from './dates'

export interface Cell {
  day: DayStats
  level: 0 | 1 | 2 | 3 | 4 // интенсивность цвета
}

/** Понедельник = 0 … воскресенье = 6 (Date.getDay считает с воскресенья). */
export function weekday(day: string): number {
  return (parseDay(day).getDay() + 6) % 7
}

/**
 * Интенсивность 0..4 относительно самого активного дня окна. 0 — только у дня без слов:
 * любой день с занятием виден хотя бы бледной клеткой, даже на фоне «рекордного».
 */
export function intensity(words: number, max: number): Cell['level'] {
  if (words <= 0 || max <= 0) return 0
  return Math.min(4, Math.max(1, Math.ceil((words / max) * 4))) as Cell['level']
}

/**
 * Колонки-недели: в каждой 7 ячеек (Пн..Вс); дни до начала окна в первой неделе — null.
 * Вход — дни подряд без пропусков (так отдаёт /api/stats/calendar).
 */
export function weeks(days: DayStats[]): (Cell | null)[][] {
  if (days.length === 0) return []
  const max = Math.max(...days.map((d) => d.words))
  const cols: (Cell | null)[][] = []
  let col: (Cell | null)[] = Array(weekday(days[0]!.day)).fill(null)
  for (const d of days) {
    col.push({ day: d, level: intensity(d.words, max) })
    if (col.length === 7) {
      cols.push(col)
      col = []
    }
  }
  if (col.length > 0) cols.push(col)
  return cols
}

/** Подписи месяцев: колонка, в которой встречается 1-е число (или первая колонка). */
export function monthLabels(cols: (Cell | null)[][]): { col: number; day: string }[] {
  const out: { col: number; day: string }[] = []
  cols.forEach((col, i) => {
    const first = col.find((c) => c !== null)
    const firstOfMonth = col.find((c) => c !== null && c.day.day.endsWith('-01'))
    if (firstOfMonth) out.push({ col: i, day: firstOfMonth.day.day })
    else if (i === 0 && first) out.push({ col: 0, day: first.day.day })
  })
  // Первая подпись наезжает на следующую, если месяц начался через 1–2 недели — убираем её.
  if (out.length > 1 && out[1]!.col - out[0]!.col < 3) out.shift()
  return out
}
