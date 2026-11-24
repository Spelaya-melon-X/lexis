<script lang="ts">
  import type { Dream } from '../../lib/api/types'
  import Icon from '../../lib/components/Icon.svelte'
  import { shortDay } from '../../lib/dates'
  import { i18n, t } from '../../lib/i18n/i18n.svelte'
  let { dream, category, project, busy, onedit, ontoggle, onconvert, ondelete }: {
    dream: Dream; category?: string; project?: string; busy: boolean
    onedit: () => void; ontoggle: () => void; onconvert: () => void; ondelete: () => void
  } = $props()
  let menu: HTMLDetailsElement | undefined = $state()
  function closeMenu(e: PointerEvent) {
    if (menu?.open && e.target instanceof Node && !menu.contains(e.target)) menu.open = false
  }
  function act(action: () => void) { if (menu) menu.open = false; action() }
</script>

<svelte:window onpointerdown={closeMenu} onkeydown={e => { if (e.key === 'Escape' && menu?.open) { menu.open = false; menu.querySelector('summary')?.focus() } }} />
<article class="wish" class:done={dream.status === 'fulfilled'}>
  <input class="complete" type="checkbox" checked={dream.status === 'fulfilled'} disabled={busy} onclick={e=>{e.preventDefault();ontoggle()}}
    aria-label={dream.status === 'fulfilled' ? t('ambitions.reopen') : t('ambitions.fulfill')} />
  <div class="body">
    <button class="title" onclick={onedit}>{dream.title}</button>
    {#if dream.note}<p class="note">{dream.note}</p>{/if}
    <div class="meta">
      {#if category}<span>{category}</span>{/if}
      {#if dream.status !== 'want'}<span class="status" class:active={dream.status === 'active'}>{t(`ambitions.status.${dream.status}`)}</span>{/if}
      {#if dream.deadline}<span class="date"><Icon name="calendar" size={12} />{shortDay(dream.deadline, i18n.locale)}</span>{/if}
      {#if dream.projectId}<a href="#/game/projects/{dream.projectId}"><Icon name="graph" size={12} />{project}</a>{/if}
    </div>
  </div>
  <details bind:this={menu} class="menu">
    <summary aria-label={t('ambitions.actions')} title={t('ambitions.actions')}><Icon name="more" size={20} /></summary>
    <div class="items">
      <button disabled={busy} onclick={() => act(onedit)}><Icon name="edit" size={16} />{t('ambitions.editAction')}</button>
      {#if !dream.projectId}<button disabled={busy} onclick={() => act(onconvert)}><Icon name="graph" size={16} />{t('ambitions.convert')}</button>{/if}
      <button class="danger" disabled={busy} onclick={() => act(ondelete)}><Icon name="trash" size={16} />{t('common.delete')}</button>
    </div>
  </details>
</article>

<style>
  .wish{position:relative;display:flex;align-items:flex-start;gap:var(--s-3);padding:var(--s-4);border:var(--bw) solid var(--c-border);border-radius:var(--radius-sm);box-shadow:var(--shadow-sm);background:var(--c-surface)}
  .complete{flex:none;width:18px;height:18px;margin:2px 0 0;accent-color:var(--c-green);cursor:pointer}
  .body{flex:1;min-width:0}.title{padding:0;border:0;background:none;font:inherit;font-weight:700;text-align:left;overflow-wrap:anywhere;cursor:pointer}
  .title:hover{text-decoration:underline}.note{color:var(--c-muted);font-size:var(--fs-sm);margin-top:var(--s-2);white-space:pre-wrap;display:-webkit-box;-webkit-line-clamp:2;line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;overflow-wrap:anywhere}
  .meta{display:flex;flex-wrap:wrap;align-items:center;gap:var(--s-1) var(--s-3);color:var(--c-muted);font-size:var(--fs-xs);margin-top:var(--s-2)}
  .meta:empty{display:none}.meta a,.date{display:inline-flex;align-items:center;gap:var(--s-1)}.meta a{color:var(--c-muted);max-width:100%;overflow-wrap:anywhere}
  .status{color:var(--c-green);font-weight:600}.status.active{color:var(--c-amber)}.done{background:var(--c-surface-2);box-shadow:none}.done .title{color:var(--c-muted)}
  .menu{flex:none}.menu summary{display:grid;place-items:center;list-style:none;width:28px;height:28px;border-radius:var(--radius-sm);cursor:pointer;color:var(--c-muted)}
  .menu summary::-webkit-details-marker{display:none}.menu summary:hover,.menu[open] summary{background:var(--c-surface-2);color:var(--c-text)}
  .items{position:absolute;z-index:5;right:var(--s-3);top:calc(var(--s-4) + 30px);min-width:210px;padding:var(--s-1);border:var(--bw) solid var(--c-border);border-radius:var(--radius-sm);box-shadow:var(--shadow-sm);background:var(--c-surface)}
  .items button{display:flex;align-items:center;gap:var(--s-2);width:100%;padding:var(--s-2);border:0;border-radius:var(--radius-sm);background:none;text-align:left;font-size:var(--fs-sm);cursor:pointer}
  .items button:hover{background:var(--c-surface-2)}.items .danger{color:var(--c-red)}.items button:disabled{opacity:.5}
</style>
