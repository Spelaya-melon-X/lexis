// Чистая логика тренировки на клиенте (SPEC §7): очередь сессии и что показывать на сторонах карточки.
import type { Answer, Span, Strategy, TrainingCard } from './api/types'
import { displayTerm, segments } from './text'

// ---------- очередь ----------

/**
 * Состояние очереди. queue[0] — текущая карточка. done — сколько карточек отвечено
 * окончательно (Знаю/Затрудняюсь). total — исходный размер: «Не знаю» возвращает карточку
 * в очередь, а не добавляет новую, поэтому done + queue.length === total всегда —
 * прогресс не прыгает назад.
 */
export interface QueueState {
  queue: TrainingCard[]
  done: number
  total: number
}

export function createQueue(cards: TrainingCard[]): QueueState {
  return { queue: [...cards], done: 0, total: cards.length }
}

export function current(s: QueueState): TrainingCard | undefined {
  return s.queue[0]
}

/**
 * Применяет ответ на текущую карточку. «Не знаю» возвращает её через requeueAt позиций
 * (сервер даёт 3–5); если осталось меньше — в конец, а если она последняя — она же
 * и будет следующей: повторяем, пока не ответишь Знаю/Затрудняюсь.
 */
export function answer(s: QueueState, a: Answer, requeueAt?: number): QueueState {
  const [head, ...rest] = s.queue
  if (!head) return s
  if (a === 'again') {
    const at = Math.min(Math.max(requeueAt ?? 3, 0), rest.length)
    return { ...s, queue: [...rest.slice(0, at), head, ...rest.slice(at)] }
  }
  return { ...s, queue: rest, done: s.done + 1 }
}

export function progress(s: QueueState): number {
  return s.total === 0 ? 1 : s.done / s.total
}

export function finished(s: QueueState): boolean {
  return s.queue.length === 0
}

// ---------- стороны карточки ----------

/**
 * Формы глагола, совпавшие с выделенными в примере словами (без учёта регистра), в порядке форм.
 * Ни одна не совпала (или не глагол) — все формы (§4). Для не-глагола — [].
 */
export function matchedForms(card: TrainingCard): string[] {
  const forms = card.forms ?? []
  if (forms.length === 0 || !card.example) return forms
  const marked = new Set(
    segments(card.example.sentence, card.example.highlights)
      .filter((s) => s.highlight)
      .map((s) => s.text.trim().toLowerCase()),
  )
  const hit = forms.filter((f) => marked.has(f.toLowerCase()))
  return hit.length > 0 ? hit : forms
}

/** Что спрашиваем (front) и что показываем в ответе (back). */
export type Face =
  | { kind: 'term'; text: string } // английское слово / 3 формы
  | { kind: 'translation'; text: string }
  | { kind: 'sentence'; sentence: string; highlights?: Span[] }
  | { kind: 'answer'; term: string; translation: string } // слово + перевод (для context)

export function faces(card: TrainingCard, strategy: Strategy): { front: Face; back: Face } {
  const term = displayTerm(card.term, card.forms)
  switch (strategy) {
    case 'ru_en':
      // ru_en: «идти» → все три формы, засчитывается целиком (§7)
      return { front: { kind: 'translation', text: card.translation }, back: { kind: 'term', text: term } }
    case 'context':
      if (card.example) {
        const forms = matchedForms(card)
        return {
          front: { kind: 'sentence', sentence: card.example.sentence, highlights: card.example.highlights },
          back: { kind: 'answer', term: forms.length > 0 ? forms.join(' – ') : term, translation: card.translation },
        }
      }
      // слово без примеров в context показываем как en_ru (§7)
      return { front: { kind: 'term', text: term }, back: { kind: 'translation', text: card.translation } }
    default:
      return { front: { kind: 'term', text: term }, back: { kind: 'translation', text: card.translation } }
  }
}
