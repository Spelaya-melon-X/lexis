<script lang="ts">
  // Выполнение привычек за период (§17.5): positive — «3 из 4 дн.» + сумма количества,
  // negative — срывы и лучшая серия без срыва.
  import type { HabitStat } from '../../lib/api/types'
  import ProgressBar from '../../lib/components/ProgressBar.svelte'
  import { formatAmount } from '../../lib/habits'
  import { i18n, t } from '../../lib/i18n/i18n.svelte'

  let { stats }: { stats: HabitStat[] } = $props()

  const fmt = (n: number) => formatAmount(n, i18n.locale)
</script>

<ul>
  {#each stats as s (s.habitId)}
    {@const negative = s.kind === 'negative'}
    <li class:archived={s.archived}>
      <div class="top">
        <span class="dot" style:background={s.color ?? 'transparent'}></span>
        <b>{s.name}</b>
        {#if s.archived}<small class="muted">· {t('journal.stats.archived')}</small>{/if}
        <span class="figure" class:bad={negative && s.checked > 0} class:good={negative && s.checked === 0}>
          {#if negative}
            {t('journal.stats.slips', { count: s.checked })}
          {:else}
            {t('journal.stats.doneOf', { checked: s.checked, days: s.days })}
          {/if}
        </span>
      </div>
      {#if negative}
        <!-- доля «чистых» дней: 1 — ни одного срыва -->
        <ProgressBar value={s.days ? (s.days - Math.min(s.checked, s.days)) / s.days : 0} color={s.color ?? undefined} label={s.name} />
      {:else}
        <ProgressBar value={s.days ? s.checked / s.days : 0} color={s.color ?? undefined} label={s.name} />
      {/if}
      <div class="sub muted">
        {#if negative}<span>{t('journal.stats.bestClean', { count: s.bestClean })}</span>{/if}
        {#if s.amount !== null}<span>{t('journal.stats.total', { value: `${fmt(s.amount)} ${s.unit}`.trim() })}</span>{/if}
        {#if s.avgRating !== null}<span>{t('journal.stats.avg', { value: fmt(s.avgRating) })}</span>{/if}
      </div>
    </li>
  {/each}
</ul>

<style>
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  li {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  li.archived {
    opacity: 0.6;
  }
  .top {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .dot {
    width: 10px;
    height: 10px;
    flex: none;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .figure {
    margin-left: auto;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .figure.bad {
    color: var(--c-red);
  }
  .figure.good {
    color: var(--c-green);
  }
  .sub {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-3);
    font-size: var(--fs-xs);
  }
  .sub:empty {
    display: none;
  }
</style>
