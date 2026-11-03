<script lang="ts">
  import type { Achievement } from '../../lib/api/types'
  import Icon, { type IconName } from '../../lib/components/Icon.svelte'
  import ProgressBar from '../../lib/components/ProgressBar.svelte'
  import { hasKey, i18n, t } from '../../lib/i18n/i18n.svelte'

  let { items }: { items: Achievement[] } = $props()

  const icon = (code: string): IconName =>
    code.startsWith('streak_') ? 'flame' : code === 'first_archived_group' ? 'archive' : 'trophy'

  // Код пришёл с бэка — если текста ещё нет в словаре (новая ачивка), показываем код, а не ломаемся.
  const text = (code: string, part: 'title' | 'desc') => {
    const key = `achievements.${code}.${part}`
    return hasKey(key) ? t(key) : part === 'title' ? code : ''
  }
  const date = (iso: string) => new Date(iso).toLocaleDateString(i18n.locale, { day: 'numeric', month: 'long', year: 'numeric' })
</script>

<ul class="grid">
  {#each items as a (a.code)}
    <li class="ach" class:unlocked={a.unlockedAt !== null}>
      <span class="badge"><Icon name={icon(a.code)} size={22} /></span>
      <div class="body">
        <strong>{text(a.code, 'title')}</strong>
        <span class="desc muted">{text(a.code, 'desc')}</span>
        {#if a.unlockedAt}
          <span class="when">{t('stats.unlocked', { date: date(a.unlockedAt) })}</span>
        {:else}
          <ProgressBar value={a.target ? a.progress / a.target : 0} label={text(a.code, 'title')} />
          <span class="when muted">{t('stats.progress', { progress: a.progress, target: a.target })}</span>
        {/if}
      </div>
    </li>
  {/each}
</ul>

<style>
  .grid {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: var(--s-3);
  }
  .ach {
    display: flex;
    gap: var(--s-3);
    padding: var(--s-3);
    border: var(--bw) dashed var(--c-line);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
  }
  .ach.unlocked {
    border-style: solid;
    border-color: var(--c-border);
    box-shadow: var(--shadow-sm);
  }
  .badge {
    flex: none;
    display: grid;
    place-items: center;
    width: 40px;
    height: 40px;
    border: var(--bw) solid var(--c-line);
    border-radius: 50%;
    color: var(--c-muted);
  }
  .unlocked .badge {
    border-color: var(--c-border);
    background: var(--c-amber-soft);
    color: var(--c-text);
  }
  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .desc,
  .when {
    font-size: var(--fs-xs);
  }
  .unlocked .when {
    color: var(--c-green);
    font-weight: 600;
  }
</style>
