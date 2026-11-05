import { parseHash, SECTIONS, toHash, type Page, type Route, type Section } from './route'

// Текущий раздел/подраздел — $state, синхронизированный с location.hash: работают «назад/вперёд»,
// закладки и обновление страницы.

const STORAGE_KEY = 'lexis.lastPages'

/** Последний открытый адрес каждого раздела: клик по разделу в шапке возвращает туда, где был. */
function loadLast(): Partial<Record<Section, string>> {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}')
    return v && typeof v === 'object' ? (v as Partial<Record<Section, string>>) : {}
  } catch {
    return {}
  }
}

let state = $state<Route>(sync())
const last = $state(loadLast())

/**
 * Разбирает хэш и приводит адрес к каноническому виду: старые "#groups/12" и мусор заменяются
 * через replaceState — без лишней записи в истории и без нового hashchange.
 */
function sync(): Route {
  const r = parseHash(location.hash)
  const canonical = toHash(r)
  if (location.hash !== canonical) history.replaceState(history.state, '', canonical)
  return r
}

function remember(r: Route) {
  last[r.section] = toHash(r)
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(last))
  } catch {
    // приватный режим / нет localStorage — просто не запоминаем
  }
}

remember(state)
window.addEventListener('hashchange', () => {
  state = sync()
  remember(state)
})

export const route = {
  get section(): Section {
    return state.section
  },
  get page(): Page {
    return state.page
  },
  /** Хвост после подраздела: "#/english/groups/12" → "12" (открытая группа). */
  get rest(): string {
    return state.rest
  },
  go<S extends Section>(section: S, page: Page<S>, rest = '') {
    location.hash = toHash({ section, page, rest } as Route) // hashchange обновит state
  },
  /** Адрес для ссылки на раздел: последний открытый в нём подраздел или первый. */
  sectionHref(section: Section): string {
    const saved = last[section]
    // В localStorage могло остаться что угодно — проверяем, что это действительно этот раздел.
    if (saved && parseHash(saved).section === section) return saved
    return toHash({ section, page: SECTIONS[section][0], rest: '' } as Route)
  },
}
