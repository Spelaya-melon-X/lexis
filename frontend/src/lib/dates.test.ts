import { describe, expect, it } from 'vitest'
import { formatDuration, parseDay } from './dates'

describe('parseDay', () => {
  it('local midnight of the given calendar day', () => {
    const d = parseDay('2026-11-02')
    expect([d.getFullYear(), d.getMonth(), d.getDate(), d.getHours()]).toEqual([2026, 10, 2, 0])
  })
  it('handles month/year boundaries', () => {
    const d = parseDay('2026-01-31')
    expect([d.getMonth(), d.getDate()]).toEqual([0, 31])
  })
})


describe('formatDuration', () => {
  it.each([
    [0, '0:00'],
    [5, '0:05'],
    [75, '1:15'],
    [600, '10:00'],
    [3725, '1:02:05'],
    [-3, '0:00'],
    [Number.NaN, '0:00'],
    [59.9, '0:59'],
  ])('%d → %s', (sec, want) => {
    expect(formatDuration(sec)).toBe(want)
  })
})
