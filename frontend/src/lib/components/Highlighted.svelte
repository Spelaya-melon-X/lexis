<script lang="ts">
  import type { Span } from '../api/types'
  import { segments } from '../text'

  // Предложение с выделенными словами (§4: всё, что было в [ ]).
  let { sentence, highlights = [] }: { sentence: string; highlights?: Span[] } = $props()
  const parts = $derived(segments(sentence, highlights))
</script>

<span>
  {#each parts as p, i (i)}{#if p.highlight}<mark>{p.text}</mark>{:else}{p.text}{/if}{/each}
</span>

<style>
  mark {
    background: var(--c-green-soft);
    color: inherit;
    font-weight: 700;
    padding: 0 2px;
    border-bottom: var(--bw) solid var(--c-green);
  }
</style>
