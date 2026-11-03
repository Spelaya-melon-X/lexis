<script lang="ts">
  import { t } from '../../lib/i18n/i18n.svelte'

  // Палитра вместо <input type=color>: цвета подобраны под стиль, и группы не уезжают в кислотные оттенки.
  const PALETTE = ['#16a34a', '#0d9488', '#2563eb', '#7c3aed', '#db2777', '#dc2626', '#ea580c', '#ca8a04']

  let { value, onchange }: { value: string | null; onchange: (c: string | null) => void } = $props()
</script>

<div class="picker" role="group" aria-label={t('groups.color')}>
  <button class="swatch none" class:active={value === null} aria-pressed={value === null} title={t('groups.noColor')} onclick={() => onchange(null)}></button>
  {#each PALETTE as c (c)}
    <button class="swatch" class:active={value === c} aria-pressed={value === c} style:background={c} title={c} onclick={() => onchange(c)}></button>
  {/each}
</div>

<style>
  .picker {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
  }
  .swatch {
    width: 22px;
    height: 22px;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
    cursor: pointer;
    transition: transform var(--dur-fast) var(--ease-spring);
  }
  .swatch:hover {
    transform: scale(1.12);
  }
  .swatch.active {
    outline: var(--bw) solid var(--c-border);
    outline-offset: 2px;
  }
  /* «без цвета» — перечёркнутый белый кружок */
  .none {
    background: linear-gradient(135deg, transparent 45%, var(--c-red) 45% 55%, transparent 55%), var(--c-surface);
  }
</style>
