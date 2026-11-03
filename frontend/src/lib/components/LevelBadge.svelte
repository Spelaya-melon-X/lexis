<script lang="ts">
  import type { Level } from '../api/types'
  import { t } from '../i18n/i18n.svelte'

  let { level }: { level: Level } = $props()
</script>

<span class="badge" style:--lc="var(--c-level-{level})">
  <span class="dot"></span>{t(`levels.${level}`)}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 1px var(--s-2);
    border: var(--bw) solid var(--c-border);
    border-radius: 999px;
    font-size: var(--fs-xs);
    font-weight: 600;
    white-space: nowrap;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--lc);
  }
</style>
