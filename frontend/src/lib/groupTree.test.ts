import { describe, expect, it } from 'vitest'
import { canCreateIn, canMove, depthOf, dropParent, heightOf, pathTo, subtree, visibleRows } from './groupTree'

// airport, gates, hotel, travel, verbs — как с бэка, по имени:
//	travel(4) ── airport(1) ── gates(2)
//	         └─ hotel(3)
//	verbs(5)
const groups = [
  { id: 1, name: 'airport', parentId: 4 },
  { id: 2, name: 'gates', parentId: 1 },
  { id: 3, name: 'hotel', parentId: 4 },
  { id: 4, name: 'travel', parentId: null },
  { id: 5, name: 'verbs', parentId: null },
]

const names = (rows: ReturnType<typeof visibleRows<(typeof groups)[number]>>) =>
  rows.map((r) => `${'-'.repeat(r.depth)}${r.group.name}${r.hasChildren ? '+' : ''}`)

describe('visibleRows', () => {
  it('depth-first, siblings keep server order', () => {
    expect(names(visibleRows(groups))).toEqual(['travel+', '-airport+', '--gates', '-hotel', 'verbs'])
  })
  it('collapsed group hides its subtree but keeps the hasChildren mark', () => {
    expect(names(visibleRows(groups, new Set([4])))).toEqual(['travel+', 'verbs'])
    expect(names(visibleRows(groups, new Set([1])))).toEqual(['travel+', '-airport+', '-hotel', 'verbs'])
  })
  it('unknown parent → shown at root, not lost', () => {
    expect(names(visibleRows([{ id: 1, name: 'orphan', parentId: 99 }]))).toEqual(['orphan'])
  })
  it('cycle in data does not hang', () => {
    const cyc = [
      { id: 1, name: 'a', parentId: 2 },
      { id: 2, name: 'b', parentId: 1 },
    ]
    expect(visibleRows(cyc)).toEqual([]) // в цикле нет корня — показать нечего, но и не зависнуть
  })
})

describe('tree helpers', () => {
  it('pathTo', () => {
    expect(pathTo(groups, 2).map((g) => g.name)).toEqual(['travel', 'airport', 'gates'])
    expect(pathTo(groups, 5).map((g) => g.name)).toEqual(['verbs'])
    expect(pathTo(groups, 42)).toEqual([])
  })
  it('depthOf / heightOf / subtree', () => {
    expect([depthOf(groups, null), depthOf(groups, 4), depthOf(groups, 2)]).toEqual([0, 1, 3])
    expect([heightOf(groups, 4), heightOf(groups, 1), heightOf(groups, 5)]).toEqual([3, 2, 1])
    expect([...subtree(groups, 4)].sort()).toEqual([1, 2, 3, 4])
  })
})

describe('canMove', () => {
  it.each([
    { what: 'leaf to root', id: 2, to: null, ok: true },
    { what: 'to the same parent — nothing to do', id: 1, to: 4, ok: false },
    { what: 'root group to root — nothing to do', id: 5, to: null, ok: false },
    { what: 'into itself', id: 1, to: 1, ok: false },
    { what: 'into own descendant', id: 4, to: 2, ok: false },
    { what: 'height 2 under a root group', id: 1, to: 5, ok: true },
    { what: 'height 3 under a root group → too deep', id: 4, to: 5, ok: false },
    { what: 'leaf under depth-2 group', id: 5, to: 1, ok: true },
    { what: 'leaf under depth-3 group → too deep', id: 5, to: 2, ok: false },
    { what: 'unknown group', id: 42, to: null, ok: false },
  ])('$what', ({ id, to, ok }) => {
    expect(canMove(groups, id, to)).toBe(ok)
  })
})

describe('canCreateIn', () => {
  it.each([
    { parent: null, ok: true },
    { parent: 4, ok: true },
    { parent: 1, ok: true },
    { parent: 2, ok: false },
  ])('$parent', ({ parent, ok }) => {
    expect(canCreateIn(groups, parent)).toBe(ok)
  })
})

describe('dropParent', () => {
  const row = { id: 1, parentId: 4 }
  it.each([
    { y: 2, want: { parent: 4, into: false } }, // верхний край — рядом
    { y: 20, want: { parent: 1, into: true } }, // середина — внутрь
    { y: 58, want: { parent: 4, into: false } }, // нижний край — рядом
  ])('y=$y', ({ y, want }) => {
    expect(dropParent(row, y, 60)).toEqual(want)
  })
  it('root row edges → root', () => {
    expect(dropParent({ id: 5, parentId: null }, 1, 60)).toEqual({ parent: null, into: false })
  })
})
