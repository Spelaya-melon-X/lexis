<script lang="ts" generics="T extends string">
  // Сегментный переключатель (неделя/месяц и т.п.): один выбранный вариант из нескольких.
  interface Props {
    options: { value: T; label: string }[]
    value: T
    label: string // для скринридера — что переключаем
    onchange: (v: T) => void
  }
  let { options, value, label, onchange }: Props = $props()
</script>

<div class="seg" role="group" aria-label={label}>
  {#each options as o (o.value)}
    <button class:active={o.value === value} aria-pressed={o.value === value} onclick={() => onchange(o.value)}>{o.label}</button>
  {/each}
</div>

<style>
  .seg {
    display: inline-flex;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  button {
    padding: 2px var(--s-3);
    border: 0;
    background: var(--c-surface);
    font-size: var(--fs-sm);
    font-weight: 600;
    cursor: pointer;
  }
  button + button {
    border-left: var(--bw) solid var(--c-border);
  }
  button.active {
    background: var(--c-text);
    color: var(--c-bg);
  }
</style>
