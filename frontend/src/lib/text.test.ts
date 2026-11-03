import { describe, expect, it } from 'vitest'
import { displayTerm, segments } from './text'

const seg = (text: string, highlight = false) => ({ text, highlight })

describe('segments', () => {
  it.each([
    { name: 'no spans', s: 'I eat', spans: [], want: [seg('I eat')] },
    { name: 'empty sentence', s: '', spans: [], want: [] },
    { name: 'one in the middle', s: 'I go home', spans: [{ start: 2, end: 4 }], want: [seg('I '), seg('go', true), seg(' home')] },
    { name: 'at start and end', s: 'Go now go', spans: [{ start: 0, end: 2 }, { start: 7, end: 9 }], want: [seg('Go', true), seg(' now '), seg('go', true)] },
    { name: 'adjacent', s: 'ab', spans: [{ start: 0, end: 1 }, { start: 1, end: 2 }], want: [seg('a', true), seg('b', true)] },
    { name: 'unsorted spans', s: 'a b c', spans: [{ start: 4, end: 5 }, { start: 0, end: 1 }], want: [seg('a', true), seg(' b '), seg('c', true)] },
    { name: 'cyrillic', s: 'Я иду домой', spans: [{ start: 2, end: 5 }], want: [seg('Я '), seg('иду', true), seg(' домой')] },
    // Главный кейс: смещения в рунах, а "🍎".length === 2 в UTF-16.
    { name: 'emoji before highlight (code points, not UTF-16)', s: '🍎 apple', spans: [{ start: 2, end: 7 }], want: [seg('🍎 '), seg('apple', true)] },
    { name: 'span past the end is clipped', s: 'abc', spans: [{ start: 1, end: 10 }], want: [seg('a'), seg('bc', true)] },
    { name: 'overlapping spans do not duplicate text', s: 'abcdef', spans: [{ start: 0, end: 4 }, { start: 2, end: 6 }], want: [seg('abcd', true), seg('ef', true)] },
  ])('$name', ({ s, spans, want }) => {
    expect(segments(s, spans)).toEqual(want)
  })

  it('joined segments always give the original sentence', () => {
    const s = 'Ёж 🦔 [ест] яблоко'
    expect(segments(s, [{ start: 3, end: 4 }, { start: 5, end: 10 }]).map((x) => x.text).join('')).toBe(s)
  })
})

describe('displayTerm', () => {
  it('joins verb forms with dashes', () => {
    expect(displayTerm('go_went_gone', ['go', 'went', 'gone'])).toBe('go – went – gone')
  })
  it('plain term as is', () => {
    expect(displayTerm('well-known')).toBe('well-known')
    expect(displayTerm('apple', [])).toBe('apple')
  })
})
