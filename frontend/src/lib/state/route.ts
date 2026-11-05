// Навигация без роутера (§2, §17.1): раздел + подраздел в #hash вида "#/english/groups/12".
// Чистая часть — для тестов.

/** Разделы (шапка) и их подразделы (сайдбар). Первый подраздел — по умолчанию. */
export const SECTIONS = {
  english: ['add', 'groups', 'training', 'stats', 'settings'],
  day: ['entry', 'calendar', 'week', 'stats'],
  habits: ['check', 'calendar', 'goals', 'notes', 'manage'],
  // Глобальные настройки (⚙ справа в шапке, не в списке разделов). Старый «#settings» — это
  // настройки английского (v1), он по-прежнему ведёт в #/english/settings.
  settings: ['general', 'data'],
} as const

export type Section = keyof typeof SECTIONS
export type Page<S extends Section = Section> = (typeof SECTIONS)[S][number]

/** Размеченное объединение: page проверяется по своему разделу (english + 'goals' — ошибка типов). */
export type Route = { [S in Section]: { section: S; page: Page<S>; rest: string } }[Section]

export const DEFAULT_SECTION: Section = 'english'

const isSection = (s: string): s is Section => Object.hasOwn(SECTIONS, s)

function isPage<S extends Section>(section: S, p: string): p is Page<S> {
  return (SECTIONS[section] as readonly string[]).includes(p)
}

function route<S extends Section>(section: S, page?: string, rest: string[] = []): Route {
  if (page !== undefined && isPage(section, page)) {
    return { section, page, rest: rest.join('/') } as Route
  }
  // Подраздела нет или он чужой — хвост от него не переносим.
  return { section, page: SECTIONS[section][0], rest: '' } as Route
}

/**
 * "#/english/groups/12" → {english, groups, "12"}. Неизвестный подраздел → первый в разделе,
 * мусор → английский по умолчанию.
 * Старые хэши v1 ("#groups/12", "#/groups") — это подразделы английского: разбираем их туда же,
 * а route.svelte.ts заменяет адрес на канонический (toHash).
 */
export function parseHash(hash: string): Route {
  const [head = '', ...tail] = hash.replace(/^#\/?/, '').split('/')
  // Новый формат всегда со слэшем ("#/settings"), старые v1 — без ("#settings" — это настройки
  // английского). Разделы v2 проверяем первыми только для "#/…", иначе «settings» v1 уехал бы в ⚙.
  const modern = hash.startsWith('#/')
  if (modern && isSection(head)) {
    const [page, ...rest] = tail
    return route(head, page, rest)
  }
  if (isPage('english', head)) return route('english', head, tail)
  if (isSection(head)) return route(head, tail[0], tail.slice(1))
  return route(DEFAULT_SECTION)
}

export function toHash(r: Route): string {
  const base = `#/${r.section}/${r.page}`
  return r.rest ? `${base}/${r.rest}` : base
}
