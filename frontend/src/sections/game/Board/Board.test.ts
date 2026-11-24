// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { expect, it, vi } from 'vitest'
import { api } from '../../../lib/api/client'
import type { Task } from '../../../lib/api/types'
import Board from './Board.svelte'

vi.mock('svelte/transition', () => ({ fade: () => ({ duration: 0 }) }))

it('scrolls toward either edge during a task drag and drops in the newly visible column', async () => {
  const task: Task = {
    id: 1, title: 'Дело', blocks: 2, ownBlocks: 2, importance: 5, leaf: true,
    parentId: null, deadline: null, typeId: null, columnId: 2, position: 0, note: '',
    done: false, doneAt: null, workSec: 0, createdAt: '', updatedAt: '',
  }
  vi.spyOn(api.tasks.columns, 'list').mockResolvedValue([
    { id: 1, name: 'Слева', position: 0 },
    { id: 2, name: 'Центр', position: 1 },
    { id: 3, name: 'Справа', position: 2 },
  ])
  vi.spyOn(api.tasks.types, 'list').mockResolvedValue([])
  vi.spyOn(api.tasks, 'list').mockResolvedValue([task, { ...task, id: 2, columnId: 3 }])
  const move = vi.spyOn(api.tasks, 'move').mockResolvedValue()
  let frame: FrameRequestCallback | undefined
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    frame = callback
    return 1
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => { frame = undefined })
  function advance(time: number) {
    const callback = frame
    expect(callback).toBeDefined()
    frame = undefined
    callback!(time)
  }
  function drag(target: Element, type: string, clientX = 0, clientY = 0) {
    target.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, clientX, clientY }))
    flushSync()
  }

  const view = mount(Board, { target: document.body })
  try {
    await vi.waitFor(() => expect(document.querySelector('[data-id="1"]')).not.toBeNull())
    const board = document.querySelector<HTMLDivElement>('.board')!
    Object.defineProperties(board, { clientWidth: { value: 600 }, scrollWidth: { value: 1200 } })
    vi.spyOn(board, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 0, 600, 600))
    board.scrollLeft = 300

    drag(document.querySelector('[data-id="1"]')!, 'dragstart')
    drag(board, 'dragover', 110, 20)
    advance(16)
    const first = board.scrollLeft
    expect(first).toBeLessThan(300)
    advance(32)
    expect(board.scrollLeft).toBeLessThan(first)

    drag(board, 'dragover', 400, 20)
    expect(frame).toBeUndefined()
    const atCenter = board.scrollLeft
    drag(board, 'dragover', 690, 20)
    advance(48)
    expect(board.scrollLeft).toBeGreaterThan(atCenter)

    const nextCard = document.querySelector('[data-id="2"]')!
    vi.spyOn(nextCard, 'getBoundingClientRect').mockReturnValue(new DOMRect(620, 40, 80, 60))
    drag(nextCard, 'drop', 690, 45)
    expect(frame).toBeUndefined()
    expect(move).toHaveBeenCalledExactlyOnceWith(1, 3, 2)
  } finally {
    await unmount(view)
    vi.restoreAllMocks()
  }
})
