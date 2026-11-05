<script lang="ts">
  // Плашка активной фокус-сессии в шапке (§18.3, B4): видна из любого раздела, клик — на страницу «Фокус».
  import { formatClock } from '../focus'
  import { t } from '../i18n/i18n.svelte'
  import { focus } from '../state/focus.svelte'
  import Icon from './Icon.svelte'

  const tl = $derived(focus.timeline)
  // Что показывать: настройка и работа — сколько прошло, перерыв — сколько осталось (или перерасход).
  const clock = $derived.by(() => {
    if (!tl) return ''
    if (tl.phase === 'setup') return formatClock(tl.setupSec)
    if (tl.phase === 'break') return tl.breakAvailableSec > 0 ? formatClock(tl.breakAvailableSec) : `+${formatClock(tl.breakOverrunSec)}`
    return formatClock(tl.workSec)
  })
</script>

{#if focus.session && tl}
  <a class="pill {tl.phase}" href="#/game/focus" title={focus.session.taskTitle} aria-label="{t(`game.focus.phase.${tl.phase as 'work'}`)}: {focus.session.taskTitle}, {clock}">
    <Icon name={tl.phase === 'break' ? 'pause' : 'target'} size={16} />
    <span class="clock">{clock}</span>
    <span class="title">{focus.session.taskTitle}</span>
  </a>
{/if}

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    max-width: 240px;
    padding: var(--s-1) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: 999px;
    background: var(--c-green-soft);
    color: var(--c-text);
    font-size: var(--fs-sm);
    font-weight: 700;
    text-decoration: none;
    box-shadow: var(--shadow-sm);
    animation: pop var(--dur) var(--ease-spring);
  }
  .pill.setup {
    background: var(--c-amber-soft);
  }
  .pill.break {
    background: var(--c-surface-2);
  }
  .clock {
    font-variant-numeric: tabular-nums;
  }
  .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
  }
  @keyframes pop {
    from {
      transform: scale(0.9);
      opacity: 0;
    }
  }
  @media (max-width: 860px) {
    .title {
      display: none;
    }
  }
</style>
