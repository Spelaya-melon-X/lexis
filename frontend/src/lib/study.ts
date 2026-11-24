import type { StudyCard, StudyData, StudyKind } from './api/types'
import { subtree } from './groupTree'
export const STUDY_KINDS: StudyKind[] = [
  'definition',
  'theorem',
  'remark',
  'property',
  'lemma',
]

export function studyLibrary(data: StudyData, grammar: boolean): StudyData {
  return {
    groups: data.groups.filter((group) => Boolean(group.grammar) === grammar),
    cards: data.cards.filter((card) => (card.kind === 'grammar_rule') === grammar),
  }
}

export function studyCards(
  data: StudyData,
  groupId: number | null,
): StudyCard[] {
  if (groupId === null) return data.cards
  const ids = subtree(data.groups, groupId)
  return data.cards.filter((c) => ids.has(c.groupId))
}
/** Session is a detached snapshot; editing the library cannot mutate an active practice. */
export function studyQueue(
  cards: StudyCard[],
  selected: ReadonlySet<number>,
  random: () => number = Math.random,
): StudyCard[] {
  const queue = cards
    .filter((c) => selected.size === 0 || selected.has(c.id))
    .map((c) => ({ ...c }))
  for (let i = queue.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1))
    ;[queue[i], queue[j]] = [queue[j]!, queue[i]!]
  }
  return queue
}

export type StudyLevel = 'new' | 'weak' | 'learning' | 'strong'
export function studyLevel(card: { total: number; score: number }): StudyLevel {
  if (card.total === 0) return 'new'
  if (card.score < 0) return 'weak'
  return card.score >= 5 ? 'strong' : 'learning'
}
