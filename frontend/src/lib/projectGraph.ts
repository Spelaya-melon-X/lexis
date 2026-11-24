import type { ProjectStep } from './api/types'
import { layoutTree } from './taskTree'

export const GRAPH_NODE_W = 232
export const GRAPH_NODE_H = 104
const COLUMN_GAP = 88
const ROW_GAP = 28
const PAD = 32

export interface GraphNode { id: number; x: number; y: number }
export interface GraphEdge { from: number; to: number; path: string }
export interface ProjectGraphLayout {
  nodes: GraphNode[]
  edges: GraphEdge[]
  width: number
  height: number
}

/** Virtual node 0 is the goal; roots become its stages. Leaves stay on separate rows. */
export function layoutProjectGraph(steps: ProjectStep[]): ProjectGraphLayout {
  const ordered = [...steps].sort((a,b) => a.position-b.position || a.id-b.id)
  const tree = layoutTree([{id:0,parentId:null}, ...ordered.map(s => ({id:s.id,parentId:s.parentId ?? 0}))])
  const nodes = [...tree.nodes.values()].map(n => ({id:n.id,x:PAD+n.depth*(GRAPH_NODE_W+COLUMN_GAP),y:PAD+n.x*(GRAPH_NODE_H+ROW_GAP)}))
  const byID = new Map(nodes.map(n => [n.id,n]))
  const edges = tree.edges.map(e => {
    const a=byID.get(e.from)!, b=byID.get(e.to)!
    const x1=a.x+GRAPH_NODE_W,y1=a.y+GRAPH_NODE_H/2,x2=b.x,y2=b.y+GRAPH_NODE_H/2
    return {...e,path:`M ${x1} ${y1} C ${(x1+x2)/2} ${y1}, ${(x1+x2)/2} ${y2}, ${x2} ${y2}`}
  })
  return {nodes,edges,width:PAD*2+tree.depth*GRAPH_NODE_W+(tree.depth-1)*COLUMN_GAP,height:PAD*2+tree.width*GRAPH_NODE_H+(tree.width-1)*ROW_GAP}
}

/** A → B means A waits for B. Route the arrow above cards so it stays distinct from tree edges. */
export function dependencyPath(a: GraphNode,b: GraphNode): string {
  const x1=a.x+GRAPH_NODE_W/2,x2=b.x+GRAPH_NODE_W/2
  if (a.x === b.x) {
    const left=a.x-36,y1=a.y+GRAPH_NODE_H/2,y2=b.y+GRAPH_NODE_H/2
    return `M ${a.x} ${y1} C ${left} ${y1}, ${left} ${y2}, ${b.x} ${y2}`
  }
  const top=Math.min(a.y,b.y)-22
  return `M ${x1} ${a.y} C ${x1} ${top}, ${x2} ${top}, ${x2} ${b.y}`
}
