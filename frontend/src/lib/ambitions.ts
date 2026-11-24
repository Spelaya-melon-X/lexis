import type { AmbitionInput, Ambitions, ProjectStep } from './api/types'
export const emptyAmbitions = (): Ambitions => ({ categories: [], dreams: [], projects: [], steps: [], dependencies: [], today: '', states: {}, summaries: {} })
export const blankAmbition = (title = '', position = 0): AmbitionInput => ({ title, note: '', deadline: null, position })
export function stepInput(data: Ambitions, step: ProjectStep): AmbitionInput {
  return { title: step.title, note: step.note, deadline: step.deadline, position: step.position, projectId: step.projectId, parentId: step.parentId, done: step.done, requires: data.dependencies.filter(d => d.stepId === step.id).map(d => d.requiresId) }
}
export function descendants(steps: ProjectStep[], id: number): Set<number> {
  const result = new Set([id])
  let changed = true
  while (changed) { changed = false; for (const n of steps) if (n.parentId !== null && result.has(n.parentId) && !result.has(n.id)) { result.add(n.id); changed = true } }
  return result
}
export function treeRows(steps: ProjectStep[], collapsed = new Set<number>()): { step: ProjectStep; depth: number }[] {
  const children = new Map<number | null, ProjectStep[]>()
  for (const n of steps) children.set(n.parentId, [...(children.get(n.parentId) ?? []), n])
  for (const list of children.values()) list.sort((a, b) => a.position - b.position || a.id - b.id)
  const out: { step: ProjectStep; depth: number }[] = []
  const walk = (parent: number | null, depth: number) => { for (const step of children.get(parent) ?? []) { out.push({ step, depth }); if (!collapsed.has(step.id)) walk(step.id, depth + 1) } }
  walk(null, 0)
  return out
}
