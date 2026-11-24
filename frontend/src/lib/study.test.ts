import { expect, it } from 'vitest'
import { studyCards, studyLibrary, studyQueue } from './study'
import type { StudyData } from './api/types'
const data: StudyData = {
  groups: [
    { id: 1, title: 'Math', parentId: null },
    { id: 2, title: 'Definitions', parentId: 1 },
    { id: 3, title: 'Other', parentId: null },
  ],
  cards: [
    { id: 4, groupId: 1, kind: 'theorem', front: 'A', back: 'B' },
    { id: 5, groupId: 2, kind: 'lemma', front: 'C', back: 'D' },
    { id: 6, groupId: 3, kind: 'definition', front: 'E', back: 'F' },
  ],
}
it('trains a group subtree, a subgroup, or a temporary selection', () => {
  expect(studyCards(data, 1).map((c) => c.id)).toEqual([4, 5])
  expect(studyCards(data, 2).map((c) => c.id)).toEqual([5])
  expect(
    studyQueue(studyCards(data, 1), new Set([5, 6]), () => 0).map((c) => c.id),
  ).toEqual([5])
  expect(
    studyQueue(data.cards, new Set(), () => 0)
      .map((c) => c.id)
      .sort(),
  ).toEqual([4, 5, 6])
})
it('detaches the session from the mutable library', () => {
  const queue = studyQueue(data.cards, new Set([4]), () => 0)
  queue[0]!.front = 'Changed'
  expect(data.cards[0]!.front).toBe('A')
})

it('separates grammar topics and their rules from existing practice material', () => {
  const combined: StudyData = {
    groups: [...data.groups, { id: 7, title: 'Grammar', parentId: null, grammar: true },
      { id: 8, title: 'Modal verbs', parentId: 7, grammar: true }],
    cards: [...data.cards, { id: 9, groupId: 8, kind: 'grammar_rule', front: 'Should', back: 'Advice', content: { formula: 'should + verb' } }],
  }
  expect(studyLibrary(combined, false)).toEqual(data)
  const grammar = studyLibrary(combined, true)
  expect(grammar.groups.map((g) => g.id)).toEqual([7, 8])
  expect(studyCards(grammar, 7).map((c) => c.id)).toEqual([9])
})

it('returns failed study cards after three others, including short and single-card queues', async () => {
  const {createQueue, answer, progress, finished}=await import('./training')
  for (const length of [1,2,3,4,5]) {
    const cards=Array.from({length},(_,i)=>({...data.cards[0]!,id:i+1}))
    let queue=createQueue(cards)
    queue=answer(queue,'again',3)
    expect(queue.queue.findIndex(c=>c.id===1)).toBe(Math.min(3,length-1))
    expect(queue.done).toBe(0)
    expect(progress(queue)).toBe(0)
    expect(queue.done+queue.queue.length).toBe(length)
    while(queue.queue.length>1)queue=answer(queue,'good',3)
    const last=queue.queue[0]!
    for(let i=0;i<3;i++){queue=answer(queue,'again',3);expect(queue.queue[0]).toBe(last);expect(finished(queue)).toBe(false)}
    queue=answer(queue,'good',3)
    expect(finished(queue)).toBe(true)
    expect(progress(queue)).toBe(1)
  }
})

it('classifies current library knowledge by answer count and score', async () => {
  const {studyLevel}=await import('./study')
  expect(studyLevel({total:0,score:0})).toBe('new')
  expect(studyLevel({total:10,score:-2})).toBe('weak')
  expect(studyLevel({total:2,score:0})).toBe('learning')
  expect(studyLevel({total:4,score:4})).toBe('learning')
  expect(studyLevel({total:5,score:5})).toBe('strong')
})
