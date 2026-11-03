// Навигация без роутера (§2): табы в #hash. Чистая часть — для тестов.

export const TABS = ['add', 'groups', 'training', 'stats', 'settings'] as const
export type Tab = (typeof TABS)[number]

export const DEFAULT_TAB: Tab = 'add'

/** "#groups" → "groups"; "#groups/12" → "groups" (хвост — параметры таба); мусор → по умолчанию. */
export function parseHash(hash: string): { tab: Tab; rest: string } {
  const [head = '', ...rest] = hash.replace(/^#\/?/, '').split('/')
  const tab = (TABS as readonly string[]).includes(head) ? (head as Tab) : DEFAULT_TAB
  return { tab, rest: tab === head ? rest.join('/') : '' }
}

export function toHash(tab: Tab, rest = ''): string {
  return rest ? `#${tab}/${rest}` : `#${tab}`
}
