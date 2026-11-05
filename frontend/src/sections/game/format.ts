// Подписи для карточек дел (через i18n — поэтому не в lib/tasks.ts с чистой логикой).
import { t } from '../../lib/i18n/i18n.svelte'
import { splitBlocks } from '../../lib/tasks'

/** 3 блока → «1 ч 30 мин», 2 → «1 ч», 1 → «30 мин». */
export function blocksText(blocks: number): string {
  const { h, m } = splitBlocks(blocks)
  if (h === 0) return t('game.minutes', { m })
  return m === 0 ? t('game.hours', { h }) : t('game.hoursMinutes', { h, m })
}

/** Секунды → «1 ч 20 мин» / «45 мин» / «0 мин» (время в фокусе, с точностью до минуты). */
export function blocksTextSec(sec: number): string {
  const total = Math.floor(Math.max(0, sec) / 60)
  const h = Math.floor(total / 60)
  const m = total % 60
  if (h === 0) return t('game.minutes', { m })
  return m === 0 ? t('game.hours', { h }) : t('game.hoursMinutes', { h, m })
}
