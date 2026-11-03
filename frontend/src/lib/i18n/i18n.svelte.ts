// Реактивный i18n на runes. t() читает $state-переменную языка, поэтому любой шаблон,
// вызвавший t(), перерисуется при смене языка — без store-подписок.
import { ApiError } from '../api/client'
import type { Locale } from '../api/types'
import en from './en.json'
import ru from './ru.json'
import { has, translate, type Dict, type Params } from './translate'

const dicts: Record<Locale, Dict> = { ru, en }
const STORAGE_KEY = 'lexis.locale'

// Ключи выводятся из ru.json — опечатка в t('tabs.ad') ловится svelte-check.
type Leaf<T, P extends string = ''> = {
  [K in keyof T & string]: T[K] extends string
    ? `${P}${K}`
    : T[K] extends { other: string }
      ? `${P}${K}`
      : Leaf<T[K], `${P}${K}.`>
}[keyof T & string]
export type TKey = Leaf<typeof ru>

function stored(): Locale {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'ru' || v === 'en') return v
  } catch {
    // приватный режим / нет localStorage — не страшно
  }
  return 'ru'
}

let current = $state<Locale>(stored())

/**
 * Текущий язык. Источник правды — /api/settings (его выставляет App при старте);
 * localStorage — только чтобы первый кадр сразу был на нужном языке.
 */
export const i18n = {
  get locale(): Locale {
    return current
  },
  set locale(l: Locale) {
    current = l
    document.documentElement.lang = l
    try {
      localStorage.setItem(STORAGE_KEY, l)
    } catch {
      // см. stored()
    }
  },
}

export function t(key: TKey, params?: Params): string {
  return translate(dicts[current], current, key, params)
}

/** Текст ошибки для пользователя: по коду ApiError из errors.*, иначе общий. */
export function errorText(e: unknown): string {
  if (e instanceof ApiError && has(dicts[current], `errors.${e.code}`)) {
    return translate(dicts[current], current, `errors.${e.code}`)
  }
  return t('errors.unknown')
}

/** Текст ошибки строки парсера (LineError.code). */
export function lineErrorText(code: string): string {
  return has(dicts[current], `errors.${code}`) ? translate(dicts[current], current, `errors.${code}`) : code
}

/** Есть ли ключ в текущем словаре — для ключей из данных (коды достижений с бэка). */
export function hasKey(key: string): key is TKey {
  return has(dicts[current], key)
}
