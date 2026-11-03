// Геометрия SVG-графиков (§2: графики — свой SVG). Чистые функции — тестируются без DOM.

export interface Box {
  width: number
  height: number
  padX: number
  padY: number
}

export interface Point {
  index: number // номер значения во входном массиве (день)
  value: number // 0..1
  x: number
  y: number
}

const r1 = (n: number) => Math.round(n * 10) / 10

/**
 * Точки ряда значений 0..1 в прямоугольнике box. null (дня без ответов) — пропускается:
 * у такого дня нет точки, а линия соединяет соседние дни с данными — тренд виден
 * и при занятиях через день. Значения вне 0..1 прижимаются к краям.
 */
export function plot(values: (number | null)[], box: Box): { points: Point[]; path: string } {
  const n = values.length
  const innerW = box.width - 2 * box.padX
  const innerH = box.height - 2 * box.padY
  const xAt = (i: number) => (n <= 1 ? box.width / 2 : box.padX + (i * innerW) / (n - 1))

  const points: Point[] = []
  values.forEach((v, i) => {
    if (v === null || Number.isNaN(v)) return
    const value = Math.min(Math.max(v, 0), 1)
    points.push({ index: i, value, x: r1(xAt(i)), y: r1(box.padY + (1 - value) * innerH) })
  })
  const path = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' ')
  return { points, path }
}

/** X-координата i-го дня — для подписей оси, даже если значения в этот день нет. */
export function xOf(i: number, n: number, box: Box): number {
  return r1(n <= 1 ? box.width / 2 : box.padX + (i * (box.width - 2 * box.padX)) / (n - 1))
}

export interface Bar {
  index: number
  x: number
  y: number
  w: number
  h: number
}

/**
 * Столбики для неотрицательных значений: максимум — во всю высоту, 0 — без столбика.
 * gap — доля ширины слота под промежуток (0..1).
 */
export function bars(values: number[], box: Box, gap = 0.3): Bar[] {
  const n = values.length
  if (n === 0) return []
  const max = Math.max(...values, 0)
  const slot = (box.width - 2 * box.padX) / n
  const w = r1(slot * (1 - gap))
  const innerH = box.height - 2 * box.padY
  const out: Bar[] = []
  values.forEach((v, i) => {
    if (v <= 0 || max === 0) return
    const h = r1((v / max) * innerH)
    out.push({ index: i, x: r1(box.padX + i * slot + (slot - w) / 2), y: r1(box.height - box.padY - h), w, h })
  })
  return out
}

/** Центр i-го слота столбиков — чтобы точки линии вставали над столбиками. */
export function slotCenter(i: number, n: number, box: Box): number {
  return r1(box.padX + ((i + 0.5) * (box.width - 2 * box.padX)) / n)
}
