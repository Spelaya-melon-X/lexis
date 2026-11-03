import { parseHash, toHash, type Tab } from './route'

// Текущий таб — $state, синхронизированный с location.hash: работают «назад/вперёд»,
// закладки и обновление страницы.
let state = $state(parseHash(location.hash))

window.addEventListener('hashchange', () => {
  state = parseHash(location.hash)
})

export const route = {
  get tab(): Tab {
    return state.tab
  },
  /** Хвост после таба: "#groups/12" → "12" (открытая группа). */
  get rest(): string {
    return state.rest
  },
  go(tab: Tab, rest = '') {
    location.hash = toHash(tab, rest) // hashchange обновит state
  },
}
