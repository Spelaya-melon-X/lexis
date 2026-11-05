<script lang="ts">
  import type { Card } from '../../../lib/api/types'
  import Highlighted from '../../../lib/components/Highlighted.svelte'
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { displayTerm } from '../../../lib/text'

  let { card }: { card: Card } = $props()
</script>

<article class="card">
  <header>
    <h3>{displayTerm(card.term, card.forms)}</h3>
    <span class="line muted">{t('add.line', { line: card.line })}</span>
  </header>
  <p class="translation">{card.translation}</p>

  {#if card.examples?.length}
    <ul class="examples">
      {#each card.examples as ex (ex.text)}
        <li><Highlighted sentence={ex.sentence} highlights={ex.highlights} /></li>
      {/each}
    </ul>
  {/if}

  {#if card.groups?.length}
    <div class="tags">
      {#each card.groups as g (g)}<span class="tag">#{g}</span>{/each}
    </div>
  {/if}
</article>

<style>
  .card {
    padding: var(--s-3) var(--s-4);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s-3);
  }
  h3 {
    font-size: var(--fs-lg);
    word-break: break-word;
  }
  .line {
    font-size: var(--fs-xs);
    white-space: nowrap;
  }
  .translation {
    color: var(--c-muted);
    margin-top: 2px;
  }
  .examples {
    margin: var(--s-2) 0 0;
    padding-left: var(--s-4);
    font-size: var(--fs-sm);
  }
  .examples li + li {
    margin-top: 2px;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
    margin-top: var(--s-2);
  }
  .tag {
    padding: 0 var(--s-2);
    border: 1px solid var(--c-border);
    border-radius: 999px;
    font-size: var(--fs-xs);
    font-weight: 600;
  }
</style>
