import { api, ApiError } from '../api/client'
import type { FocusSession, FocusTimeline } from '../api/types'
import { timeline } from '../focus'
import { errorText } from '../i18n/i18n.svelte'
import { route } from './route.svelte'
import { toasts } from './toasts.svelte'

// Активная фокус-сессия (§18.3, B4) — одна на всё приложение: её видит и страница «Фокус», и плашка в шапке.
// Таймеры тикают раз в секунду от серверных отметок; offset — разница часов сервера и браузера
// (сервер отдаёт now в каждом ответе), так что перезагрузка или спящий ноутбук ничего не сбивают.
// Перечитывается при старте, возврате во вкладку и раз в минуту — на случай действий из другой вкладки.

const REFRESH_MS = 60 * 1000

let session = $state<FocusSession | null>(null)
let offset = $state(0)
let tick = $state(Date.now())
let started = false
let seq = 0

function set(s: FocusSession | null, serverNow: string) {
  offset = Date.parse(serverNow) - Date.now()
  session = s && !s.finishedAt ? s : null
}

async function refresh() {
  const my = ++seq
  try {
    const r = await api.focus.active()
    if (my === seq) set(r.session, r.now)
  } catch {
    // плашка вспомогательная: сервер недоступен — показываем, что было
  }
}

export const focus = {
  get session() {
    return session
  },
  /** Разбивка «сейчас» по часам сервера; null — сессии нет. */
  get timeline(): FocusTimeline | null {
    return session ? timeline(session, tick + offset) : null
  },
  /** «Сейчас» по часам сервера, мс. */
  get now() {
    return tick + offset
  },
  /** Ответ любого действия над сессией: завершённая — снимается. */
  apply(s: FocusSession) {
    ++seq // свежий ответ действия важнее летящего фонового refresh
    set(s, s.now)
  },
  refresh: () => void refresh(),
  /**
   * «▶ Начать» (распорядок, карточка дела): новая сессия и переход на «Фокус».
   * Уже идёт другая — объясняем и ведём к ней, а не молча теряем клик.
   */
  async begin(taskId: number) {
    try {
      this.apply(await api.focus.start(taskId))
    } catch (e) {
      toasts.error(errorText(e))
      if (!(e instanceof ApiError && e.code === 'focus_active')) return
      void refresh()
    }
    route.go('game', 'focus')
  },
  /** Один раз из App. */
  start() {
    if (started) return
    started = true
    void refresh()
    window.addEventListener('focus', () => void refresh())
    setInterval(() => void refresh(), REFRESH_MS)
    setInterval(() => (tick = Date.now()), 1000)
  },
}
