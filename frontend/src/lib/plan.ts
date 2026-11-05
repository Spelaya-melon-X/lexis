// Шкала распорядка дня (§18.3, B3): дело ставится в любое время по получасам. Пересекаться пункты
// не могут: перетащенное дело, наложившееся на другое, меняется с ним местами, а если из-за разной
// длины что-то всё ещё пересекается — последующие дела сдвигаются вниз. Чистые функции — для тестов.

export const BLOCK_MIN = 30
export const DAY_MIN = 24 * 60

/** Пункт на шкале: id дела, начало (минуты от полуночи) и объём в блоках. */
export interface Slot {
  id: number
  start: number
  blocks: number
}

export const endOf = (s: Slot) => s.start + s.blocks * BLOCK_MIN

const overlaps = (a: Slot, b: Slot) => a.start < endOf(b) && b.start < endOf(a)

/** Округлить к ближайшему получасу. */
export function snap(min: number): number {
  return Math.round(min / BLOCK_MIN) * BLOCK_MIN
}

/** Начало так, чтобы дело целиком помещалось в [lo, сутки]. */
export function clampStart(start: number, blocks: number, lo = 0): number {
  return Math.max(lo, Math.min(start, DAY_MIN - blocks * BLOCK_MIN))
}

/**
 * Убрать пересечения: по времени начала (равные — в исходном порядке) каждое дело
 * начинается не раньше конца предыдущего. Ранние дела не двигаются никогда.
 */
export function resolve(slots: Slot[]): Slot[] {
  const sorted = slots.map((s, i) => ({ s, i })).sort((a, b) => a.s.start - b.s.start || a.i - b.i)
  let prevEnd = -Infinity
  return sorted.map(({ s }) => {
    const start = Math.max(s.start, prevEnd)
    prevEnd = start + s.blocks * BLOCK_MIN
    return start === s.start ? s : { ...s, start }
  })
}

/**
 * Поставить дело id на время start. Наложилось на другое — меняются местами: перетащенное
 * встаёт на начало того, на которое уронили (ближайшее по началу из перекрытых), а то — на
 * освободившееся место. Оставшиеся пересечения (дела разной длины) снимает resolve.
 */
export function place(slots: Slot[], id: number, start: number): Slot[] {
  const moving = slots.find((s) => s.id === id)
  if (!moving) return slots
  const others = slots.filter((s) => s.id !== id)
  const moved = { ...moving, start }
  const hits = others.filter((o) => overlaps(moved, o))
  if (hits.length === 0) return resolve([...others, moved])
  const target = hits.reduce((a, b) => (Math.abs(b.start - start) < Math.abs(a.start - start) ? b : a))
  const swapped = others.map((o) => (o.id === target.id ? { ...o, start: moving.start } : o))
  return resolve([...swapped, { ...moving, start: target.start }])
}

/** Изменить объём дела; разросшееся сдвигает последующие вниз. */
export function resize(slots: Slot[], id: number, blocks: number): Slot[] {
  return resolve(slots.map((s) => (s.id === id ? { ...s, blocks } : s)))
}

/**
 * Первое свободное время не раньше from, куда помещается дело на blocks блоков и заканчивается
 * до to. Места нет — сразу после последнего дела (может вылезти за конец дня — шкала подсветит).
 */
export function firstFree(slots: Slot[], blocks: number, from: number, to: number): number {
  const probe = { id: -1, start: from, blocks }
  for (let t = from; t + blocks * BLOCK_MIN <= to; t += BLOCK_MIN) {
    probe.start = t
    if (!slots.some((s) => overlaps(probe, s))) return t
  }
  return clampStart(Math.max(from, ...slots.map(endOf)), blocks)
}

/** 570 → "9:30", 1440 → "24:00". */
export function formatTime(min: number): string {
  return `${Math.floor(min / 60)}:${String(min % 60).padStart(2, '0')}`
}

/** "09:30" → 570; мусор → null. */
export function parseTime(s: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s)
  if (!m) return null
  const h = Number(m[1])
  const min = Number(m[2])
  return h <= 24 && min < 60 && h * 60 + min <= DAY_MIN ? h * 60 + min : null
}

/** Часовые отметки шкалы: целые часы от начала до конца включительно. */
export function hourTicks(dayStart: number, dayEnd: number): number[] {
  const out: number[] = []
  for (let t = Math.ceil(dayStart / 60) * 60; t <= dayEnd; t += 60) out.push(t)
  return out
}

/** Сколько займёт новое дело: его объём, но не больше 40 блоков (столько можно в один день). */
export function defaultBlocks(taskBlocks: number): number {
  return Math.min(Math.max(taskBlocks, 1), 40)
}
