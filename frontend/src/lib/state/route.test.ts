import { describe, expect, it } from 'vitest'
import { parseHash, toHash } from './route'

describe('parseHash', () => {
  it.each([
    { hash: '', want: { tab: 'add', rest: '' } },
    { hash: '#', want: { tab: 'add', rest: '' } },
    { hash: '#groups', want: { tab: 'groups', rest: '' } },
    { hash: '#/groups', want: { tab: 'groups', rest: '' } },
    { hash: '#groups/12', want: { tab: 'groups', rest: '12' } },
    { hash: '#training/5/x', want: { tab: 'training', rest: '5/x' } },
    { hash: '#nope/12', want: { tab: 'add', rest: '' } }, // хвост от чужого таба не переносим
    { hash: '#GROUPS', want: { tab: 'add', rest: '' } },
  ])('$hash', ({ hash, want }) => {
    expect(parseHash(hash)).toEqual(want)
  })

  it('toHash round-trips', () => {
    expect(parseHash(toHash('groups', '7'))).toEqual({ tab: 'groups', rest: '7' })
    expect(toHash('stats')).toBe('#stats')
  })
})
