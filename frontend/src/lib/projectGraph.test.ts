import { describe, expect, it } from 'vitest'
import type { ProjectStep } from './api/types'
import { blankAmbition } from './ambitions'
import { GRAPH_NODE_H, GRAPH_NODE_W, layoutProjectGraph } from './projectGraph'

const step = (id:number,parentId:number|null,position=0):ProjectStep => ({...blankAmbition(`Step ${id}`,position),id,parentId,projectId:1,done:false})

describe('project graph layout', () => {
  it('keeps a visible goal even before the first step', () => {
    const graph=layoutProjectGraph([])
    expect(graph.nodes.map(n=>n.id)).toEqual([0])
    expect(graph.width).toBeGreaterThan(GRAPH_NODE_W)
    expect(graph.height).toBeGreaterThan(GRAPH_NODE_H)
  })
  it('connects all stages to the goal, even when children precede parents in the input', () => {
    const graph=layoutProjectGraph([step(3,1),step(2,null,1),step(1,null)])
    expect(graph.edges.map(e=>[e.from,e.to])).toEqual([[0,1],[1,3],[0,2]])
    for (const e of graph.edges) {
      const parent=graph.nodes.find(n=>n.id===e.from)!, child=graph.nodes.find(n=>n.id===e.to)!
      expect(child.x).toBeGreaterThan(parent.x+GRAPH_NODE_W)
    }
  })
  it('puts leaf tasks on separate rows with no overlapping cards', () => {
    const graph=layoutProjectGraph([step(1,null),step(2,1),step(3,1,1),step(4,null,1)])
    const leaves=graph.nodes.filter(n=>[2,3,4].includes(n.id)).sort((a,b)=>a.y-b.y)
    for (let i=1;i<leaves.length;i++) expect(leaves[i]!.y-leaves[i-1]!.y).toBeGreaterThan(GRAPH_NODE_H)
    for (const n of graph.nodes) {
      expect(n.x+GRAPH_NODE_W).toBeLessThanOrEqual(graph.width)
      expect(n.y+GRAPH_NODE_H).toBeLessThanOrEqual(graph.height)
    }
  })
  it('respects sibling order and can preview a new task without changing saved steps', () => {
    const steps=[step(1,null),step(2,1),step(3,1,1)]
    const before=structuredClone(steps)
    const graph=layoutProjectGraph([...steps,step(-1,1,2)])
    expect(graph.edges.some(e=>e.from===1&&e.to===-1)).toBe(true)
    expect(graph.nodes.find(n=>n.id===-1)!.y).toBeGreaterThan(graph.nodes.find(n=>n.id===3)!.y)
    expect(steps).toEqual(before)
  })
})
