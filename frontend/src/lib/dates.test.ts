import { describe, expect, it } from 'vitest'
import { addDays, addMonths, formatDay, formatDuration, isDay, isMonth, monthGrid, parseDay, today } from './dates'

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

describe('formatDay / today', () => {
  it('uses the local calendar date, not UTC', () => {
    // 00:30 локального времени — в UTC-ISO это мог бы быть ещё вчерашний день.
    expect(formatDay(new Date(2026, 10, 3, 0, 30))).toBe('2026-11-03')
    expect(today(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05')
  })
})

describe('addDays', () => {
  it.each([
    ['2026-11-03', 1, '2026-11-04'],
    ['2026-11-03', -1, '2026-11-02'],
    ['2026-11-30', 1, '2026-12-01'],
    ['2026-12-31', 1, '2027-01-01'],
    ['2028-02-28', 1, '2028-02-29'],
    ['2026-03-29', 1, '2026-03-30'], // переход на летнее время в Европе
    ['2026-11-03', 0, '2026-11-03'],
  ])('%s %+d → %s', (day, n, want) => {
    expect(addDays(day, n)).toBe(want)
  })
})

describe('isDay / isMonth', () => {
  it.each([
    ['2026-11-03', true],
    ['2028-02-29', true],
    ['2026-02-30', false],
    ['2026-13-01', false],
    ['2026-1-3', false],
    ['', false],
    ['today', false],
  ])('isDay(%j) = %s', (s, want) => {
    expect(isDay(s)).toBe(want)
  })
  it.each([
    ['2026-11', true],
    ['2026-00', false],
    ['2026-13', false],
    ['2026-11-01', false],
  ])('isMonth(%j) = %s', (s, want) => {
    expect(isMonth(s)).toBe(want)
  })
})

describe('addMonths', () => {
  it.each([
    ['2026-11', 1, '2026-12'],
    ['2026-12', 1, '2027-01'],
    ['2026-01', -1, '2025-12'],
  ])('%s %+d → %s', (m, n, want) => {
    expect(addMonths(m, n)).toBe(want)
  })
})

describe('monthGrid', () => {
  it('November 2026 starts on Sunday', () => {
    const g = monthGrid('2026-11')
    expect(g[0]).toEqual([null, null, null, null, null, null, '2026-11-01'])
    expect(g.at(-1)).toEqual(['2026-11-30', null, null, null, null, null, null])
    expect(g).toHaveLength(6)
  })
  it('February 2027 fits exactly into 4 weeks (Mon 1st, 28 days)', () => {
    const g = monthGrid('2027-02')
    expect(g).toHaveLength(4)
    expect(g[0]![0]).toBe('2027-02-01')
    expect(g[3]![6]).toBe('2027-02-28')
  })
  it('every row has 7 cells and every day appears once', () => {
    const g = monthGrid('2028-02')
    expect(g.every((w) => w.length === 7)).toBe(true)
    expect(g.flat().filter(Boolean)).toHaveLength(29)
  })
})
