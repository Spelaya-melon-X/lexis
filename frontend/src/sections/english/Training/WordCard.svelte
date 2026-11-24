<script lang="ts">
  import type { TrainingCard, TrainingDirection } from '../../../lib/api/types'
  import Highlighted from '../../../lib/components/Highlighted.svelte'
  import { displayTerm, exampleHighlights } from '../../../lib/text'
  import FlipCard from './FlipCard.svelte'

  let { card, revealed, strategy }: { card: TrainingCard; revealed: boolean; strategy: TrainingDirection } = $props()
  const term = $derived(displayTerm(card.term, card.forms))
  let heading: HTMLHeadingElement | undefined = $state()
  $effect(() => {
    if (revealed) heading?.focus({ preventScroll: true })
  })
</script>

<FlipCard flipped={revealed}>
  {#snippet front()}
    <h2 class="term" lang={strategy === 'en_ru' ? 'en' : 'ru'}>{strategy === 'en_ru' ? term : card.translation}</h2>
  {/snippet}
  {#snippet back()}
    <h2 class="term answer-term" lang="en" tabindex="-1" bind:this={heading}>{term}</h2>
    {#if card.examples?.length}
      <div class="examples" lang="en">
        {#each card.examples as example (example.text)}
          <p><Highlighted sentence={example.sentence} highlights={exampleHighlights(example, card.term, card.forms)} /></p>
        {/each}
      </div>
    {/if}
    <p class="translation">{card.translation}</p>
  {/snippet}
</FlipCard>

<style>
  .term {
    font-size: clamp(2rem, 5.5vw, 3.5rem);
    font-weight: 800;
    letter-spacing: -0.04em;
    line-height: 1.12;
    text-wrap: balance;
  }
  .answer-term:focus {
    outline: none;
  }
  .examples {
    display: grid;
    gap: var(--s-2);
    font-size: clamp(1rem, 2.3vw, 1.25rem);
    font-weight: 600;
    line-height: 1.5;
    max-width: 50ch;
    text-wrap: pretty;
  }
  .translation {
    font-size: clamp(1rem, 2vw, 1.15rem);
    font-style: italic;
    color: var(--c-muted);
    border-top: 1px solid var(--c-line);
    padding-top: var(--s-3);
  }
</style>
