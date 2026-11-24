import { describe, expect, it } from 'vitest'
import { blankAmbition, descendants, emptyAmbitions, stepInput, treeRows } from './ambitions'
import type { ProjectStep } from './api/types'
const step = (id: number, parentId: number | null, position = 0): ProjectStep => ({ ...blankAmbition(`Step ${id}`, position), id, parentId, projectId: 1, done: false })
describe('project tree', () => {
  const steps = [step(3, 1), step(2, null, 1), step(1, null), step(4, 3)]
  it('sorts siblings and traverses parents before children regardless of input order', () => {
    expect(treeRows(steps).map(r => [r.step.id, r.depth])).toEqual([[1,0],[3,1],[4,2],[2,0]])
  })
  it('collapses a branch without hiding other roots', () => {
    expect(treeRows(steps, new Set([1])).map(r => r.step.id)).toEqual([1,2])
  })
  it('excludes the entire subtree when choosing a new parent', () => {
    expect([...descendants(steps,1)].sort()).toEqual([1,3,4])
  })
  it('preserves prerequisites and completion in a full PUT', () => {
    const data = emptyAmbitions(); data.dependencies = [{stepId:3,requiresId:2},{stepId:4,requiresId:2}]
    expect(stepInput(data,step(3,1)).requires).toEqual([2])
    expect(stepInput(data,step(3,1))).not.toHaveProperty('id')
  })
})
