<script lang="ts">
  import type { Snippet } from 'svelte'

  // §12: карточка — 3D flip. Обе стороны в одной ячейке грида: высота карточки = высоте
  // большей стороны, и ничего не прыгает при перевороте.
  let { flipped, front, back }: { flipped: boolean; front: Snippet; back: Snippet } = $props()
</script>

<div class="scene">
  <div class="inner" class:flipped>
    <div class="face front" inert={flipped} aria-hidden={flipped}>{@render front()}</div>
    <div class="face back" inert={!flipped} aria-hidden={!flipped}>{@render back()}</div>
  </div>
</div>

<style>
  .scene {
    perspective: 1200px;
  }
  .inner {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    transform-style: preserve-3d;
    transition: transform var(--dur-slow) var(--ease-out);
  }
  .inner.flipped {
    transform: rotateY(180deg);
  }
  .face {
    grid-area: 1 / 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-4);
    min-width: 0;
    min-height: clamp(16rem, 36dvh, 21rem);
    padding: clamp(1.5rem, 5vw, 3rem) clamp(1rem, 4vw, 2.5rem);
    border: var(--bw-strong) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow);
    text-align: center;
    overflow-wrap: anywhere;
    backface-visibility: hidden;
  }
  .back {
    transform: rotateY(180deg);
    background: color-mix(in srgb, var(--c-green-soft) 45%, var(--c-surface));
  }
  @media (prefers-reduced-motion: reduce) {
    .inner.flipped, .back {
      transform: none;
    }
    .face[aria-hidden='true'] {
      visibility: hidden;
    }
  }
</style>
