import { describe, expect, it } from 'vitest'
import type { Checkin, Habit } from './api/types'
import { dayDue, habitsDue } from './reminders'

describe('dayDue', () => {
  it.each([
    [17, 59, false, false],
    [18, 0, false, true],
    [23, 30, false, true],
    [20, 0, true, false], // уже оценён
    [9, 0, false, false],
  ])('%i:%i rated=%s → %s', (h, m, rated, want) => {
    expect(dayDue(new Date(2026, 10, 5, h, m), rated)).toBe(want)
  })
})

describe('habitsDue', () => {
  const habit = (id: number, p: Partial<Habit> = {}): Habit => ({
    id,
    name: `h${id}`,
    kind: 'positive',
    color: null,
    trackTrigger: false,
    trackRating: false,
    trackAmount: false,
    unit: '',
    archived: false,
    startDay: '2026-11-01',
    createdAt: '',
    ...p,
  })
  const checkin = (habitId: number): Checkin => ({ habitId, day: '2026-11-05', trigger: null, rating: null, amount: null })

  it('counts active positive habits without a check-in', () => {
    const habits = [habit(1), habit(2), habit(3, { kind: 'negative' }), habit(4, { archived: true })]
    expect(habitsDue(habits, [])).toBe(2)
    expect(habitsDue(habits, [checkin(1)])).toBe(1)
    expect(habitsDue(habits, [checkin(1), checkin(2), checkin(3)])).toBe(0)
  })
  it('no habits → 0', () => {
    expect(habitsDue([], [])).toBe(0)
  })
})
