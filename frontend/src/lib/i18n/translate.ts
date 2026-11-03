// Чистая часть i18n: поиск ключа, подстановка параметров, множественные формы.
// Реактивная обёртка (текущий язык) — в i18n.svelte.ts.

export type Params = Record<string, string | number>

/**
 * Словарь: вложенные объекты, листья — строки или объект множественных форм.
 * Объект форм узнаётся по ключу "other" (обязателен в обоих языках):
 *   "cards": { "one": "{count} карточка", "few": "{count} карточки", "many": "{count} карточек", "other": "{count} карточки" }
 * Формы выбирает Intl.PluralRules — правила языков уже встроены в браузер.
 */
export type Dict = { [key: string]: string | Dict }

export function isPlural(v: unknown): v is Partial<Record<Intl.LDMLPluralRule, string>> & { other: string } {
  return typeof v === 'object' && v !== null && typeof (v as { other?: unknown }).other === 'string'
}

function lookup(dict: Dict, key: string): string | Dict | undefined {
  let node: string | Dict | undefined = dict
  for (const part of key.split('.')) {
    if (typeof node !== 'object' || node === null) return undefined
    node = node[part]
  }
  return node
}

export function has(dict: Dict, key: string): boolean {
  const v = lookup(dict, key)
  return typeof v === 'string' || isPlural(v)
}

/** "{count} слов" + {count: 5} → "5 слов". Неизвестный плейсхолдер остаётся как есть — так опечатку видно. */
export function interpolate(s: string, params?: Params): string {
  if (!params) return s
  return s.replace(/\{(\w+)\}/g, (m, name: string) => (name in params ? String(params[name]) : m))
}

const pluralRules = new Map<string, Intl.PluralRules>()

function pluralCategory(locale: string, n: number): Intl.LDMLPluralRule {
  let rules = pluralRules.get(locale)
  if (!rules) {
    rules = new Intl.PluralRules(locale)
    pluralRules.set(locale, rules)
  }
  return rules.select(n)
}

/**
 * Перевод ключа. Нет ключа — возвращаем сам ключ: в UI сразу видно, чего не хватает,
 * а приложение не падает.
 */
export function translate(dict: Dict, locale: string, key: string, params?: Params): string {
  const v = lookup(dict, key)
  if (typeof v === 'string') return interpolate(v, params)
  if (isPlural(v)) {
    const count = typeof params?.count === 'number' ? params.count : 0
    const form = v[pluralCategory(locale, count)] ?? v.other
    return interpolate(form, params)
  }
  return key
}

/** Все пути к листьям (строкам и объектам форм) — для теста «в en те же ключи, что в ru». */
export function leafKeys(dict: Dict, prefix = ''): string[] {
  const out: string[] = []
  for (const [k, v] of Object.entries(dict)) {
    const path = prefix + k
    if (typeof v === 'string' || isPlural(v)) out.push(path)
    else out.push(...leafKeys(v, `${path}.`))
  }
  return out.sort()
}
