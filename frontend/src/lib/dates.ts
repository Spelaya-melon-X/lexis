// Даты с бэкенда: "YYYY-MM-DD" — календарный день в TZ пользователя, без времени.

/**
 * "2026-11-02" → Date в локальной полуночи. Не new Date("2026-11-02"): такая строка
 * парсится как UTC-полночь, и западнее Гринвича превратилась бы в 1 ноября.
 */
export function parseDay(day: string): Date {
  const [y, m, d] = day.split('-').map(Number)
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1)
}

/** Короткая подпись дня для осей: «2 нояб.» / «Nov 2». */
export function shortDay(day: string, locale: string): string {
  return parseDay(day).toLocaleDateString(locale, { day: 'numeric', month: 'short' })
}

/** 75 → "1:15", 3725 → "1:02:05". Отрицательное/NaN → "0:00". */
export function formatDuration(totalSec: number): string {
  const s = Number.isFinite(totalSec) && totalSec > 0 ? Math.floor(totalSec) : 0
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`
}

/** Длинная подпись дня: «понедельник, 2 ноября» / «Monday, November 2». */
export function longDay(day: string, locale: string): string {
  return parseDay(day).toLocaleDateString(locale, { weekday: 'long', day: 'numeric', month: 'long' })
}


// ---------- календарные дни для раздела «День» (§17.2) ----------
// Арифметика — через локальный Date (parseDay): new Date(y, m, d + n) сам переносит
// месяц/год и не ломается на переходе на летнее время, в отличие от «+ 86400000 мс».

/** Date → "YYYY-MM-DD" по локальной дате (не toISOString — тот в UTC). */
export function formatDay(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/** Сегодня по часам браузера. Бэкенд считает «сегодня» в LEXIS_TZ — на одной машине это одно и то же. */
export function today(now = new Date()): string {
  return formatDay(now)
}

export function addDays(day: string, n: number): string {
  const d = parseDay(day)
  return formatDay(new Date(d.getFullYear(), d.getMonth(), d.getDate() + n))
}

/** Строка — существующая дата "YYYY-MM-DD" (2026-02-30 — нет). */
export function isDay(s: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(s) && formatDay(parseDay(s)) === s
}

/** "YYYY-MM" месяца, в который входит день. */
export function monthOf(day: string): string {
  return day.slice(0, 7)
}

export function isMonth(s: string): boolean {
  return /^\d{4}-(0[1-9]|1[0-2])$/.test(s)
}

export function addMonths(month: string, n: number): string {
  const d = parseDay(`${month}-01`)
  return monthOf(formatDay(new Date(d.getFullYear(), d.getMonth() + n, 1)))
}

/**
 * Сетка месяца для календаря: недели-строки с понедельника, дни чужих месяцев — null.
 * "2026-11" (1 ноября — воскресенье) → первая строка [null×6, "2026-11-01"].
 */
export function monthGrid(month: string): (string | null)[][] {
  const first = parseDay(`${month}-01`)
  const lead = (first.getDay() + 6) % 7 // Пн = 0 … Вс = 6
  const days = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()
  const cells: (string | null)[] = Array.from({ length: lead }, () => null)
  for (let i = 1; i <= days; i++) cells.push(`${month}-${String(i).padStart(2, '0')}`)
  while (cells.length % 7) cells.push(null)
  const weeks: (string | null)[][] = []
  for (let i = 0; i < cells.length; i += 7) weeks.push(cells.slice(i, i + 7))
  return weeks
}

/** Название месяца: «ноябрь 2026» / «November 2026». */
export function monthTitle(month: string, locale: string): string {
  return parseDay(`${month}-01`).toLocaleDateString(locale, { month: 'long', year: 'numeric' })
}
