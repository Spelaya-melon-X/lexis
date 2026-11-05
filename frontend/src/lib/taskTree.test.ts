import { describe, expect, it } from 'vitest'
import { layoutTree } from './taskTree'

describe('layoutTree', () => {
  it('leaves take consecutive slots, parents are centered over children', () => {
    //	1
    //	├── 2
    //	│   ├── 4
    //	│   └── 5
    //	└── 3
    const l = layoutTree([
      { id: 1, parentId: null },
      { id: 2, parentId: 1 },
      { id: 4, parentId: 2 },
      { id: 5, parentId: 2 },
      { id: 3, parentId: 1 },
    ])
    const at = (id: number) => [l.nodes.get(id)!.x, l.nodes.get(id)!.depth]
    expect([at(4), at(5), at(3)]).toEqual([
      [0, 2],
      [1, 2],
      [2, 1],
    ])
    expect(at(2)).toEqual([0.5, 1])
    expect(at(1)).toEqual([1.25, 0]) // посередине между 2 (0.5) и 3 (2)
    expect([l.width, l.depth]).toEqual([3, 3])
    expect(l.edges).toEqual([
      { from: 1, to: 2 },
      { from: 2, to: 4 },
      { from: 2, to: 5 },
      { from: 1, to: 3 },
    ])
  })

  it('single task', () => {
    const l = layoutTree([{ id: 7, parentId: null }])
    expect(l.nodes.get(7)).toEqual({ id: 7, x: 0, depth: 0 })
    expect([l.width, l.depth]).toEqual([1, 1])
  })

  it('empty', () => {
    expect(layoutTree([])).toEqual({ nodes: new Map(), edges: [], width: 0, depth: 0 })
  })

  it('subtree whose parent is missing is laid out as a root', () => {
    const l = layoutTree([
      { id: 2, parentId: 1 },
      { id: 3, parentId: 2 },
    ])
    expect(l.nodes.get(2)!.depth).toBe(0)
    expect(l.nodes.get(3)!.depth).toBe(1)
  })
})
