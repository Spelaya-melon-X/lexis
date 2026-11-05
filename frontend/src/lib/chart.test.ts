import { describe, expect, it } from 'vitest'
import { bars, plot, slotCenter, xOf } from './chart'

const box = { width: 100, height: 50, padX: 0, padY: 0 }

describe('plot', () => {
  it('maps 0..1 to bottom..top and spreads days evenly', () => {
    const { points, path } = plot([0, 0.5, 1], box)
    expect(points).toEqual([
      { index: 0, value: 0, x: 0, y: 50 },
      { index: 1, value: 0.5, x: 50, y: 25 },
      { index: 2, value: 1, x: 100, y: 0 },
    ])
    expect(path).toBe('M0 50 L50 25 L100 0')
  })

  it('skips null days but keeps their x slot', () => {
    const { points, path } = plot([1, null, null, 0], box)
    expect(points.map((p) => [p.index, p.x])).toEqual([
      [0, 0],
      [3, 100],
    ])
    expect(path).toBe('M0 0 L100 50')
  })

  it('all null → nothing to draw', () => {
    expect(plot([null, null], box)).toEqual({ points: [], path: '' })
  })

  it('single value is centered', () => {
    expect(plot([0.5], box).points[0]).toMatchObject({ x: 50, y: 25 })
  })

  it('respects padding', () => {
    const { points } = plot([0, 1], { width: 100, height: 60, padX: 10, padY: 5 })
    expect(points.map((p) => [p.x, p.y])).toEqual([
      [10, 55],
      [90, 5],
    ])
  })

  it('clamps out-of-range values', () => {
    expect(plot([1.5, -1], box).points.map((p) => p.y)).toEqual([0, 50])
  })

  it('rounds coordinates to 0.1', () => {
    expect(plot([0, 0, 1 / 3], { width: 10, height: 10, padX: 0, padY: 0 }).points[2]).toMatchObject({ x: 10, y: 6.7 })
  })
})

describe('xOf', () => {
  it('matches plot x', () => {
    expect(xOf(3, 4, box)).toBe(100)
    expect(xOf(0, 1, box)).toBe(50)
  })
})


describe('bars', () => {
  const b = { width: 100, height: 50, padX: 0, padY: 0 }
  it('max fills the height, zeros are skipped', () => {
    expect(bars([0, 5, 10, 0], b, 0)).toEqual([
      { index: 1, x: 25, y: 25, w: 25, h: 25 },
      { index: 2, x: 50, y: 0, w: 25, h: 50 },
    ])
  })
  it('gap shrinks bars and centers them in the slot', () => {
    expect(bars([1], b, 0.5)).toEqual([{ index: 0, x: 25, y: 0, w: 50, h: 50 }])
  })
  it('all zeros / empty → no bars', () => {
    expect(bars([0, 0], b)).toEqual([])
    expect(bars([], b)).toEqual([])
  })
  it('slotCenter is the middle of the slot', () => {
    expect(slotCenter(0, 4, b)).toBe(12.5)
    expect(slotCenter(3, 4, b)).toBe(87.5)
  })
})

describe('bars with a fixed scale', () => {
  const box = { width: 100, height: 70, padX: 0, padY: 0 }
  it('scales to scaleMax, not to the data maximum', () => {
    const b = bars([2, 7, 0], box, 0, 7)
    expect(b.map((x) => x.h)).toEqual([20, 70])
  })
  it('clamps values above scaleMax', () => {
    expect(bars([14], box, 0, 7)[0]!.h).toBe(70)
  })
  it('without scaleMax keeps relative scaling', () => {
    expect(bars([2, 4], box, 0).map((x) => x.h)).toEqual([35, 70])
  })
})
