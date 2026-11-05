import { describe, expect, it } from 'vitest'
import { clampStart, defaultBlocks, firstFree, formatTime, hourTicks, parseTime, place, resize, resolve, snap, type Slot } from './plan'

const s = (id: number, start: number, blocks: number): Slot => ({ id, start, blocks })
// «id@ЧЧ:ММ+блоки» по времени — удобно сравнивать раскладки целиком
const show = (slots: Slot[]) => [...slots].sort((a, b) => a.start - b.start).map((x) => `${x.id}@${formatTime(x.start)}+${x.blocks}`)

describe('resolve', () => {
  it('no overlaps — untouched', () => {
    expect(show(resolve([s(1, 600, 2), s(2, 540, 1)]))).toEqual(['2@9:00+1', '1@10:00+2'])
  })
  it('later items are pushed down, earlier never move', () => {
    expect(show(resolve([s(1, 540, 4), s(2, 600, 1), s(3, 630, 1)]))).toEqual(['1@9:00+4', '2@11:00+1', '3@11:30+1'])
  })
  it('equal starts keep input order', () => {
    expect(show(resolve([s(2, 540, 1), s(1, 540, 1)]))).toEqual(['2@9:00+1', '1@9:30+1'])
  })
})

describe('place', () => {
  // 1: 9:00–10:00, 2: 10:00–12:00, 3: 14:00–14:30
  const plan = [s(1, 540, 2), s(2, 600, 4), s(3, 840, 1)]

  it('into free time — just moves', () => {
    expect(show(place(plan, 3, 780))).toEqual(['1@9:00+2', '2@10:00+4', '3@13:00+1'])
  })
  it('onto an item of the same length — swap', () => {
    const p = [s(1, 540, 2), s(2, 660, 2)]
    expect(show(place(p, 1, 690))).toEqual(['2@9:00+2', '1@11:00+2'])
  })
  it('short dragged onto long below — swap, rest pushed', () => {
    // 1 (час) уронили на 2 (два часа): 1 → 10:00, 2 → 9:00 и тянется до 11:00 → 1 сдвигается на 11:00
    expect(show(place(plan, 1, 630))).toEqual(['2@9:00+4', '1@11:00+2', '3@14:00+1'])
  })
  it('long dragged onto short above — swap', () => {
    expect(show(place(plan, 2, 540))).toEqual(['2@9:00+4', '1@11:00+2', '3@14:00+1'])
  })
  it('overlapping two — swaps with the one whose start is closest', () => {
    // 3 (полчаса) на 11:30–12:00 задевает только 2 → меняется с ним
    expect(show(place(plan, 3, 690))).toEqual(['1@9:00+2', '3@10:00+1', '2@14:00+4'])
  })
  it('keyboard step down into the next item swaps them', () => {
    const p = [s(1, 540, 1), s(2, 570, 1)]
    expect(show(place(p, 1, 570))).toEqual(['2@9:00+1', '1@9:30+1'])
  })
  it('only overlaps itself — moves freely', () => {
    expect(show(place([s(1, 540, 4)], 1, 570))).toEqual(['1@9:30+4'])
  })
  it('unknown id — unchanged', () => {
    expect(place(plan, 42, 0)).toBe(plan)
  })
})

describe('resize', () => {
  it('growing pushes the following items', () => {
    expect(show(resize([s(1, 540, 2), s(2, 600, 1)], 1, 4))).toEqual(['1@9:00+4', '2@11:00+1'])
  })
  it('shrinking leaves a gap', () => {
    expect(show(resize([s(1, 540, 4), s(2, 660, 1)], 1, 1))).toEqual(['1@9:00+1', '2@11:00+1'])
  })
})

describe('firstFree', () => {
  const plan = [s(1, 540, 2), s(2, 660, 2)] // 9–10, 11–12
  it.each([
    { blocks: 2, from: 540, want: 600 }, // окно 10–11
    { blocks: 3, from: 540, want: 720 }, // полтора часа — только после 12
    { blocks: 1, from: 630, want: 630 },
    { blocks: 1, from: 540, want: 600 },
  ])('$blocks blocks from $from', ({ blocks, from, want }) => {
    expect(firstFree(plan, blocks, from, 1380)).toBe(want)
  })
  it('no room before the day end — right after the last item', () => {
    expect(firstFree(plan, 4, 540, 780)).toBe(720)
  })
  it('empty plan — from', () => {
    expect(firstFree([], 2, 540, 1380)).toBe(540)
  })
})

describe('snap / clampStart', () => {
  it.each([
    [544, 540],
    [556, 570],
    [585, 600],
  ])('snap(%i) = %i', (m, want) => {
    expect(snap(m)).toBe(want)
  })
  it('clampStart keeps the task inside the day', () => {
    expect([clampStart(1430, 2), clampStart(-30, 1), clampStart(400, 1, 540)]).toEqual([1380, 0, 540])
  })
})

describe('time', () => {
  it.each([
    [570, '9:30'],
    [0, '0:00'],
    [1440, '24:00'],
  ])('formatTime(%i)', (min, str) => {
    expect(formatTime(min)).toBe(str)
  })
  it.each([
    ['09:30', 570],
    ['9:00', 540],
    ['24:00', 1440],
    ['24:30', null],
    ['9:60', null],
    ['', null],
    ['утро', null],
  ])('parseTime(%s)', (str, want) => {
    expect(parseTime(str)).toBe(want)
  })
})

describe('hourTicks', () => {
  it('whole hours inclusive', () => {
    expect(hourTicks(540, 720)).toEqual([540, 600, 660, 720])
  })
  it('half-hour start rounds up', () => {
    expect(hourTicks(570, 690)).toEqual([600, 660])
  })
})

describe('defaultBlocks', () => {
  it.each([
    [3, 3],
    [60, 40],
    [0, 1],
  ])('%i → %i', (b, want) => {
    expect(defaultBlocks(b)).toBe(want)
  })
})
