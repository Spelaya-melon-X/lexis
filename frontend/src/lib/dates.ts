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

