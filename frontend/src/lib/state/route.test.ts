import { describe, expect, it } from 'vitest'
import { parseHash, toHash, type Route } from './route'

describe('parseHash', () => {
  it.each<{ hash: string; want: Route }>([
    { hash: '', want: { section: 'english', page: 'add', rest: '' } },
    { hash: '#', want: { section: 'english', page: 'add', rest: '' } },
    { hash: '#/', want: { section: 'english', page: 'add', rest: '' } },
    // новый формат
    { hash: '#/english/groups', want: { section: 'english', page: 'groups', rest: '' } },
    { hash: '#/english/groups/12', want: { section: 'english', page: 'groups', rest: '12' } },
    { hash: '#/english/training/5/x', want: { section: 'english', page: 'training', rest: '5/x' } },
    { hash: '#/day', want: { section: 'day', page: 'entry', rest: '' } },
    { hash: '#/day/entry/2026-11-03', want: { section: 'day', page: 'entry', rest: '2026-11-03' } },
    { hash: '#/habits/notes', want: { section: 'habits', page: 'notes', rest: '' } },
    { hash: '#/settings', want: { section: 'settings', page: 'general', rest: '' } },
    { hash: '#/settings/data', want: { section: 'settings', page: 'data', rest: '' } },
    // чужой/неизвестный подраздел → первый в разделе, хвост не переносим
    { hash: '#/day/groups/12', want: { section: 'day', page: 'entry', rest: '' } },
    { hash: '#/english/nope/1', want: { section: 'english', page: 'add', rest: '' } },
    // старые хэши v1 → английский
    { hash: '#add', want: { section: 'english', page: 'add', rest: '' } },
    { hash: '#groups', want: { section: 'english', page: 'groups', rest: '' } },
    { hash: '#/groups', want: { section: 'english', page: 'groups', rest: '' } },
    { hash: '#groups/12', want: { section: 'english', page: 'groups', rest: '12' } },
    { hash: '#training/5', want: { section: 'english', page: 'training', rest: '5' } },
    { hash: '#stats', want: { section: 'english', page: 'stats', rest: '' } },
    { hash: '#settings', want: { section: 'english', page: 'settings', rest: '' } },
    // «#settings» v1 и «#/settings» v2 — разные вещи
    { hash: '#/settings', want: { section: 'settings', page: 'general', rest: '' } },
    { hash: '#day', want: { section: 'day', page: 'entry', rest: '' } }, // раздел без слэша — тоже понимаем
    // мусор
    { hash: '#nope/12', want: { section: 'english', page: 'add', rest: '' } },
    { hash: '#GROUPS', want: { section: 'english', page: 'add', rest: '' } },
    { hash: '#/toString', want: { section: 'english', page: 'add', rest: '' } },
  ])('$hash', ({ hash, want }) => {
    expect(parseHash(hash)).toEqual(want)
  })
})

describe('toHash', () => {
  it.each<{ hash: string; want: string }>([
    { hash: '#groups/12', want: '#/english/groups/12' },
    { hash: '#stats', want: '#/english/stats' },
    { hash: '', want: '#/english/add' },
    { hash: '#/habits/goals', want: '#/habits/goals' },
  ])('canonical $hash → $want', ({ hash, want }) => {
    expect(toHash(parseHash(hash))).toBe(want)
  })

  it('round-trips', () => {
    const r: Route = { section: 'day', page: 'entry', rest: '2026-11-03' }
    expect(parseHash(toHash(r))).toEqual(r)
  })
})
