import { api } from '../api/client'
import { today } from '../dates'
import { dayDue, habitsDue } from '../reminders'

// Напоминания в шапке (§17.11): точка на «Дне», счётчик на «Привычках».
// Обновляются при старте, переходах, возврате во вкладку, раз в 5 минут и после
// сохранения оценки/отметок (страницы зовут refresh сами). Системных уведомлений нет (§9).

const INTERVAL_MS = 5 * 60 * 1000

let state = $state({ dayDue: false, habitsDue: 0 })
let started = false
let seq = 0

async function refresh() {
  const my = ++seq
  const d = today()
  try {
    const [day, habits, checkins] = await Promise.all([api.journal.day(d), api.habits.list(), api.habits.checkins(d, d)])
    if (my !== seq) return // пока ждали, ушёл более свежий запрос
    state = { dayDue: dayDue(new Date(), day.rating !== null), habitsDue: habitsDue(habits, checkins) }
  } catch {
    // напоминания вспомогательные: сервер недоступен — просто не показываем
  }
}

export const reminders = {
  get dayDue() {
    return state.dayDue
  },
  get habitsDue() {
    return state.habitsDue
  },
  refresh: () => void refresh(),
  /** Один раз из App: подписки живут всё время работы страницы. */
  start() {
    if (started) return
    started = true
    void refresh()
    window.addEventListener('hashchange', () => void refresh())
    window.addEventListener('focus', () => void refresh())
    setInterval(() => void refresh(), INTERVAL_MS)
  },
}
