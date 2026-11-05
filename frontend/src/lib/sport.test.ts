import { describe, expect, it } from 'vitest'
import type { Exercise, Workout } from './api/types'
import { draftOf, entryTonnage, findExercise, findMeal, newEntry, sumKcal, toInput, weekSummary, weightParts, withLast } from './sport'

const squat: Exercise = { id: 1, name: 'Присед', uses: 3, lastDay: '2026-11-03', last: { sets: 5, reps: 5, weight: 85 } }
const pull: Exercise = { id: 2, name: 'Подтягивания', uses: 1, lastDay: '2026-11-03', last: { sets: 4, reps: 8, weight: null } }

describe('findExercise / withLast', () => {
  it.each([
    ['присед', 1],
    ['  ПРИСЕД ', 1],
    ['Жим', undefined],
    ['', undefined],
  ])('find %s', (name, id) => {
    expect(findExercise([squat, pull], name)?.id).toBe(id)
  })
  it('fills «as last time»', () => {
    expect(withLast({ ...newEntry(), name: 'Присед' }, squat)).toMatchObject({ sets: '5', reps: '5', weight: '85' })
    expect(withLast({ ...newEntry(), name: 'Подтягивания' }, pull)).toMatchObject({ sets: '4', reps: '8', weight: '' })
  })
  it('keeps numbers typed by hand and unknown exercises', () => {
    const typed = { ...newEntry(), sets: '2', touched: true }
    expect(withLast(typed, squat)).toBe(typed)
    const fresh = newEntry()
    expect(withLast(fresh, undefined)).toBe(fresh)
  })
})

describe('toInput', () => {
  const base = { day: '2026-11-05', title: 'Ноги', notes: '', durationMin: '60', kcal: '450' }
  it('rows → API, empty rows skipped, «62,5» accepted', () => {
    expect(
      toInput(base, [
        { name: ' Присед ', sets: '3', reps: '5', weight: '62,5', touched: true },
        { name: 'Подтягивания', sets: '4', reps: '8', weight: '', touched: false },
        { ...newEntry(), name: '  ' },
      ]),
    ).toEqual({
      day: '2026-11-05', title: 'Ноги', notes: '', durationMin: 60, kcal: 450,
      exercises: [
        { name: 'Присед', sets: 3, reps: 5, weight: 62.5 },
        { name: 'Подтягивания', sets: 4, reps: 8, weight: null },
      ],
    })
  })
  it.each([
    { what: 'kcal', patch: { kcal: 'много' }, rows: [], want: { error: 'invalid_kcal' } },
    { what: 'duration', patch: { durationMin: '1,5' }, rows: [], want: { error: 'invalid_duration' } },
    { what: 'zero sets', patch: {}, rows: [{ ...newEntry(), name: 'Жим', sets: '0' }], want: { error: 'invalid_set', entry: 0 } },
    { what: 'bad weight', patch: {}, rows: [{ ...newEntry(), name: 'Жим', weight: 'тяжело' }], want: { error: 'invalid_set', entry: 0 } },
  ])('$what', ({ patch, rows, want }) => {
    expect(toInput({ ...base, ...patch }, rows)).toEqual(want)
  })
})

describe('tonnage', () => {
  it.each([
    [{ sets: '3', reps: '10', weight: '60' }, 1800],
    [{ sets: '4', reps: '8', weight: '' }, 0],
    [{ sets: '', reps: '8', weight: '20' }, 0],
  ])('%o', (e, want) => {
    expect(entryTonnage(e)).toBe(want)
  })
  it.each([
    [820, { value: 820, unit: 'kg' }],
    [1240, { value: 1.2, unit: 't' }],
    [12560, { value: 12.6, unit: 't' }],
  ])('weightParts(%i)', (kg, want) => {
    expect(weightParts(kg)).toEqual(want)
  })
})

const w = (id: number, day: string, extra: Partial<Workout> = {}): Workout => ({
  id, day, title: 't', notes: '', durationMin: null, kcal: null, exercises: [], tonnage: 0, createdAt: '', updatedAt: '', ...extra,
})

describe('weekSummary', () => {
  const days = ['2026-11-02', '2026-11-03', '2026-11-04']
  const list = [
    w(1, '2026-11-02', { kcal: 400, durationMin: 60, tonnage: 1500, exercises: [
      { exerciseId: 1, name: 'Присед', sets: 3, reps: 5, weight: 80, tonnage: 1200 },
      { exerciseId: 2, name: 'Подтягивания', sets: 4, reps: 8, weight: null, tonnage: 0 },
      { exerciseId: 3, name: 'Жим', sets: 2, reps: 5, weight: 30, tonnage: 300 },
    ] }),
    w(2, '2026-11-04', { kcal: 300, tonnage: 1700, exercises: [
      { exerciseId: 1, name: 'Присед', sets: 4, reps: 5, weight: 85, tonnage: 1700 },
    ] }),
    w(3, '2026-11-04', { durationMin: 30 }), // бег без ккал и весов
  ]
  const s = weekSummary(list, days)
  it('totals and averages count only workouts that have the value', () => {
    expect([s.workouts, s.kcal, s.avgKcal, s.durationMin, s.avgDuration, s.tonnage, s.avgTonnage]).toEqual([3, 700, 350, 90, 45, 3200, 1600])
  })
  it('every day of the week is present', () => {
    expect(s.days.map((d) => [d.day, d.workouts, d.kcal, d.tonnage])).toEqual([
      ['2026-11-02', 1, 400, 1500],
      ['2026-11-03', 0, 0, 0],
      ['2026-11-04', 2, 300, 1700],
    ])
  })
  it('exercises by tonnage', () => {
    expect(s.exercises).toEqual([
      { name: 'Присед', times: 2, sets: 7, reps: 35, maxWeight: 85, tonnage: 2900 },
      { name: 'Жим', times: 1, sets: 2, reps: 10, maxWeight: 30, tonnage: 300 },
      { name: 'Подтягивания', times: 1, sets: 4, reps: 32, maxWeight: null, tonnage: 0 },
    ])
  })
  it('empty week', () => {
    expect(weekSummary([], days)).toMatchObject({ workouts: 0, avgKcal: null, avgTonnage: null, exercises: [] })
  })
})

describe('draftOf', () => {
  it('workout → touched rows', () => {
    expect(draftOf(w(1, '2026-11-02', { exercises: [{ exerciseId: 2, name: 'Подтягивания', sets: 4, reps: 8, weight: null, tonnage: 0 }] }))).toEqual([
      { name: 'Подтягивания', sets: '4', reps: '8', weight: '', touched: true },
    ])
  })
})

describe('meals', () => {
  const sug = [{ text: 'Овсянка', kcal: 350 }, { text: 'Яблоко', kcal: null }]
  it.each([
    ['овсянка', 350],
    ['  ОВСЯНКА ', 350],
    ['Яблоко', null],
  ])('findMeal(%s)', (text, kcal) => {
    expect(findMeal(sug, text)?.kcal).toBe(kcal)
  })
  it('unknown / empty', () => {
    expect(findMeal(sug, 'Каша')).toBeUndefined()
    expect(findMeal(sug, ' ')).toBeUndefined()
  })
  it('sumKcal skips unknown', () => {
    expect(sumKcal([{ text: 'a', kcal: 300 }, { text: 'b', kcal: null }, { text: 'c', kcal: 50 }])).toBe(350)
  })
})
