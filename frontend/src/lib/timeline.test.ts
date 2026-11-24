import { expect, it } from 'vitest'
import { timelineYears, goalsFor } from './timeline'
import type { TimelineGoal } from './api/types'
it('uses the server civil year, ten years inclusively and retained extensions', () => {
  expect(timelineYears('2026-01-01', [], [])).toEqual([
    2026, 2027, 2028, 2029, 2030, 2031, 2032, 2033, 2034, 2035,
  ])
  const goal: TimelineGoal = {
    id: 1,
    title: 'Goal',
    note: '',
    year: 2025,
    month: 1,
    dreamId: null,
  }
  expect(timelineYears('2026-01-01', [2036, 2036], [goal])).toEqual([
    2025, 2026, 2027, 2028, 2029, 2030, 2031, 2032, 2033, 2034, 2035, 2036,
  ])
  expect(timelineYears('9999-12-31', [], [])).toEqual([9999])
  expect(timelineYears('', [], [])).toEqual([])
})
it('keeps annual goals distinct from all twelve monthly periods', () => {
  const goals: TimelineGoal[] = Array.from({ length: 13 }, (_, i) => ({
    id: i + 1,
    title: 'Goal',
    note: '',
    year: 2026,
    month: i === 0 ? null : i,
    dreamId: null,
  }))
  expect(goalsFor(goals, 2026, null).map((g) => g.id)).toEqual([1])
  for (let i = 1; i <= 12; i++)
    expect(goalsFor(goals, 2026, i).map((g) => g.id)).toEqual([i + 1])
  expect(goalsFor(goals, 2027, null)).toEqual([])
})
