<script lang="ts">
  interface Props {
    value: number // 0..1
    color?: string // CSS-цвет; по умолчанию зелёный
    label?: string
  }
  let { value, color = 'var(--c-green)', label }: Props = $props()
  const pct = $derived(Math.round(Math.min(Math.max(value, 0), 1) * 100))
</script>

<!-- §12: смена уровня/прогресса — анимация ширины -->
<div class="track" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
  <div class="fill" style:width="{pct}%" style:background={color}></div>
</div>

<style>
  .track {
    height: 12px;
    border: var(--bw) solid var(--c-border);
    border-radius: 999px;
    background: var(--c-surface);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    transition: width var(--dur-slow) var(--ease-out);
  }
</style>
