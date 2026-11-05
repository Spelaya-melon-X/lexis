<script lang="ts">
  // Навигация по дням: ← дата →, «Сегодня», выбор даты. Будущие дни недоступны (§17.2).
  import Button from './Button.svelte'
  import Icon from './Icon.svelte'
  import { addDays, isDay, longDay } from '../dates'
  import { i18n, t } from '../i18n/i18n.svelte'

  interface Props {
    day: string
    max: string // сегодня
    onchange: (day: string) => void
  }
  let { day, max, onchange }: Props = $props()

  const isToday = $derived(day === max)
</script>

<div class="pager">
  <button class="arrow" aria-label={t('date.prevDay')} title={t('date.prevDay')} onclick={() => onchange(addDays(day, -1))}>
    <Icon name="chevronLeft" />
  </button>

  <label class="title">
    <h1>{longDay(day, i18n.locale)}</h1>
    <!-- нативный выбор даты поверх заголовка: клик по дате открывает календарь браузера -->
    <input
      type="date"
      value={day}
      {max}
      aria-label={t('date.pickDate')}
      onchange={(e) => {
        const v = e.currentTarget.value
        if (isDay(v) && v <= max) onchange(v)
      }}
    />
  </label>

  <button class="arrow" aria-label={t('date.nextDay')} title={t('date.nextDay')} disabled={isToday} onclick={() => onchange(addDays(day, 1))}>
    <Icon name="chevronRight" />
  </button>

  {#if !isToday}
    <Button size="sm" variant="ghost" icon="sun" onclick={() => onchange(max)}>{t('date.today')}</Button>
  {/if}
</div>

<style>
  .pager {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  .arrow {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    cursor: pointer;
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  .arrow:hover:not(:disabled) {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  .arrow:disabled {
    opacity: 0.35;
    cursor: default;
  }
  .title {
    position: relative;
    min-width: 0;
    cursor: pointer;
  }
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  h1::first-letter {
    text-transform: uppercase;
  }
  /* Невидимое поле на весь заголовок: открывает пикер браузера, сохраняя наш вид даты */
  input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
  }
  input::-webkit-calendar-picker-indicator {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    cursor: pointer;
  }
</style>
