import { describe, expect, it } from 'vitest'
import type { TrainingCard } from './api/types'
import { answer, createQueue, current, finished, progress } from './training'

const card = (id: number, extra: Partial<TrainingCard> = {}): TrainingCard => ({
  wordId: id,
  term: `w${id}`,
  translation: `t${id}`,
  level: 2,
  ...extra,
})
const ids = (s: { queue: TrainingCard[] }) => s.queue.map((c) => c.wordId)

describe('queue', () => {
  it('good/hard remove the card and count as done', () => {
    let s = createQueue([card(1), card(2), card(3)])
    s = answer(s, 'good')
    expect(ids(s)).toEqual([2, 3])
    s = answer(s, 'hard')
    expect(ids(s)).toEqual([3])
    expect(s.done).toBe(2)
    expect(progress(s)).toBeCloseTo(2 / 3)
  })

  it.each([
    { requeueAt: 3, want: [2, 3, 4, 1, 5, 6] },
    { requeueAt: 5, want: [2, 3, 4, 5, 6, 1] },
    { requeueAt: 10, want: [2, 3, 4, 5, 6, 1] }, // осталось меньше — в конец
    { requeueAt: undefined, want: [2, 3, 4, 1, 5, 6] }, // по умолчанию 3
  ])('again with requeueAt=$requeueAt', ({ requeueAt, want }) => {
    const s = answer(createQueue([1, 2, 3, 4, 5, 6].map((i) => card(i))), 'again', requeueAt)
    expect(ids(s)).toEqual(want)
    expect(s.done).toBe(0)
  })

  it('again does not move progress backwards or forwards', () => {
    let s = createQueue([card(1), card(2)])
    s = answer(s, 'good')
    const before = progress(s)
    s = answer(s, 'again', 4)
    expect(progress(s)).toBe(before)
    expect(s.done + s.queue.length).toBe(s.total)
  })

  it('last card answered "again" comes right back until known', () => {
    let s = createQueue([card(1)])
    s = answer(s, 'again', 4)
    expect(current(s)?.wordId).toBe(1)
    expect(finished(s)).toBe(false)
    s = answer(s, 'good')
    expect(finished(s)).toBe(true)
    expect(progress(s)).toBe(1)
  })

  it('does not mutate the previous state', () => {
    const s0 = createQueue([card(1), card(2), card(3), card(4)])
    answer(s0, 'again', 3)
    expect(ids(s0)).toEqual([1, 2, 3, 4])
  })

  it('empty queue: answer is a no-op, progress is complete', () => {
    const s = createQueue([])
    expect(answer(s, 'good')).toBe(s)
    expect(progress(s)).toBe(1)
    expect(finished(s)).toBe(true)
  })
})
