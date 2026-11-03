<script lang="ts">
  import type { FinishResult } from '../../lib/api/types'
  import Button from '../../lib/components/Button.svelte'
  import Card from '../../lib/components/Card.svelte'
  import Icon from '../../lib/components/Icon.svelte'
  import LevelBadge from '../../lib/components/LevelBadge.svelte'
  import ProgressBar from '../../lib/components/ProgressBar.svelte'
  import { formatDuration } from '../../lib/dates'
  import { t } from '../../lib/i18n/i18n.svelte'
  import { route } from '../../lib/state/route.svelte'
  import { session } from '../../lib/state/session.svelte'

  let { result, groupId }: { result: FinishResult; groupId: number | null } = $props()

  const rows = $derived([
    { key: 'good', label: t('training.good'), count: result.good, color: 'var(--c-green)' },
    { key: 'hard', label: t('training.hard'), count: result.hard, color: 'var(--c-amber)' },
    { key: 'again', label: t('training.again'), count: result.again, color: 'var(--c-red)' },
  ])
</script>

<div class="results scroll">
  <header>
    <Icon name="trophy" size={36} />
    <h2>{t('training.results.title')}</h2>
    {#if result.free}<p class="muted">{t('training.results.freeNote')}</p>{/if}
  </header>

  <div class="tiles">
    <Card padding="sm"><span class="label muted">{t('training.results.words')}</span><span class="value">{result.total}</span></Card>
    <Card padding="sm"><span class="label muted">{t('training.results.time')}</span><span class="value">{formatDuration(result.durationSec)}</span></Card>
  </div>

  <Card>
    <ul class="answers">
      {#each rows as r (r.key)}
        <li>
          <span class="name">{r.label}</span>
          <ProgressBar value={result.total ? r.count / result.total : 0} color={r.color} label={r.label} />
          <span class="count">{r.count}</span>
        </li>
      {/each}
    </ul>
  </Card>

  <Card>
    <h3>{t('training.results.levelChanges')}</h3>
    {#if result.levelChanges.length === 0}
      <p class="muted">{t('training.results.noChanges')}</p>
    {:else}
      <ul class="changes">
        {#each result.levelChanges as c (c.wordId)}
          <li><span class="term">{c.term}</span><LevelBadge level={c.levelBefore} /> → <LevelBadge level={c.levelAfter} /></li>
        {/each}
      </ul>
    {/if}
  </Card>

  <div class="actions">
    {#if groupId !== null}
      <Button icon="folder" onclick={() => route.go('groups', String(groupId))}>{t('training.results.toGroups')}</Button>
    {/if}
    <Button variant="primary" icon="refresh" onclick={() => session.reset()}>{t('training.results.again')}</Button>
  </div>
</div>

<style>
  .results {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    max-width: 560px;
    width: 100%;
    margin: 0 auto;
    padding: 2px 6px 6px 2px;
  }
  header {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-2);
    text-align: center;
  }
  h2 {
    font-size: var(--fs-xl);
  }
  h3 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-3);
  }
  .tiles {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3);
  }
  .tiles :global(.card) {
    display: flex;
    flex-direction: column;
  }
  .label {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .value {
    font-size: var(--fs-xl);
    font-weight: 800;
    font-variant-numeric: tabular-nums;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .answers li {
    display: grid;
    grid-template-columns: 120px 1fr 32px;
    align-items: center;
    gap: var(--s-3);
  }
  .name {
    font-weight: 600;
  }
  .count {
    text-align: right;
    font-weight: 700;
  }
  .changes li {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .term {
    flex: 1;
    font-weight: 700;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--s-2);
  }
</style>
