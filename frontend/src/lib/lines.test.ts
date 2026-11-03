import { describe, expect, it } from 'vitest'
import { keepLines, lineRange } from './lines'

describe('keepLines', () => {
  it.each([
    { text: 'a\nb\nc', lines: [2], want: 'b' },
    { text: 'a\nb\nc', lines: [1, 3], want: 'a\nc' },
    { text: 'a\nb\nc', lines: [], want: '' },
    { text: 'a\nb', lines: [5], want: '' },
    { text: 'a\r\nb\r\n', lines: [2], want: 'b\r' }, // CRLF: \r остаётся, бэк его съест trim-ом
    { text: 'x\n\n// c\ny', lines: [4], want: 'y' },
  ])('$lines of $text', ({ text, lines, want }) => {
    expect(keepLines(text, lines)).toBe(want)
  })
})

describe('lineRange', () => {
  const text = 'apple || яблоко\n\npear || груша'
  it.each([
    { line: 1, want: [0, 15] },
    { line: 2, want: [16, 16] }, // пустая строка
    { line: 3, want: [17, 30] },
    { line: 0, want: null },
    { line: 4, want: null },
  ])('line $line', ({ line, want }) => {
    expect(lineRange(text, line)).toEqual(want)
  })

  it('range matches the line text', () => {
    const r = lineRange(text, 3)!
    expect(text.slice(r[0], r[1])).toBe('pear || груша')
  })
})
