import { describe, expect, it } from 'vitest'
import type { TrainingCard } from './api/types'
import { answer, createQueue, current, faces, finished, matchedForms, progress } from './training'

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

const verb = (sentence: string, highlights: { start: number; end: number }[]): TrainingCard =>
  card(1, { term: 'go_went_gone', forms: ['go', 'went', 'gone'], translation: 'идти', example: { text: '', sentence, highlights } })

describe('matchedForms', () => {
  it.each([
    { name: 'one form highlighted', c: verb('I went home', [{ start: 2, end: 6 }]), want: ['went'] },
    { name: 'case-insensitive', c: verb('Went home', [{ start: 0, end: 4 }]), want: ['went'] },
    { name: 'two forms, kept in form order', c: verb('gone and go', [{ start: 0, end: 4 }, { start: 9, end: 11 }]), want: ['go', 'gone'] },
    { name: 'no match → all forms', c: verb('I am going', [{ start: 5, end: 10 }]), want: ['go', 'went', 'gone'] },
    { name: 'no highlights → all forms', c: verb('I went home', []), want: ['go', 'went', 'gone'] },
    { name: 'not a verb → []', c: card(2, { example: { text: '', sentence: 'An apple', highlights: [{ start: 3, end: 8 }] } }), want: [] },
  ])('$name', ({ c, want }) => {
    expect(matchedForms(c)).toEqual(want)
  })
})

describe('faces', () => {
  const plain = card(1, { term: 'apple', translation: 'яблоко' })
  const v = card(2, { term: 'go_went_gone', forms: ['go', 'went', 'gone'], translation: 'идти' })

  it('en_ru: term → translation', () => {
    expect(faces(plain, 'en_ru')).toEqual({ front: { kind: 'term', text: 'apple' }, back: { kind: 'translation', text: 'яблоко' } })
  })

  it('ru_en: translation → all three forms', () => {
    expect(faces(v, 'ru_en')).toEqual({ front: { kind: 'translation', text: 'идти' }, back: { kind: 'term', text: 'go – went – gone' } })
  })

  it('en_ru shows verb forms joined', () => {
    expect(faces(v, 'en_ru').front).toEqual({ kind: 'term', text: 'go – went – gone' })
  })

  it('context: sentence → matched form + translation', () => {
    const c = verb('I went home', [{ start: 2, end: 6 }])
    expect(faces(c, 'context')).toEqual({
      front: { kind: 'sentence', sentence: 'I went home', highlights: [{ start: 2, end: 6 }] },
      back: { kind: 'answer', term: 'went', translation: 'идти' },
    })
  })

  it('context, plain word: back shows the term', () => {
    const c = card(3, { term: 'apple', translation: 'яблоко', example: { text: 'An [apple]', sentence: 'An apple', highlights: [{ start: 3, end: 8 }] } })
    expect(faces(c, 'context').back).toEqual({ kind: 'answer', term: 'apple', translation: 'яблоко' })
  })

  it('context without example falls back to en_ru', () => {
    expect(faces(plain, 'context')).toEqual(faces(plain, 'en_ru'))
  })
})
