// Доска дел «Игры» (§18.3): раскладка по колонкам, дедлайны, перестановки.
// Перестановка — зеркало Go-шного tasks.Reorder: фронт применяет её сразу (оптимистично),
// а на бэк уходит то же самое действие «встань перед beforeId».

import type { Task, TaskColumn } from './api/types'
import { addDays } from './dates'

/** Объём в блоках по 30 минут → часы и минуты: 3 → {h: 1, m: 30}. */
export function splitBlocks(blocks: number): { h: number; m: number } {
  return { h: Math.floor(blocks / 2), m: (blocks % 2) * 30 }
}

export type DeadlineState = 'overdue' | 'today' | 'soon' | 'later'

/** «Скоро» — в ближайшие 3 дня: такие дела пора двигать. */
export const SOON_DAYS = 3

/** Насколько горит дедлайн относительно сегодня (даты — YYYY-MM-DD, сравниваются как строки). */
export function deadlineState(deadline: string | null, today: string): DeadlineState | null {
  if (deadline === null) return null
  if (deadline < today) return 'overdue'
  if (deadline === today) return 'today'
  return deadline <= addDays(today, SOON_DAYS) ? 'soon' : 'later'
}

/** Фильтр доски по типу: все, без типа или конкретный. */
export type TypeFilter = 'all' | 'none' | number

export interface BoardColumn {
  column: TaskColumn
  tasks: Task[]
  blocks: number // сумма объёма невыполненных — «сколько работы в колонке»
}

const byPosition = (a: Task, b: Task) => a.position - b.position || a.id - b.id

/** Предки дела от корня (без самого дела) — для «Курсовая › Глава 1» на карточке шага. */
export function ancestors(tasks: Task[], id: number): Task[] {
  const byId = new Map(tasks.map((t) => [t.id, t]))
  const out: Task[] = []
  for (let p = byId.get(id)?.parentId ?? null; p !== null && out.length < tasks.length; p = byId.get(p)?.parentId ?? null) {
    const t = byId.get(p)
    if (!t) break
    out.unshift(t)
  }
  return out
}

/** Колонки в их порядке, в каждой — дела по position (выполненные — если showDone). */
export function board(columns: TaskColumn[], tasks: Task[], filter: TypeFilter, showDone: boolean): BoardColumn[] {
  // Дело с шагами на доске не показываем — вместо него его листья (§18.3, B2).
  const visible = tasks.filter(
    (t) => t.leaf && (showDone || !t.done) && (filter === 'all' || (filter === 'none' ? t.typeId === null : t.typeId === filter)),
  )
  return [...columns]
    .sort((a, b) => a.position - b.position || a.id - b.id)
    .map((column) => {
      const own = visible.filter((t) => t.columnId === column.id).sort(byPosition)
      return { column, tasks: own, blocks: own.reduce((sum, t) => sum + (t.done ? 0 : t.blocks), 0) }
    })
}

/**
 * Дело id встаёт в колонку columnId перед beforeId (null или не найден — в конец).
 * Возвращает новый массив: у целевой колонки позиции 0..n-1, остальные дела не трогаются.
 */
export function moveTask(tasks: Task[], id: number, columnId: number, beforeId: number | null): Task[] {
  const moving = tasks.find((t) => t.id === id)
  if (!moving) return tasks
  const order = tasks.filter((t) => t.columnId === columnId && t.id !== id).sort(byPosition)
  const at = beforeId === null ? -1 : order.findIndex((t) => t.id === beforeId)
  order.splice(at < 0 ? order.length : at, 0, moving)
  const placed = new Map(order.map((t, i) => [t.id, { ...t, columnId, position: i }]))
  return tasks.map((t) => placed.get(t.id) ?? t)
}

/**
 * Куда вставить при перетаскивании: перед первой карточкой, чья середина ниже курсора.
 * mids — вертикальные середины видимых карточек колонки (по порядку), y — курсор. null — в конец.
 */
export function dropBefore(cards: { id: number; mid: number }[], y: number, dragged: number): number | null {
  for (const c of cards) {
    if (c.id !== dragged && y < c.mid) return c.id
  }
  return null
}

/**
 * Сдвиг на одну позицию среди видимых карточек (клавиатура, кнопки ↑↓): какой beforeId отправить.
 * undefined — двигать некуда. Вниз = «перед тем, что после следующего» (null — в конец).
 */
export function shiftBefore(visible: number[], id: number, dir: -1 | 1): number | null | undefined {
  const i = visible.indexOf(id)
  if (i < 0) return undefined
  if (dir === -1) return i === 0 ? undefined : visible[i - 1]!
  if (i === visible.length - 1) return undefined
  return visible[i + 2] ?? null
}
