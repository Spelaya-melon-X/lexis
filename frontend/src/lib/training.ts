// Чистая логика очереди тренировки (SPEC §7).
import type { Answer, TrainingCard } from './api/types'

// ---------- очередь ----------

/**
 * Состояние очереди. queue[0] — текущая карточка. done — сколько карточек отвечено
 * окончательно (Знаю/Затрудняюсь). total — исходный размер: «Не знаю» возвращает карточку
 * в очередь, а не добавляет новую, поэтому done + queue.length === total всегда —
 * прогресс не прыгает назад.
 */
export interface QueueState<C = TrainingCard> {
  queue: C[]
  done: number
  total: number
}

export function createQueue<C>(cards: C[]): QueueState<C> {
  return { queue: [...cards], done: 0, total: cards.length }
}

export function current<C>(s: QueueState<C>): C | undefined {
  return s.queue[0]
}

/**
 * Применяет ответ на текущую карточку. «Не знаю» возвращает её через requeueAt позиций
 * (сервер даёт 3–5); если осталось меньше — в конец, а если она последняя — она же
 * и будет следующей: повторяем, пока не ответишь Знаю/Затрудняюсь.
 */
export function answer<C>(s: QueueState<C>, a: Answer, requeueAt?: number): QueueState<C> {
  const [head, ...rest] = s.queue
  if (!head) return s
  if (a === 'again') {
    const at = Math.min(Math.max(requeueAt ?? 3, 0), rest.length)
    return { ...s, queue: [...rest.slice(0, at), head, ...rest.slice(at)] }
  }
  return { ...s, queue: rest, done: s.done + 1 }
}

export function progress(s: QueueState<unknown>): number {
  return s.total === 0 ? 1 : s.done / s.total
}

export function finished(s: QueueState<unknown>): boolean {
  return s.queue.length === 0
}
