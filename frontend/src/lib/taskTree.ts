// Раскладка графа дела (§18.3, B2): свой SVG без библиотек.
// Простой «аккуратный» алгоритм: листья занимают слоты слева направо в порядке обхода,
// родитель — посередине между первым и последним ребёнком. Глубина — строка.

export interface TreeNode {
  id: number
  parentId: number | null
}

export interface Placed {
  id: number
  x: number // слот по горизонтали (может быть дробным — середина детей)
  depth: number // 0 — корень
}

export interface Layout {
  nodes: Map<number, Placed>
  edges: { from: number; to: number }[]
  width: number // число слотов-листьев
  depth: number // число уровней
}

/**
 * nodes — дерево в порядке обхода в глубину (как отдаёт GET /tasks/{id}/tree): соседи уже упорядочены.
 * Корень — узел, чьего родителя нет в списке.
 */
export function layoutTree(nodes: TreeNode[]): Layout {
  const ids = new Set(nodes.map((n) => n.id))
  const children = new Map<number, number[]>()
  const roots: number[] = []
  for (const n of nodes) {
    if (n.parentId !== null && ids.has(n.parentId)) {
      const list = children.get(n.parentId)
      if (list) list.push(n.id)
      else children.set(n.parentId, [n.id])
    } else {
      roots.push(n.id)
    }
  }

  const placed = new Map<number, Placed>()
  const edges: Layout['edges'] = []
  let slot = 0
  let maxDepth = 0
  const place = (id: number, depth: number): number => {
    maxDepth = Math.max(maxDepth, depth)
    const kids = (children.get(id) ?? []).filter((k) => !placed.has(k)) // защита от цикла
    let x: number
    if (kids.length === 0) {
      x = slot++
    } else {
      placed.set(id, { id, x: 0, depth }) // занять до обхода детей — на случай цикла
      const xs = kids.map((k) => {
        edges.push({ from: id, to: k })
        return place(k, depth + 1)
      })
      x = (xs[0]! + xs[xs.length - 1]!) / 2
    }
    placed.set(id, { id, x, depth })
    return x
  }
  for (const r of roots) place(r, 0)
  return { nodes: placed, edges, width: slot, depth: nodes.length === 0 ? 0 : maxDepth + 1 }
}
