import { describe, expect, it } from 'vitest'
import { splitLines, splitTags } from './form'

describe('splitTags', () => {
  it.each([
    { s: '', want: [] },
    { s: '   ', want: [] },
    { s: 'food', want: ['food'] },
    { s: 'food, #verbs  nouns;x', want: ['food', '#verbs', 'nouns', 'x'] },
    { s: ' ,, food ,', want: ['food'] },
  ])('"$s"', ({ s, want }) => {
    expect(splitTags(s)).toEqual(want)
  })
})

describe('splitLines', () => {
  it('trims and drops empty lines', () => {
    expect(splitLines('  I [go]  \n\n  \nI [went]\r')).toEqual(['I [go]', 'I [went]'])
  })
  it('empty → []', () => {
    expect(splitLines('')).toEqual([])
  })
})
