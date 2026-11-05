import { describe, expect, it } from 'vitest'
import type { DaySummary } from './api/types'
import { bestDay, isFilled, move, ratingTone, summarize } from './journal'

describe('ratingTone', () => {
  it.each([
    [null, 'none'],
    [1, 'red'],
    [3, 'red'],
    [4, 'amber'],
    [6, 'amber'],
    [7, 'green'],
    [10, 'green'],
  ] as const)('%s → %s', (r, want) => {
    expect(ratingTone(r)).toBe(want)
  })
})

describe('move', () => {
  const list = ['a', 'b', 'c']
  it.each([
    [1, -1, ['b', 'a', 'c']],
    [1, 1, ['a', 'c', 'b']],
    [0, -1, ['a', 'b', 'c']], // за верхний край — без изменений
    [2, 1, ['a', 'b', 'c']], // за нижний край
    [5, -1, ['a', 'b', 'c']], // индекс вне списка
  ] as const)('move(%i, %i)', (i, dir, want) => {
    expect(move(list, i, dir)).toEqual(want)
  })
  it('does not mutate the input', () => {
    move(list, 0, 1)
    expect(list).toEqual(['a', 'b', 'c'])
  })
})

const day = (d: string, p: Partial<DaySummary> = {}): DaySummary => ({
  day: d,
  rating: null,
  dopamineLevel: 0,
  wins: 0,
  hasStory: false,
  ...p,
})

describe('isFilled', () => {
  it.each([
    [day('2026-11-01'), false],
    [day('2026-11-01', { rating: 1 }), true],
    [day('2026-11-01', { dopamineLevel: 1 }), true],
    [day('2026-11-01', { wins: 1 }), true],
    [day('2026-11-01', { hasStory: true }), true],
  ])('%j → %s', (d, want) => {
    expect(isFilled(d)).toBe(want)
  })
})

describe('summarize', () => {
  it('empty period', () => {
    expect(summarize([day('2026-11-01'), day('2026-11-02')], '2026-11-30')).toEqual({ filled: 0, avgRating: null, avgDopamine: null })
  })
  it('averages over filled days only, ignores days after upTo', () => {
    const days = [
      day('2026-11-01', { rating: 8, dopamineLevel: 4 }),
      day('2026-11-02'), // пустой — не тянет средние вниз
      day('2026-11-03', { dopamineLevel: 2, hasStory: true }), // без оценки — только в дофамин
      day('2026-11-04', { rating: 6, dopamineLevel: 6 }),
      day('2026-11-05', { rating: 1, dopamineLevel: 7 }), // после upTo
    ]
    expect(summarize(days, '2026-11-04')).toEqual({ filled: 3, avgRating: 7, avgDopamine: 4 })
  })
})

describe('bestDay', () => {
  it('highest rating wins', () => {
    expect(bestDay([day('2026-11-01', { rating: 6 }), day('2026-11-02', { rating: 9 }), day('2026-11-03', { rating: 7 })])?.day).toBe('2026-11-02')
  })
  it('tie on rating → more dopamine, then the later day', () => {
    const days = [
      day('2026-11-01', { rating: 8, dopamineLevel: 5 }),
      day('2026-11-02', { rating: 8, dopamineLevel: 3 }),
      day('2026-11-03', { rating: 8, dopamineLevel: 5 }),
    ]
    expect(bestDay(days)?.day).toBe('2026-11-03')
  })
  it('no rated days → null', () => {
    expect(bestDay([day('2026-11-01', { dopamineLevel: 7 })])).toBeNull()
    expect(bestDay([])).toBeNull()
  })
})
