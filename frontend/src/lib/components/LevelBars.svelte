<script lang="ts">
  import type { Level, Levels } from '../api/types'
  import { t } from '../i18n/i18n.svelte'

  // Распределение слов по 5 уровням. compact — одна полоска-стек для списков,
  // иначе — строки с подписями и анимированной шириной (§12: смена уровня — анимация).
  let { levels, compact = false }: { levels: Levels; compact?: boolean } = $props()

  const total = $derived(levels.reduce((a, b) => a + b, 0))
  const max = $derived(Math.max(...levels, 1))
  const rows = $derived(levels.map((count, i) => ({ level: (i + 1) as Level, count })))
</script>

{#if compact}
  <div class="stack" aria-hidden="true">
    {#if total > 0}
      {#each rows as r (r.level)}
        {#if r.count > 0}<span style:flex-grow={r.count} style:background="var(--c-level-{r.level})"></span>{/if}
      {/each}
    {/if}
  </div>
{:else}
  <ul class="rows">
    {#each rows as r (r.level)}
      <li>
        <span class="name">{t(`levels.${r.level}`)}</span>
        <span class="track"><span class="fill" style:width="{(r.count / max) * 100}%" style:background="var(--c-level-{r.level})"></span></span>
        <span class="count">{r.count}</span>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .stack {
    display: flex;
    height: 8px;
    border: 1px solid var(--c-border);
    border-radius: 999px;
    overflow: hidden;
    background: var(--c-surface-2);
  }
  .rows {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  li {
    display: grid;
    grid-template-columns: 80px 1fr 40px;
    align-items: center;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .track {
    height: 14px;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    overflow: hidden;
  }
  .fill {
    display: block;
    height: 100%;
    transition: width var(--dur-slow) var(--ease-out);
  }
  .count {
    text-align: right;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
</style>
