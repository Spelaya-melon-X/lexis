import { describe, expect, it } from 'vitest'
import type { Task, TaskColumn } from './api/types'
import { ancestors, board, deadlineState, dropBefore, moveTask, shiftBefore, splitBlocks } from './tasks'

const task = (id: number, columnId: number, position: number, extra: Partial<Task> = {}): Task => ({
  id,
  parentId: null,
  leaf: true,
  title: `t${id}`,
  blocks: 1,
  ownBlocks: 1,
  importance: 5,
  deadline: null,
  typeId: null,
  columnId,
  position,
  note: '',
  done: false,
  doneAt: null,
  workSec: 0,
  createdAt: '',
  updatedAt: '',
  ...extra,
})

describe('splitBlocks', () => {
  it.each([
    { blocks: 1, want: { h: 0, m: 30 } },
    { blocks: 2, want: { h: 1, m: 0 } },
    { blocks: 7, want: { h: 3, m: 30 } },
  ])('$blocks', ({ blocks, want }) => {
    expect(splitBlocks(blocks)).toEqual(want)
  })
})

describe('deadlineState', () => {
  const today = '2026-11-05'
  it.each([
    { d: null, want: null },
    { d: '2026-11-04', want: 'overdue' },
    { d: '2026-11-05', want: 'today' },
    { d: '2026-11-08', want: 'soon' }, // ровно через 3 дня
    { d: '2026-11-09', want: 'later' },
    { d: '2027-01-01', want: 'later' },
  ])('$d', ({ d, want }) => {
    expect(deadlineState(d, today)).toBe(want)
  })
})

describe('board', () => {
  const columns: TaskColumn[] = [
    { id: 2, name: 'Doing', position: 1 },
    { id: 1, name: 'Todo', position: 0 },
  ]
  const tasks = [
    task(1, 1, 1, { blocks: 3, typeId: 7 }),
    task(2, 1, 0, { blocks: 2 }),
    task(3, 1, 2, { done: true, blocks: 5 }),
    task(4, 2, 0, { typeId: 7 }),
  ]
  const ids = (cols: ReturnType<typeof board>) => cols.map((c) => [c.column.id, c.tasks.map((t) => t.id), c.blocks])

  it('columns by position, tasks by position, done hidden', () => {
    expect(ids(board(columns, tasks, 'all', false))).toEqual([
      [1, [2, 1], 5],
      [2, [4], 1],
    ])
  })
  it('done shown but not counted in blocks', () => {
    expect(ids(board(columns, tasks, 'all', true))[0]).toEqual([1, [2, 1, 3], 5])
  })
  it('tasks with steps are hidden — their leaves are shown', () => {
    const withParent = [...tasks, task(5, 1, 3, { leaf: false, blocks: 9 })]
    expect(ids(board(columns, withParent, 'all', false))[0]).toEqual([1, [2, 1], 5])
  })
  it('type filters', () => {
    expect(ids(board(columns, tasks, 7, false))).toEqual([
      [1, [1], 3],
      [2, [4], 1],
    ])
    expect(ids(board(columns, tasks, 'none', false))).toEqual([
      [1, [2], 2],
      [2, [], 0],
    ])
  })
})

describe('moveTask', () => {
  const tasks = [task(1, 1, 0), task(2, 1, 1), task(3, 1, 2), task(9, 2, 0)]
  const col = (ts: Task[], c: number) =>
    ts
      .filter((t) => t.columnId === c)
      .sort((a, b) => a.position - b.position)
      .map((t) => t.id)

  it.each([
    { what: 'within: down', id: 1, to: 1, before: 3, c1: [2, 1, 3], c2: [9] },
    { what: 'within: up', id: 3, to: 1, before: 1, c1: [3, 1, 2], c2: [9] },
    { what: 'within: to end', id: 1, to: 1, before: null, c1: [2, 3, 1], c2: [9] },
    { what: 'to other column, before', id: 2, to: 2, before: 9, c1: [1, 3], c2: [2, 9] },
    { what: 'to other column, end', id: 2, to: 2, before: null, c1: [1, 3], c2: [9, 2] },
    { what: 'unknown before → end', id: 1, to: 2, before: 42, c1: [2, 3], c2: [9, 1] },
  ])('$what', ({ id, to, before, c1, c2 }) => {
    const got = moveTask(tasks, id, to, before)
    expect(col(got, 1)).toEqual(c1)
    expect(col(got, 2)).toEqual(c2)
    expect(tasks.map((t) => [t.id, t.columnId, t.position])).toEqual([
      [1, 1, 0],
      [2, 1, 1],
      [3, 1, 2],
      [9, 2, 0],
    ]) // вход не мутирован
  })
})

describe('dropBefore', () => {
  const cards = [
    { id: 1, mid: 50 },
    { id: 2, mid: 150 },
    { id: 3, mid: 250 },
  ]
  it.each([
    { y: 10, dragged: 9, want: 1 },
    { y: 100, dragged: 9, want: 2 },
    { y: 300, dragged: 9, want: null },
    { y: 100, dragged: 2, want: 3 }, // себя пропускаем
  ])('y=$y', ({ y, dragged, want }) => {
    expect(dropBefore(cards, y, dragged)).toBe(want)
  })
})

describe('shiftBefore', () => {
  const visible = [1, 2, 3]
  it.each([
    { id: 2, dir: -1 as const, want: 1 },
    { id: 1, dir: -1 as const, want: undefined },
    { id: 1, dir: 1 as const, want: 3 },
    { id: 2, dir: 1 as const, want: null }, // после последнего — в конец
    { id: 3, dir: 1 as const, want: undefined },
    { id: 9, dir: 1 as const, want: undefined },
  ])('$id $dir', ({ id, dir, want }) => {
    expect(shiftBefore(visible, id, dir)).toBe(want)
  })
})

describe('ancestors', () => {
  const tree = [task(1, 1, 0, { leaf: false }), task(2, 1, 1, { parentId: 1, leaf: false }), task(3, 1, 2, { parentId: 2 }), task(4, 1, 3)]
  it.each([
    { id: 3, want: [1, 2] },
    { id: 2, want: [1] },
    { id: 1, want: [] },
    { id: 4, want: [] },
    { id: 99, want: [] },
  ])('$id', ({ id, want }) => {
    expect(ancestors(tree, id).map((t) => t.id)).toEqual(want)
  })
  it('cycle in data does not hang', () => {
    const cyc = [task(1, 1, 0, { parentId: 2 }), task(2, 1, 1, { parentId: 1 })]
    expect(ancestors(cyc, 1).length).toBeLessThanOrEqual(2)
  })
})
