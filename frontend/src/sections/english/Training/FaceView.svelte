<script lang="ts">
  import Highlighted from '../../../lib/components/Highlighted.svelte'
  import type { Face } from '../../../lib/training'

  let { face }: { face: Face } = $props()
</script>

{#if face.kind === 'sentence'}
  <p class="sentence"><Highlighted sentence={face.sentence} highlights={face.highlights} /></p>
{:else if face.kind === 'answer'}
  <p class="big">{face.term}</p>
  <p class="sub">{face.translation}</p>
{:else}
  <p class="big">{face.text}</p>
{/if}

<style>
  .big {
    font-size: var(--fs-2xl);
    font-weight: 800;
    letter-spacing: -0.02em;
    line-height: 1.15;
    word-break: break-word;
  }
  .sub {
    font-size: var(--fs-lg);
    color: var(--c-muted);
  }
  .sentence {
    font-size: var(--fs-xl);
    line-height: 1.4;
    max-width: 28ch;
  }
</style>
