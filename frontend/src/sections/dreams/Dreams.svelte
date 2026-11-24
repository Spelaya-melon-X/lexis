<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../../lib/api/client'
  import type { AmbitionInput, AmbitionKind, Dream } from '../../lib/api/types'
  import { blankAmbition, emptyAmbitions } from '../../lib/ambitions'
  import { errorText, t } from '../../lib/i18n/i18n.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'
  import ConfirmDialog from '../../lib/components/ConfirmDialog.svelte'
  import Editor from '../ambitions/Editor.svelte'
  import Button from '../../lib/components/Button.svelte'
  import Icon from '../../lib/components/Icon.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import DreamRow from './DreamRow.svelte'
  let data = $state(emptyAmbitions())
  let loading = $state(true), busy = $state(false), failure = $state('')
  let category = $state<number | 'all' | null>('all')
  let editor = $state<{
    kind: AmbitionKind
    id: number | null
    initial: AmbitionInput
  } | null>(null)
  let deleting = $state<{
    kind: AmbitionKind
    id: number
  } | null>(null)
  let converting = $state<Dream | null>(null), projectTitle = $state('')
  const dreams = $derived(data.dreams.filter(d => category === 'all' || d.categoryId === category).slice().sort((a, b) => a.position - b.position || a.id - b.id))
  async function load() {
    loading = true
    failure = ''
    try {
      data = await api.ambitions.get()
    } catch (e) {
      failure = errorText(e)
    } finally {
      loading = false
    }
  }
  onMount(load)
  async function run(action: () => ReturnType<typeof api.ambitions.get>) {
    if (busy)
      return
    busy = true
    try {
      data = await action()
      return true
    } catch (e) {
      toasts.error(errorText(e))
      return false
    } finally {
      busy = false
    }
  }
  function editDream(d: Dream) {
    editor = { kind: 'dreams', id: d.id, initial: { title: d.title, note: d.note, deadline: d.deadline, position: d.position, status: d.status, categoryId: d.categoryId, projectId: d.projectId } }
  }
  async function save(v: AmbitionInput) {
    const e = editor
    if (!e)
      return
    if (await run(() => e.id === null ? api.ambitions.create(e.kind, v) : api.ambitions.update(e.kind, e.id, v)))
      editor = null
  }
  async function remove() {
    const d = deleting
    if (d && await run(() => api.ambitions.remove(d.kind, d.id))) {
      if (d.kind === 'categories' && category === d.id)
        category = null
      deleting = null
    }
  }
</script>

