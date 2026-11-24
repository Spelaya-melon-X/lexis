// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, expect, it, vi } from 'vitest'
import { api, ApiError } from '../../lib/api/client'
import type { Plan as DayPlan, Task } from '../../lib/api/types'
import { t } from '../../lib/i18n/i18n.svelte'
import TaskDialog from './Board/TaskDialog.svelte'
import Calendar from './Calendar/Calendar.svelte'
import Plan from './Plan/Plan.svelte'

vi.mock('svelte/transition', () => ({ fade: () => ({ duration: 0 }) }))

// jsdom не реализует открытие нативного диалога.
Object.defineProperties(HTMLDialogElement.prototype, {
  showModal: { value(this: HTMLDialogElement) { this.open = true }, configurable: true },
  close: { value(this: HTMLDialogElement) { this.open = false }, configurable: true },
})

const today = '2026-11-05'
const future = '2026-11-08'
const past = '2026-11-04'
const task: Task = {
  id: 1, title: 'Отчёт', blocks: 2, ownBlocks: 2, importance: 5, leaf: true,
  parentId: null, deadline: '2026-12-10', typeId: null, columnId: 2, position: 0, note: '',
  done: false, doneAt: null, workSec: 0, createdAt: '', updatedAt: '',
}
const item = { task, start: 540, blocks: 2 }
const settings = { dayStart: 540, dayEnd: 1380, dayLimit: 18 }

let view: ReturnType<typeof mount> | undefined
afterEach(async () => {
  if (view) await unmount(view)
  view = undefined
  vi.restoreAllMocks()
})

function go(hash: string) {
  history.replaceState(null, '', hash)
  window.dispatchEvent(new HashChangeEvent('hashchange'))
}

function button(label: string, selector = 'button') {
  return [...document.querySelectorAll<HTMLButtonElement>(selector)].find((b) => b.textContent?.trim() === label)
}

function fill(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  flushSync()
}

function submit() {
  document.querySelector('dialog[open] form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}

it('loads the visible month once and adds a task from the day cell', async () => {
  const calendar = vi.spyOn(api.tasks, 'calendar').mockResolvedValue({ today, from: '2026-11-01', to: '2026-11-30', items: [] })
  vi.spyOn(api.tasks, 'list').mockResolvedValue([task])
  const add = vi.spyOn(api.tasks, 'addPlanTask').mockResolvedValue({
    day: future, today, editable: true, totalBlocks: 2, settings, items: [item], top: [], leftovers: [],
  })
  go('#/game/calendar/2026-11')
  view = mount(Calendar, { target: document.body })
  const cell = (day: string) => document.querySelector(`a[href="#/game/plan/${day}"]`)?.closest('.cell') ?? null
  await vi.waitFor(() => expect(cell(future)).not.toBeNull())
  expect(calendar).toHaveBeenCalledExactlyOnceWith('2026-11-01', '2026-11-30')
  expect(cell(past)?.querySelector('button')).toBeNull()
  expect(cell(today)?.querySelector('a')?.getAttribute('aria-current')).toBe('date')
  cell(future)!.querySelector('button')!.click()
  await vi.waitFor(() => expect(document.querySelector('dialog[open] li button')?.textContent).toContain(task.title))
  document.querySelector<HTMLButtonElement>('dialog[open] li button')!.click()
  await vi.waitFor(() => expect(cell(future)?.querySelector('a')?.textContent).toContain(`9:00 ${task.title}`))
  expect(add).toHaveBeenCalledExactlyOnceWith(future, task.id)
  expect(calendar).toHaveBeenCalledTimes(1)
})

it('retries creation on the opened future date and displays past plans read-only', async () => {
  const opened: DayPlan = { day: future, today, editable: true, totalBlocks: 2, settings, items: [item], top: [], leftovers: [] }
  vi.spyOn(api.tasks, 'plan').mockImplementation(async (day) => day === past ? { ...opened, day, editable: false } : opened)
  vi.spyOn(api.tasks, 'list').mockResolvedValue([task])
  vi.spyOn(api.tasks.types, 'list').mockResolvedValue([])
  vi.spyOn(api.tasks.columns, 'list').mockResolvedValue([{ id: 2, name: 'Учёба', position: 0 }])
  const created = { ...task, id: 2, title: 'Новая задача', note: '#учёба' }
  const create = vi.spyOn(api.tasks, 'createPlanTask')
    .mockRejectedValueOnce(new ApiError(400, 'plan_no_slot', 'No slot'))
    .mockResolvedValueOnce({ ...opened, items: [item, { ...item, task: created, start: 600 }], totalBlocks: 4 })
  go(`#/game/plan/${future}`)
  view = mount(Plan, { target: document.body })
  const action = t('game.plan.newTask')
  await vi.waitFor(() => expect(button(action)).toBeDefined())
  button(action)!.click()
  await vi.waitFor(() => expect(document.querySelector('dialog[open] #task-form')).not.toBeNull())
  const title = document.querySelector<HTMLInputElement>('dialog[open] input[maxlength="200"]')!
  const note = document.querySelector<HTMLTextAreaElement>('dialog[open] textarea')!
  fill(title, created.title)
  fill(note, created.note)
  submit()
  await vi.waitFor(() => expect(document.querySelector('dialog[open] [role="alert"]')?.textContent).toBe(t('errors.plan_no_slot')))
  expect(title.value).toBe(created.title)
  expect(note.value).toBe(created.note)
  submit()
  await vi.waitFor(() => expect(document.querySelector('.scale')?.textContent).toContain(created.title))
  expect(document.querySelector('.scale')?.textContent).toContain(task.title)
  expect(create).toHaveBeenLastCalledWith(future, expect.objectContaining({ title: created.title, note: created.note, columnId: 2 }))

  go(`#/game/plan/${past}`)
  await vi.waitFor(() => expect(document.querySelector('.scale-card .note')?.textContent).toBe(t('game.plan.readonly')))
  expect(button(action)).toBeUndefined()
  expect(document.querySelector('.scale input[type="checkbox"]:disabled')).not.toBeNull()
})

it('updates and removes tags in the dialog while saving the original note', async () => {
  const update = vi.spyOn(api.tasks, 'update').mockImplementation(async (_, patch) => ({ ...task, note: patch.note ?? '' }))
  view = mount(TaskDialog, {
    target: document.body,
    props: { open: true, task: { ...task, note: '#Go #go' }, column: null, columns: [], types: [], onclose: () => {} },
  })
  flushSync()
  const note = document.querySelector<HTMLTextAreaElement>('dialog[open] textarea')!
  const tags = () => [...document.querySelectorAll('dialog[open] ul li')].map((li) => li.textContent)
  expect(tags()).toEqual(['#go'])
  fill(note, 'Текст #Учёба')
  expect(tags()).toEqual(['#учёба'])
  fill(note, '  Текст # https://example.com/#section  ')
  expect(tags()).toEqual([])
  submit()
  await vi.waitFor(() => expect(update).toHaveBeenCalledExactlyOnceWith(task.id, expect.objectContaining({ note: note.value })))
})
