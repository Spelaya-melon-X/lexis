// Дерево групп английского (§18.2): раскладка для списка, проверки переноса.
// Бэк отдаёт группы плоско (parentId) и отсортированными по имени — порядок соседей сохраняем.
// Правила те же, что в Go (groups/tree): не в себя и не в потомка, глубина ≤ MAX_DEPTH.

/** Сколько уровней можно: корень → подгруппа → под-подгруппа. */
export const MAX_DEPTH = 3

export interface Node {
  id: number
  parentId: number | null
}

/** Строка списка: группа + отступ. */
export interface Row<G extends Node> {
  group: G
  depth: number // 0 — корень
  hasChildren: boolean
}

/** Дети по id родителя (null — корневые), в исходном порядке. */
export function childrenOf<G extends Node>(groups: G[]): Map<number | null, G[]> {
  const ids = new Set(groups.map((g) => g.id))
  const m = new Map<number | null, G[]>()
  for (const g of groups) {
    // Родителя нет в списке (не должно быть, но список мог устареть) — показываем в корне, а не теряем.
    const p = g.parentId !== null && ids.has(g.parentId) ? g.parentId : null
    const list = m.get(p)
    if (list) list.push(g)
    else m.set(p, [g])
  }
  return m
}

/** Плоский список для рендера: обход в глубину, у свёрнутых групп дети не показываются. */
export function visibleRows<G extends Node>(groups: G[], collapsed: ReadonlySet<number> = new Set()): Row<G>[] {
  const kids = childrenOf(groups)
  const rows: Row<G>[] = []
  const seen = new Set<number>() // защита от цикла в данных
  const walk = (parent: number | null, depth: number) => {
    for (const g of kids.get(parent) ?? []) {
      if (seen.has(g.id)) continue
      seen.add(g.id)
      const hasChildren = (kids.get(g.id)?.length ?? 0) > 0
      rows.push({ group: g, depth, hasChildren })
      if (hasChildren && !collapsed.has(g.id)) walk(g.id, depth + 1)
    }
  }
  walk(null, 0)
  return rows
}

/** Путь от корня до группы включительно (хлебные крошки). Неизвестный id — []. */
export function pathTo<G extends Node>(groups: G[], id: number): G[] {
  const byId = new Map(groups.map((g) => [g.id, g]))
  const path: G[] = []
  for (let cur = byId.get(id); cur && path.length <= groups.length; cur = cur.parentId === null ? undefined : byId.get(cur.parentId)) {
    path.unshift(cur)
  }
  return path
}

/** Глубина группы: корневая = 1, null (корень дерева) = 0. */
export function depthOf(groups: Node[], id: number | null): number {
  return id === null ? 0 : pathTo(groups, id).length
}

/** Сама группа и все потомки. */
export function subtree(groups: Node[], id: number): Set<number> {
  const kids = childrenOf(groups)
  const out = new Set<number>([id])
  const stack = [id]
  while (stack.length > 0) {
    for (const c of kids.get(stack.pop()!) ?? []) {
      if (!out.has(c.id)) {
        out.add(c.id)
        stack.push(c.id)
      }
    }
  }
  return out
}

/** Число уровней в поддереве: лист = 1. */
export function heightOf(groups: Node[], id: number): number {
  const kids = childrenOf(groups)
  const h = (n: number, guard: number): number =>
    guard > groups.length ? 0 : 1 + Math.max(0, ...(kids.get(n) ?? []).map((c) => h(c.id, guard + 1)))
  return h(id, 0)
}

/**
 * Можно ли положить группу id в target (null — в корень). Перенос на то же место — false:
 * нечего делать, и цель не надо подсвечивать.
 */
export function canMove(groups: Node[], id: number, target: number | null): boolean {
  const g = groups.find((x) => x.id === id)
  if (!g || g.parentId === target) return false
  if (target !== null && subtree(groups, id).has(target)) return false
  return depthOf(groups, target) + heightOf(groups, id) <= MAX_DEPTH
}

/** Можно ли создать новую группу внутри parent (null — в корне). */
export function canCreateIn(groups: Node[], parent: number | null): boolean {
  return depthOf(groups, parent) + 1 <= MAX_DEPTH
}

/**
 * Куда упадёт перетаскиваемая группа: на середину строки — внутрь неё, на верхний/нижний край —
 * рядом, т.е. к тому же родителю (соседи сортируются по имени, так что «между» = «в того же родителя»).
 * y — позиция курсора внутри строки высотой height.
 */
export function dropParent(row: Node, y: number, height: number): { parent: number | null; into: boolean } {
  const edge = Math.min(10, height / 4)
  return y < edge || y > height - edge ? { parent: row.parentId, into: false } : { parent: row.id, into: true }
}