<div class="page">
  <div class="head">
    <div><h1>{t('ambitions.dreamHeading')}</h1><p class="muted">{t('ambitions.dreamSubtitle')}</p></div>
    <Button variant="primary" icon="plus" disabled={loading || busy} onclick={() => editor={kind:'dreams',id:null,initial:{...blankAmbition('',data.dreams.length),categoryId:category === 'all' ? null : category}}}>{t('ambitions.new.dreams')}</Button>
  </div>
  {#if loading}<div class="state"><Spinner size={28} /></div>
  {:else if failure}<p class="error" role="alert">{failure}</p><Button onclick={load}>{t('common.retry')}</Button>
  {:else}
    <nav class="categories" aria-label={t('ambitions.category')}>
      <button class:active={category === 'all'} onclick={() => category='all'}>{t('ambitions.all')}<span>{data.dreams.length}</span></button>
      <button class:active={category === null} onclick={() => category=null}>{t('ambitions.uncategorized')}</button>
      {#each data.categories.slice().sort((a,b) => a.position-b.position || a.id-b.id) as c}
        <button class:active={category === c.id} onclick={() => category=c.id}>{c.title}</button>
      {/each}
      <Button variant="ghost" size="sm" icon="plus" aria-label={t('ambitions.new.categories')} title={t('ambitions.new.categories')} onclick={() => editor={kind:'categories',id:null,initial:blankAmbition('',data.categories.length)}} />
    </nav>
    {#if typeof category === 'number'}
      {@const c=data.categories.find(c => c.id === category)}
      {#if c}<div class="category-head"><h2>{c.title}</h2>
        <Button variant="ghost" size="sm" icon="edit" aria-label={t('ambitions.editAction')} title={t('ambitions.editAction')} disabled={busy} onclick={() => editor={kind:'categories',id:c.id,initial:blankAmbition(c.title,c.position)}} />
        <Button variant="ghost" size="sm" icon="trash" aria-label={t('common.delete')} title={t('common.delete')} disabled={busy} onclick={() => deleting={kind:'categories',id:c.id}} />
      </div>{/if}
    {/if}
    {#if dreams.length === 0}
      <div class="empty"><Icon name="target" size={32} /><h2>{t('ambitions.dreamEmpty')}</h2><p class="muted">{t('ambitions.dreamEmptyNew')}</p>
        <Button icon="plus" onclick={() => editor={kind:'dreams',id:null,initial:{...blankAmbition('',data.dreams.length),categoryId:category === 'all' ? null : category}}}>{t('ambitions.new.dreams')}</Button>
      </div>
    {:else}
      <div class="wishes">
        {#each dreams as d (d.id)}
          <DreamRow dream={d} category={data.categories.find(c => c.id === d.categoryId)?.title} project={data.projects.find(p => p.id === d.projectId)?.title} {busy}
            onedit={() => editDream(d)}
            ontoggle={() => run(() => api.ambitions.update('dreams',d.id,{title:d.title,note:d.note,deadline:d.deadline,position:d.position,categoryId:d.categoryId,projectId:d.projectId,status:d.status === 'fulfilled' ? 'want' : 'fulfilled'}))}
            onconvert={() => {converting=d;projectTitle=''}} ondelete={() => deleting={kind:'dreams',id:d.id}} />
        {/each}
      </div>
    {/if}
  {/if}
</div>
{#if editor}<Editor {...editor} {data} saving={busy} onsave={save} onclose={() => editor=null} />{/if}
<ConfirmDialog open={deleting !== null} title={t('ambitions.deleteTitle')} danger loading={busy} onconfirm={remove} oncancel={() => deleting=null}>
  <p>{deleting?.kind === 'categories' ? t('ambitions.deleteCategory') : t('ambitions.deleteDream')}</p>
</ConfirmDialog>
<ConfirmDialog open={converting !== null} title={t('ambitions.convert')} loading={busy} oncancel={() => converting=null}
  onconfirm={async () => {const d=converting;if(!projectTitle.trim()){toasts.error(t('errors.invalid_title'));return}if(d && await run(() => api.ambitions.convert(d.id,blankAmbition(projectTitle,data.projects.length)))) converting=null}}>
  <div class="convert"><p>{t('ambitions.convertHelp')}</p><label>{t('ambitions.projectTitle')}<input class="input" bind:value={projectTitle} maxlength="200" required disabled={busy} /></label></div>
</ConfirmDialog>

<style>
  .page{display:flex;flex-direction:column;gap:var(--s-5);min-height:0;overflow-y:auto;padding:0 var(--s-1) var(--s-5) 0}
  .head{display:flex;justify-content:space-between;align-items:flex-start;gap:var(--s-4);flex-wrap:wrap}.head h1{font-size:var(--fs-xl)}.head p{margin-top:var(--s-2);font-size:var(--fs-sm)}
  .categories{display:flex;flex-wrap:wrap;align-items:center;gap:var(--s-1);border-bottom:var(--bw) solid var(--c-line);padding-bottom:var(--s-3)}
  .categories>button{display:flex;align-items:center;gap:var(--s-2);padding:var(--s-2) var(--s-3);border:var(--bw) solid transparent;border-radius:var(--radius-sm);background:none;color:var(--c-muted);font-weight:600;font-size:var(--fs-sm);cursor:pointer;overflow-wrap:anywhere}
  .categories>button:hover{background:var(--c-surface-2)}.categories>button.active{border-color:var(--c-border);background:var(--c-green-soft);color:var(--c-text)}
  .categories span{font-size:var(--fs-xs);font-weight:400}.category-head{display:flex;align-items:center;gap:var(--s-1)}.category-head h2{font-size:var(--fs-lg);margin-right:auto;overflow-wrap:anywhere}
  .wishes{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(340px,100%),1fr));align-items:start;gap:var(--s-3);padding-bottom:var(--s-7)}
  .state,.empty{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:var(--s-3);padding:var(--s-7) var(--s-4);text-align:center}.empty h2{font-size:var(--fs-lg)}.empty p{max-width:400px;font-size:var(--fs-sm)}
  .error{color:var(--c-red)}.convert{display:flex;flex-direction:column;gap:var(--s-4)}.convert label{display:flex;flex-direction:column;gap:var(--s-2);font-size:var(--fs-sm)}
</style>
