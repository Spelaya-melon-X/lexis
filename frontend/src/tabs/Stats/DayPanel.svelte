<script lang="ts">
  import type { Overview } from '../../lib/api/types'
  import ProgressBar from '../../lib/components/ProgressBar.svelte'
  import { formatDuration, longDay } from '../../lib/dates'
  import { i18n, t } from '../../lib/i18n/i18n.svelte'

  let { overview }: { overview: Overview } = $props()

  const d = $derived(overview.day)
  const maxGroup = $derived(Math.max(...overview.byGroup.map((g) => g.words), 1))
  const answers = $derived([
    { key: 'good', label: t('training.good'), count: d.good, color: 'var(--c-green)' },
    { key: 'hard', label: t('training.hard'), count: d.hard, color: 'var(--c-amber)' },
    { key: 'again', label: t('training.again'), count: d.again, color: 'var(--c-red)' },
  ])
</script>

<div class="day">
  <h3 class="date">{longDay(d.day, i18n.locale)}</h3>
  {#if d.sessions === 0}
    <p class="muted">{t('stats.dayEmpty')}</p>
  {:else}
    <div class="nums">
      <span><strong>{t('stats.dayWords', { count: d.words })}</strong></span>
      <span class="muted">{formatDuration(d.seconds)}</span>
      <span class="muted">{t('stats.groupsToday')}: {overview.groupsTrained}</span>
    </div>

    <h4>{t('stats.answers')}</h4>
    <ul class="rows">
      {#each answers as a (a.key)}
        <li><span>{a.label}</span><ProgressBar value={d.words ? a.count / d.words : 0} color={a.color} label={a.label} /><b>{a.count}</b></li>
      {/each}
    </ul>

    {#if overview.byGroup.length > 0}
      <h4>{t('stats.byGroup')}</h4>
      <ul class="rows">
        {#each overview.byGroup as g (g.id)}
          <li><span class="name">#{g.name}</span><ProgressBar value={g.words / maxGroup} color="var(--c-level-3)" label={g.name} /><b>{g.words}</b></li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .day {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  .date {
    font-size: var(--fs-md);
    text-transform: capitalize;
  }
  .nums {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-3);
    font-size: var(--fs-sm);
  }
  h4 {
    font-size: var(--fs-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--c-muted);
    margin: 0;
  }
  .rows {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .rows li {
    display: grid;
    grid-template-columns: 110px 1fr 32px;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .rows b {
    text-align: right;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
