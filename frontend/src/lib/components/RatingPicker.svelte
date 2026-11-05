<script lang="ts">
  // Оценка 1–10 (день, отметка привычки); клик по выбранной — снять оценку.
  import { t } from '../i18n/i18n.svelte'
  import { ratingTone } from '../journal'

  interface Props {
    value: number | null
    label: string // для скринридера — что оцениваем
    size?: 'sm' | 'md'
    onchange: (v: number | null) => void
  }
  let { value, label, size = 'md', onchange }: Props = $props()

  const tone = $derived(ratingTone(value))
</script>

<div class="wrap">
  <div class="rating tone-{tone} {size}" role="radiogroup" aria-label={label}>
    {#each { length: 10 } as _, i (i)}
      {@const n = i + 1}
      {@const on = value === n}
      <button
        role="radio"
        aria-checked={on}
        class:on
        class:filled={value !== null && n <= value}
        title={on ? t('date.ratingClear') : undefined}
        onclick={() => onchange(on ? null : n)}
      >
        {n}
      </button>
    {/each}
  </div>
</div>

<style>
  /* Обёртка — контейнер для @container: шкала подстраивается под ширину карточки, а не окна */
  .wrap {
    container-type: inline-size;
  }
  .rating {
    display: grid;
    grid-template-columns: repeat(10, minmax(0, 1fr));
    gap: var(--s-1);
    --tone: var(--c-surface-2);
    --tone-strong: var(--c-text);
  }
  .tone-red {
    --tone: var(--c-red-soft);
    --tone-strong: var(--c-red);
  }
  .tone-amber {
    --tone: var(--c-amber-soft);
    --tone-strong: var(--c-amber);
  }
  .tone-green {
    --tone: var(--c-green-soft);
    --tone-strong: var(--c-green);
  }
  button {
    aspect-ratio: 1;
    max-height: 44px;
    min-width: 0;
    padding: 0; /* без браузерного padding «10» влезает даже в узкую ячейку */
    display: grid;
    place-items: center;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    font-weight: 700;
    font-variant-numeric: tabular-nums;
    line-height: 1;
    cursor: pointer;
    transition:
      background var(--dur) var(--ease-out),
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  button:hover {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  /* «шкала»: всё до выбранного подкрашено, сам выбор — насыщенным цветом */
  /* Компактная шкала (отметки привычек): фиксированные ячейки рядом, без растягивания на всю строку */
  .rating.sm {
    grid-template-columns: repeat(10, 30px);
    justify-content: start;
  }
  .sm button {
    max-height: 30px;
    font-size: var(--fs-sm);
  }
  /* Узкая карточка: два ряда по 5 — ячейки остаются крупными и читаемыми */
  @container (max-width: 340px) {
    .rating {
      grid-template-columns: repeat(5, minmax(0, 1fr));
    }
    .rating.sm {
      grid-template-columns: repeat(5, 30px);
    }
    button {
      max-height: none;
    }
  }
  .filled {
    background: var(--tone);
  }
  .on {
    background: var(--tone-strong);
    color: var(--c-bg);
    box-shadow: var(--shadow-sm);
  }
</style>
