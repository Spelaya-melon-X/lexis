import { describe, expect, it } from 'vitest'
import type { Goal, Habit } from './api/types'
import { cellState, daysRange, draftOf, formatAmount, goalSpans, parseAmount, sameDraft, toInput, weekStart, type Draft } from './habits'

describe('cellState', () => {
  const today = '2026-11-05'
  const start = '2026-11-02'
  it.each([
    ['positive', '2026-11-03', true, 'done'],
    ['positive', '2026-11-03', false, 'empty'],
    ['negative', '2026-11-03', true, 'slip'],
    ['negative', '2026-11-03', false, 'clean'],
    ['negative', '2026-11-05', false, 'clean'], // сегодня без срыва — пока держится
    ['negative', '2026-11-01', false, 'before'], // до создания — не «удержался»
    ['positive', '2026-11-01', true, 'done'], // отметка задним числом раньше создания
    ['positive', '2026-11-06', false, 'future'],
    ['negative', '2026-11-06', true, 'future'],
  ] as const)('%s %s checked=%s → %s', (kind, day, checked, want) => {
    expect(cellState(kind, start, day, checked, today)).toBe(want)
  })
})

describe('weekStart', () => {
  it.each([
    ['2026-11-02', '2026-11-02'], // понедельник
    ['2026-11-05', '2026-11-02'],
    ['2026-11-08', '2026-11-02'], // воскресенье
    ['2026-11-01', '2026-10-26'], // воскресенье, неделя через границу месяца
    ['2027-01-01', '2026-12-28'],
  ])('%s → %s', (day, want) => {
    expect(weekStart(day)).toBe(want)
  })
})

describe('daysRange', () => {
  it('inclusive, across months', () => {
    expect(daysRange('2026-10-30', '2026-11-02')).toEqual(['2026-10-30', '2026-10-31', '2026-11-01', '2026-11-02'])
  })
  it('single day and empty', () => {
    expect(daysRange('2026-11-02', '2026-11-02')).toEqual(['2026-11-02'])
    expect(daysRange('2026-11-03', '2026-11-02')).toEqual([])
  })
})

describe('goalSpans', () => {
  const g = (id: number, from: string, to: string): Goal => ({ id, habitId: 1, title: '', from, to, progress: null })
  const week = daysRange('2026-11-02', '2026-11-08')
  it('clips to the window and skips non-overlapping', () => {
    const spans = goalSpans([g(1, '2026-11-01', '2026-11-09'), g(2, '2026-11-04', '2026-11-05'), g(3, '2026-10-01', '2026-11-01'), g(4, '2026-11-08', '2026-11-20')], week)
    expect(spans.map((s) => [s.goal.id, s.start, s.end])).toEqual([
      [1, 0, 6],
      [2, 2, 3],
      [4, 6, 6],
    ])
  })
  it('empty window', () => {
    expect(goalSpans([g(1, '2026-11-01', '2026-11-09')], [])).toEqual([])
  })
})

describe('amounts', () => {
  it.each([
    ['5', 5],
    [' 5,5 ', 5.5],
    ['5.25', 5.25],
    ['0', 0],
    ['', null],
    ['-1', null],
    ['5 км', null],
    ['1e3', null],
  ])('parseAmount(%j) = %s', (s, want) => {
    expect(parseAmount(s)).toBe(want)
  })
  it('formatAmount rounds to 2 digits', () => {
    expect(formatAmount(5.256, 'en')).toBe('5.26')
    expect(formatAmount(5, 'en')).toBe('5')
  })
})

describe('check form', () => {
  const habit = (p: Partial<Habit> = {}): Habit => ({
    id: 1,
    name: 'Бег',
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
  const d = (p: Partial<Draft> = {}): Draft => ({ done: true, trigger: '', rating: null, amount: '', ...p })

  it('draftOf: no checkin → empty, checkin → its values', () => {
    expect(draftOf(undefined)).toEqual({ done: false, trigger: '', rating: null, amount: '' })
    expect(draftOf({ habitId: 1, day: '', trigger: 'стресс', rating: 7, amount: 5.5 })).toEqual({ done: true, trigger: 'стресс', rating: 7, amount: '5.5' })
  })

  it.each([
    ['undone', habit({ trackAmount: true }), d({ done: false, amount: '5' }), { habitId: 1, done: false }],
    ['plain', habit(), d({ trigger: 'x', rating: 5, amount: '3' }), { habitId: 1, done: true }], // выключенные поля не шлём
    ['all fields', habit({ trackTrigger: true, trackRating: true, trackAmount: true }), d({ trigger: ' стресс ', rating: 5, amount: '5,5' }),
      { habitId: 1, done: true, trigger: 'стресс', rating: 5, amount: 5.5 }],
    ['blank trigger, no rating', habit({ trackTrigger: true, trackRating: true }), d({ trigger: '  ' }), { habitId: 1, done: true }],
    ['amount required', habit({ trackAmount: true }), d(), { error: 'amount_required' }],
    ['bad amount', habit({ trackAmount: true }), d({ amount: 'много' }), { error: 'invalid_amount' }],
  ] as const)('toInput: %s', (_, h, draft, want) => {
    expect(toInput(h, draft)).toEqual(want)
  })

  it('sameDraft ignores surrounding spaces in amount', () => {
    expect(sameDraft(d({ amount: '5' }), d({ amount: ' 5 ' }))).toBe(true)
    expect(sameDraft(d(), d({ done: false }))).toBe(false)
    expect(sameDraft(d({ rating: 1 }), d({ rating: 2 }))).toBe(false)
  })
})
