import { describe, expect, it } from 'vitest'
import type { DayStats } from './api/types'
import { intensity, monthLabels, weekday, weeks } from './heatmap'

const ds = (day: string, words = 0): DayStats => ({ day, sessions: 0, words, good: 0, hard: 0, again: 0, seconds: 0, goodShare: null })

// Дни подряд, начиная с from (YYYY-MM-DD), — как отдаёт /api/stats/calendar.
function range(from: string, n: number, words: (i: number) => number = () => 0): DayStats[] {
  const [y, m, d] = from.split('-').map(Number)
  return Array.from({ length: n }, (_, i) => {
    const dt = new Date(y!, m! - 1, d! + i)
    const s = `${dt.getFullYear()}-${String(dt.getMonth() + 1).padStart(2, '0')}-${String(dt.getDate()).padStart(2, '0')}`
    return ds(s, words(i))
  })
}

describe('weekday', () => {
  it.each([
    ['2026-11-02', 0], // понедельник
    ['2026-11-04', 2],
    ['2026-11-08', 6], // воскресенье
  ])('%s → %d', (day, want) => {
    expect(weekday(day)).toBe(want)
  })
})

describe('intensity', () => {
  it.each([
    [0, 10, 0],
    [1, 10, 1], // любой день с занятием виден
    [3, 10, 2],
    [5, 10, 2],
    [6, 10, 3],
    [10, 10, 4],
    [5, 0, 0],
    [15, 10, 4], // больше максимума — прижимаем
  ])('words=%d max=%d → %d', (w, max, want) => {
    expect(intensity(w, max)).toBe(want)
  })
})

describe('weeks', () => {
  it('pads the first week so rows are weekdays', () => {
    const cols = weeks(range('2026-11-04', 10)) // среда
    expect(cols).toHaveLength(2)
    expect(cols[0]!.slice(0, 2)).toEqual([null, null])
    expect(cols[0]![2]!.day.day).toBe('2026-11-04')
    expect(cols[0]![6]!.day.day).toBe('2026-11-08') // воскресенье — последняя строка
    expect(cols[1]![0]!.day.day).toBe('2026-11-09') // понедельник — новая колонка
    expect(cols[1]).toHaveLength(5) // 9–13 ноября: последняя неделя неполная — без хвостовых null
  })

  it('levels are relative to the busiest day', () => {
    const cols = weeks(range('2026-11-02', 3, (i) => [0, 2, 8][i]!))
    expect(cols[0]!.map((c) => c?.level)).toEqual([0, 1, 4])
  })

  it('a full year gives 53 columns', () => {
    expect(weeks(range('2025-11-11', 365)).length).toBe(53)
  })

  it('empty → []', () => {
    expect(weeks([])).toEqual([])
  })
})

describe('monthLabels', () => {
  it('labels columns where a month starts', () => {
    const labels = monthLabels(weeks(range('2026-09-28', 40))) // пн 28 сен → 6 нояб.
    expect(labels.map((l) => l.day)).toEqual(['2026-10-01', '2026-11-01'])
    expect(labels[0]!.col).toBe(0)
  })

  it('first column gets a label if the window starts mid-month, unless it collides', () => {
    expect(monthLabels(weeks(range('2026-09-07', 60))).map((l) => l.day)).toEqual(['2026-09-07', '2026-10-01', '2026-11-01'])
    // 1 октября — во 2-й колонке: подпись «сентябрь» в 1-й наехала бы на неё
    expect(monthLabels(weeks(range('2026-09-21', 30))).map((l) => l.day)).toEqual(['2026-10-01'])
  })
})
