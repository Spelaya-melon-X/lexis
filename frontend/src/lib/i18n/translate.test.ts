import { describe, expect, it } from 'vitest'
import en from './en.json'
import ru from './ru.json'
import { has, interpolate, isPlural, leafKeys, translate, type Dict } from './translate'

const dict: Dict = {
  title: 'Lexis',
  greet: 'Привет, {name}!',
  nested: { deep: { key: 'глубоко' } },
  cards: { one: '{count} карточка', few: '{count} карточки', many: '{count} карточек', other: '{count} карточки' },
}

describe('translate', () => {
  it.each([
    { key: 'title', params: undefined, want: 'Lexis' },
    { key: 'greet', params: { name: 'Danik' }, want: 'Привет, Danik!' },
    { key: 'nested.deep.key', params: undefined, want: 'глубоко' },
    { key: 'missing.key', params: undefined, want: 'missing.key' },
    { key: 'nested', params: undefined, want: 'nested' }, // не лист
    { key: 'title.extra', params: undefined, want: 'title.extra' },
  ])('$key', ({ key, params, want }) => {
    expect(translate(dict, 'ru', key, params)).toBe(want)
  })

  // Русские формы: 1, 21 — one; 2–4, 22–24 — few; 0, 5–20, 25 — many.
  it.each([
    [0, '0 карточек'],
    [1, '1 карточка'],
    [2, '2 карточки'],
    [4, '4 карточки'],
    [5, '5 карточек'],
    [11, '11 карточек'],
    [12, '12 карточек'],
    [21, '21 карточка'],
    [22, '22 карточки'],
    [25, '25 карточек'],
    [111, '111 карточек'],
    [1.5, '1.5 карточки'], // дробные — other
  ])('ru plural %d', (count, want) => {
    expect(translate(dict, 'ru', 'cards', { count })).toBe(want)
  })

  it('en plural uses one/other', () => {
    const enDict: Dict = { cards: { one: '{count} card', other: '{count} cards' } }
    expect(translate(enDict, 'en', 'cards', { count: 1 })).toBe('1 card')
    expect(translate(enDict, 'en', 'cards', { count: 2 })).toBe('2 cards')
    expect(translate(enDict, 'en', 'cards', { count: 0 })).toBe('0 cards')
  })

  it('missing plural form falls back to other', () => {
    const d: Dict = { x: { one: 'один', other: 'много' } }
    expect(translate(d, 'ru', 'x', { count: 5 })).toBe('много')
  })
})

describe('interpolate', () => {
  it('leaves unknown placeholders visible', () => {
    expect(interpolate('{a} и {b}', { a: 1 })).toBe('1 и {b}')
  })
  it('no params — as is', () => {
    expect(interpolate('{a}')).toBe('{a}')
  })
})

describe('has', () => {
  it('finds strings and plurals, not branches', () => {
    expect(has(dict, 'title')).toBe(true)
    expect(has(dict, 'cards')).toBe(true)
    expect(has(dict, 'nested')).toBe(false)
    expect(has(dict, 'nope')).toBe(false)
  })
})

// Словари должны совпадать по ключам — иначе в одном из языков вылезет «nav.english.add» вместо текста.
describe('dictionaries', () => {
  it('ru and en have the same keys', () => {
    expect(leafKeys(en as Dict)).toEqual(leafKeys(ru as Dict))
  })

  it('every plural object has "other" and only known forms', () => {
    const forms = new Set(['zero', 'one', 'two', 'few', 'many', 'other'])
    const walk = (d: Dict, path: string) => {
      for (const [k, v] of Object.entries(d)) {
        if (typeof v === 'string') continue
        if (Object.keys(v).some((f) => forms.has(f))) {
          expect(isPlural(v), `${path}${k} must have "other"`).toBe(true)
          for (const f of Object.keys(v)) expect(forms.has(f), `${path}${k}.${f}`).toBe(true)
        } else {
          walk(v, `${path}${k}.`)
        }
      }
    }
    walk(ru as Dict, '')
    walk(en as Dict, '')
  })

  it('ru plurals have one/few/many', () => {
    const walk = (d: Dict): void => {
      for (const v of Object.values(d)) {
        if (typeof v === 'string') continue
        if (isPlural(v)) expect(Object.keys(v).sort()).toEqual(['few', 'many', 'one', 'other'])
        else walk(v)
      }
    }
    walk(ru as Dict)
  })
})
