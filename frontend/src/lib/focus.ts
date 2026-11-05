// Таймеры фокус-сессии (§18.3, B4). Зеркало Go focus.Compute: фронт пересчитывает разбивку
// каждую секунду от серверных отметок (с поправкой на разницу часов), поэтому перезагрузка ничего не сбивает.

import type { FocusSession, FocusTimeline } from './api/types'

export const WORK_CHUNK_SEC = 30 * 60
export const BREAK_PER_CHUNK_SEC = 5 * 60

type Times = Pick<FocusSession, 'startedAt' | 'workStartedAt' | 'finishedAt' | 'breaks'>

const ms = (iso: string) => Date.parse(iso)
const sec = (from: number, to: number) => Math.max(0, Math.floor((to - from) / 1000))

/** Разбивка на момент nowMs (для завершённой — на момент завершения). */
export function timeline(s: Times, nowMs: number): FocusTimeline {
  const end = s.finishedAt ? ms(s.finishedAt) : nowMs
  const last = s.breaks.at(-1)
  const phase = s.finishedAt ? 'finished' : last && last.endedAt === null ? 'break' : s.workStartedAt ? 'work' : 'setup'

  const setupSec = sec(ms(s.startedAt), s.workStartedAt ? ms(s.workStartedAt) : end)
  let breakSec = 0
  for (const b of s.breaks) {
    const bEnd = b.endedAt && ms(b.endedAt) < end ? ms(b.endedAt) : end
    breakSec += sec(ms(b.startedAt), bEnd)
  }
  const workSec = s.workStartedAt ? Math.max(0, sec(ms(s.workStartedAt), end) - breakSec) : 0
  const breakEarnedSec = Math.floor(workSec / WORK_CHUNK_SEC) * BREAK_PER_CHUNK_SEC
  return {
    phase,
    setupSec,
    workSec,
    breakSec,
    breakEarnedSec,
    breakAvailableSec: Math.max(0, breakEarnedSec - breakSec),
    breakOverrunSec: Math.max(0, breakSec - breakEarnedSec),
  }
}

/** 75 → "1:15", 3725 → "1:02:05". */
export function formatClock(totalSec: number): string {
  const s = Math.max(0, Math.floor(totalSec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

/** Секунды текущего перерыва (открытого) — для таймера «вернуться через». */
export function currentBreakSec(s: Times, nowMs: number): number {
  const last = s.breaks.at(-1)
  return last && last.endedAt === null ? sec(ms(last.startedAt), nowMs) : 0
}

/** Отмечены ли все пункты ритуала (пустой ритуал — можно начинать сразу). */
export function ritualDone(items: number, checked: readonly number[]): boolean {
  return Array.from({ length: items }, (_, i) => i).every((i) => checked.includes(i))
}
