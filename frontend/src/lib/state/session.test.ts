import { expect, it } from 'vitest'
import { session } from './session.svelte'

it('releases the temporary queue on finish, retaining the group and ordinary results', () => {
  session.begin({sessionId:1,groupId:7,groupName:'Selected',strategy:'ru_en',free:true},[
    {wordId:2,term:'apple',translation:'яблоко',level:1},
    {wordId:4,term:'pear',translation:'груша',level:2},
  ])
  expect(session.active?.q.queue.map(c=>c.wordId)).toEqual([2,4])
  session.apply('again',3)
  expect(session.active?.q.queue.map(c=>c.wordId)).toEqual([4,2])
  session.showResults({sessionId:1,free:true,total:1,good:0,hard:0,again:1,durationSec:10,levelChanges:[]})
  expect(session.phase).toBe('results')
  expect(session.active?.q.queue).toEqual([])
  expect(session.active?.q.total).toBe(0)
  expect(session.active?.groupId).toBe(7)
  session.reset()
  session.begin({sessionId:2,groupId:7,groupName:'Whole group',strategy:'en_ru',free:true},[
    {wordId:5,term:'new',translation:'новый',level:1},
  ])
  expect(session.active?.q.queue.map(c=>c.wordId)).toEqual([5])
})
