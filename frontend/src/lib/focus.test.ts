import { describe, expect, it } from 'vitest'
import { currentBreakSec, formatClock, ritualDone, timeline } from './focus'

const t0 = Date.parse('2026-11-05T10:00:00Z')
const at = (min: number) => new Date(t0 + min * 60_000).toISOString()
const now = (min: number) => t0 + min * 60_000
const base = { startedAt: at(0), workStartedAt: null, finishedAt: null, breaks: [] }

// Те же случаи, что в Go TestCompute: фронт и бэк должны считать одинаково.
describe('timeline', () => {
  it('setup', () => {
    expect(timeline(base, now(4))).toMatchObject({ phase: 'setup', setupSec: 240, workSec: 0 })
  })
  it('work 65 min — 10 min of break earned', () => {
    expect(timeline({ ...base, workStartedAt: at(10) }, now(75))).toEqual({
      phase: 'work',
      setupSec: 600,
      workSec: 65 * 60,
      breakSec: 0,
      breakEarnedSec: 600,
      breakAvailableSec: 600,
      breakOverrunSec: 0,
    })
  })
  it('on a break: break time is not work', () => {
    const s = { ...base, workStartedAt: at(10), breaks: [{ startedAt: at(70), endedAt: null }] }
    expect(timeline(s, now(73))).toMatchObject({ phase: 'break', workSec: 3600, breakSec: 180, breakAvailableSec: 420 })
    expect(currentBreakSec(s, now(73))).toBe(180)
  })
  it('break overrun', () => {
    const s = { ...base, workStartedAt: at(0), breaks: [{ startedAt: at(30), endedAt: at(42) }] }
    expect(timeline(s, now(50))).toMatchObject({ phase: 'work', workSec: 38 * 60, breakEarnedSec: 300, breakOverrunSec: 420, breakAvailableSec: 0 })
    expect(currentBreakSec(s, now(50))).toBe(0)
  })
  it('finished: counted up to finish, not now', () => {
    const s = { ...base, workStartedAt: at(10), finishedAt: at(100), breaks: [{ startedAt: at(40), endedAt: at(45) }] }
    expect(timeline(s, now(500))).toMatchObject({ phase: 'finished', workSec: 85 * 60, breakSec: 300 })
  })
  it('clock skew never gives negative time', () => {
    expect(timeline(base, now(-5)).setupSec).toBe(0)
  })
})

describe('formatClock', () => {
  it.each([
    [0, '0:00'],
    [75, '1:15'],
    [600, '10:00'],
    [3725, '1:02:05'],
    [-3, '0:00'],
  ])('%i → %s', (s, want) => {
    expect(formatClock(s)).toBe(want)
  })
})

describe('ritualDone', () => {
  it.each([
    { items: 0, checked: [], want: true },
    { items: 3, checked: [0, 2], want: false },
    { items: 3, checked: [2, 0, 1], want: true },
    { items: 2, checked: [0, 1, 5], want: true },
  ])('$items / $checked', ({ items, checked, want }) => {
    expect(ritualDone(items, checked)).toBe(want)
  })
})
