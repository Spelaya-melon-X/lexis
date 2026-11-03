<script lang="ts">
  import type { Snippet } from 'svelte'

  // §12: карточка — 3D flip. Обе стороны в одной ячейке грида: высота карточки = высоте
  // большей стороны, и ничего не прыгает при перевороте.
  let { flipped, front, back }: { flipped: boolean; front: Snippet; back: Snippet } = $props()
</script>

<div class="scene">
  <div class="inner" class:flipped>
    <!-- inert, а не aria-hidden: невидимая сторона не должна получать фокус по Tab -->
    <div class="face front" inert={flipped}>{@render front()}</div>
    <div class="face back" inert={!flipped}>{@render back()}</div>
  </div>
</div>

<style>
  .scene {
    perspective: 1200px;
  }
  .inner {
    display: grid;
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
    min-height: 300px;
    padding: var(--s-6) var(--s-5);
    border: var(--bw-strong) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow);
    text-align: center;
    backface-visibility: hidden;
  }
  .back {
    transform: rotateY(180deg);
    background: var(--c-green-soft);
  }
</style>
