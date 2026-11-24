<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import { api } from '../../../lib/api/client'
  import type { AmbitionInput, AmbitionKind, ProjectStep } from '../../../lib/api/types'
  import { blankAmbition, emptyAmbitions, stepInput } from '../../../lib/ambitions'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import ProgressBar from '../../../lib/components/ProgressBar.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { shortDay } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import Editor from '../../ambitions/Editor.svelte'
  import ProjectGraph from './ProjectGraph.svelte'

  let data = $state(emptyAmbitions())
  let loading = $state(true)
  let busy = $state(false)
  let failure = $state('')
  let selected = $state<number | null>(null)
  let adding = $state(false)
  let parentId = $state<number | null>(null)
  let stepTitle = $state('')
  let linking = $state<number | null>(null)
  let graph: ProjectGraph | undefined = $state()
  let stepInputElement: HTMLInputElement | undefined = $state()
  let editor = $state<{kind:AmbitionKind;id:number|null;initial:AmbitionInput} | null>(null)
  let deleting = $state<{kind:AmbitionKind;id:number} | null>(null)
  const project = $derived(data.projects.find(p => String(p.id) === route.rest))
  const projectID = $derived(project?.id)
  const steps = $derived(data.steps.filter(s => s.projectId === projectID))
  const chosen = $derived(steps.find(s => s.id === selected))
  const chosenState = $derived(chosen ? data.states[chosen.id] : undefined)
  const summary = $derived(project ? data.summaries[project.id] : undefined)
  const required = $derived(data.dependencies.filter(d => d.stepId === selected))
  const preview = $derived<ProjectStep | null>(adding && project && stepTitle.trim() ? {
    ...blankAmbition(stepTitle,steps.filter(s=>s.parentId===parentId).length),id:-1,projectId:project.id,parentId,done:false,
  } : null)

  $effect(() => {
    projectID
    untrack(() => { selected=null;adding=false;stepTitle='';parentId=null;linking=null;editor=null })
  })
  async function load() {
    loading=true
    try { data=await api.ambitions.get();failure='' }
    catch(e) { failure=errorText(e) }
    finally { loading=false }
  }
  onMount(load)
  async function run(action:()=>ReturnType<typeof api.ambitions.get>) {
    if (busy) return false
    busy=true
    try { data=await action();return true }
    catch(e) { toasts.error(errorText(e));return false }
    finally { busy=false }
  }
  async function beginAdd(parent:number|null) {
    selected=parent;parentId=parent;adding=true;stepTitle='';linking=null
    await tick()
    stepInputElement?.focus()
    stepInputElement?.scrollIntoView({block:'nearest'})
  }
  async function addStep(e:SubmitEvent) {
    e.preventDefault()
    if (!project || !stepTitle.trim()) return
    const projectId=project.id
    const ids=new Set(steps.map(s=>s.id))
    if (await run(()=>api.ambitions.create('steps',{...blankAmbition(stepTitle,steps.filter(s=>s.parentId===parentId).length),projectId,parentId,done:false,requires:[]}))) {
      stepTitle=''
      const created=steps.find(s=>!ids.has(s.id))
      if (created) { selected=created.id;await tick();graph?.reveal(created.id) }
      stepInputElement?.focus()
    }
  }
  async function pick(id:number) {
    if (id<0) return
    if (linking!==null) {
      const source=steps.find(s=>s.id===linking)
      if (!source || id===0 || id===source.id) return
      const input=stepInput(data,source)
      if (input.requires?.includes(id)) { linking=null;return }
      if (await run(()=>api.ambitions.update('steps',source.id,{...input,requires:[...(input.requires ?? []),id]}))) linking=null
      return
    }
    selected=id===0?null:id
    adding=false
  }
  async function toggle(step:ProjectStep) {
    await run(()=>api.ambitions.update('steps',step.id,{...stepInput(data,step),done:!data.states[step.id]?.done}))
  }
  function editStep(step:ProjectStep) { editor={kind:'steps',id:step.id,initial:stepInput(data,step)} }
  async function save(input:AmbitionInput) {
    const e=editor
    if (!e) return
    const ids=new Set(data.projects.map(p=>p.id))
    if (await run(()=>e.id===null?api.ambitions.create(e.kind,input):api.ambitions.update(e.kind,e.id,input))) {
      editor=null
      if (e.kind==='projects' && e.id===null) {
        const p=data.projects.find(p=>!ids.has(p.id))
        if (p) location.hash=`#/game/projects/${p.id}`
      }
    }
  }
  async function remove() {
    const d=deleting
    if (d && await run(()=>api.ambitions.remove(d.kind,d.id))) {
      deleting=null;linking=null;adding=false
      if (d.kind==='projects') location.hash='#/game/projects'
      if (!data.steps.some(s=>s.id===selected)) selected=null
    }
  }
  async function unlink(requiresId:number) {
    if (!chosen) return
    const step=chosen,input=stepInput(data,step)
    await run(()=>api.ambitions.update('steps',step.id,{...input,requires:input.requires?.filter(id=>id!==requiresId)}))
  }
</script>

<div class="page" class:board={!!project}>
  {#if loading}<div class="state"><Spinner size={28}/></div>
  {:else if failure}<p class="error" role="alert">{failure}</p><Button onclick={load}>{t('common.retry')}</Button>
  {:else if project && summary}
    <div class="head">
      <div class="heading">
        <a class="back" href="#/game/projects"><Icon name="chevronLeft" size={16}/>{t('nav.game.projects')}</a>
        <h1>{project.title}</h1>
      </div>
      <Button icon="plus" variant="primary" disabled={busy} onclick={()=>beginAdd(selected)}>{t('ambitions.addStep')}</Button>
    </div>
    <div class="overview">
      <strong>{summary.progress}%</strong><div class="progress"><ProgressBar value={summary.progress/100} label={t('ambitions.progress')}/></div>
      <span>{t('ambitions.counts',{done:summary.done,open:summary.open,overdue:summary.overdue})}</span>
    </div>
    {#if linking!==null}<div class="link-banner" role="status"><Icon name="graph" size={18}/><span>{t('ambitions.graph.pickDependency')}</span><Button size="sm" variant="ghost" onclick={()=>linking=null}>{t('common.cancel')}</Button></div>{/if}
    <div class="workspace">
      <ProjectGraph bind:this={graph} {project} {steps} {data} {selected} {busy} {preview} linking={linking!==null} onselect={id=>void pick(id)} ontoggle={step=>void toggle(step)} onadd={parent=>void beginAdd(parent)}/>
      <aside class="inspector" aria-label={t('ambitions.graph.details')}>
        {#if adding}
          <form class="composer" onsubmit={addStep}>
            <div class="panel-head"><h2>{t('ambitions.addStep')}</h2><Button variant="ghost" size="sm" icon="x" aria-label={t('common.close')} onclick={()=>{adding=false;stepTitle=''}}/></div>
            <p class="muted small">{t('ambitions.graph.addingTo')}: <strong>{steps.find(s=>s.id===parentId)?.title ?? project.title}</strong></p>
            <input class="input" bind:this={stepInputElement} bind:value={stepTitle} required maxlength="200" disabled={busy} placeholder={t('ambitions.stepPlaceholder')} aria-label={t('ambitions.title')}/>
            <Button type="submit" variant="primary" size="sm" icon="plus" loading={busy} disabled={!stepTitle.trim()}>{t('ambitions.addStep')}</Button>
            <p class="muted small">{t('ambitions.graph.previewHint')}</p>
          </form>
        {/if}
        {#if chosen && chosenState}
          <div class="panel-head"><span class="eyebrow">{chosenState.leaf?t('ambitions.graph.task'):t('ambitions.stage')}</span>
            <Button variant="ghost" size="sm" icon="edit" aria-label={t('ambitions.editAction')} title={t('ambitions.editAction')} disabled={busy} onclick={()=>editStep(chosen)}/>
            <Button variant="ghost" size="sm" icon="trash" aria-label={t('common.delete')} title={t('common.delete')} disabled={busy} onclick={()=>deleting={kind:'steps',id:chosen.id}}/>
          </div>
          <h2>{chosen.title}</h2>
          <span class="status" class:success={chosenState.done||chosenState.ready}>{chosenState.done?'✓ ':''}{chosenState.done?t('ambitions.done'):chosenState.ready?t('ambitions.ready'):chosenState.leaf?t('ambitions.blocked'):t('ambitions.stage')}</span>
          {#if chosen.note}<p class="note">{chosen.note}</p>{/if}
          {#if chosen.deadline}<p class="small" class:error={chosenState.overdue}><Icon name="calendar" size={14}/>{shortDay(chosen.deadline,i18n.locale)}{chosenState.overdue?` · ${t('ambitions.overdue')}`:''}</p>{/if}
          {#if chosenState.leaf}<Button size="sm" icon={chosenState.done?'refresh':'check'} disabled={busy||(!chosenState.done&&!chosenState.ready)} onclick={()=>toggle(chosen)}>{chosenState.done?t('ambitions.reopen'):t('ambitions.complete')}</Button>{/if}
          <Button variant="ghost" size="sm" icon="plus" disabled={busy||chosen.done} onclick={()=>beginAdd(chosen.id)}>{t('ambitions.child')}</Button>
          <section class="dependencies">
            <h3>{t('ambitions.requires')}</h3>
            {#each required as d (d.requiresId)}
              <div class="requirement"><button class="text-link" onclick={()=>{selected=d.requiresId;graph?.reveal(d.requiresId)}}>{data.states[d.requiresId]?.done?'✓ ':''}{steps.find(s=>s.id===d.requiresId)?.title}</button><Button variant="ghost" size="sm" icon="x" disabled={busy} aria-label={t('ambitions.graph.unlink')} title={t('ambitions.graph.unlink')} onclick={()=>unlink(d.requiresId)}/></div>
            {/each}
            {#if chosenState.blockedBy.length}<p class="small muted">{t('ambitions.blockedBy')}: {chosenState.blockedBy.map(id=>steps.find(s=>s.id===id)?.title).join(' · ')}</p>{/if}
            <Button size="sm" variant="ghost" icon="graph" disabled={busy} onclick={()=>{linking=chosen.id;adding=false}}>{t('ambitions.graph.connect')}</Button>
            <p class="small muted">{t('ambitions.graph.connectHint')}</p>
          </section>
        {:else}
          <div class="panel-head"><span class="eyebrow">{t('ambitions.graph.goal')}</span>
            <Button variant="ghost" size="sm" icon="edit" aria-label={t('ambitions.editAction')} title={t('ambitions.editAction')} disabled={busy} onclick={()=>editor={kind:'projects',id:project.id,initial:{title:project.title,note:project.note,deadline:project.deadline,position:project.position}}}/>
            <Button variant="ghost" size="sm" icon="trash" aria-label={t('common.delete')} title={t('common.delete')} disabled={busy} onclick={()=>deleting={kind:'projects',id:project.id}}/>
          </div>
          <h2>{project.title}</h2>
          {#if project.note}<p class="note">{project.note}</p>{/if}
          {#if project.deadline}<p class="small"><Icon name="calendar" size={14}/>{shortDay(project.deadline,i18n.locale)}</p>{/if}
          {#if !steps.length}<p class="muted small">{t('ambitions.stepEmpty')}</p>{/if}
          <section class="next"><h3>{t('ambitions.next')}</h3>
            {#each summary.next as id}<button class="next-step" onclick={()=>{selected=id;graph?.reveal(id)}}><Icon name="chevronRight" size={14}/><span>{steps.find(s=>s.id===id)?.title}</span></button>{/each}
            {#if !summary.next.length}<p class="muted small">{summary.open?t('ambitions.noReady'):steps.length?t('ambitions.finished'):t('ambitions.graph.startHint')}</p>{/if}
          </section>
          <details class="rule"><summary>{t('ambitions.graph.progressRule')}</summary><p>{t('ambitions.progressHelp')}</p></details>
        {/if}
      </aside>
    </div>
  {:else if route.rest}<p role="alert">{t('ambitions.projectMissing')}</p><a href="#/game/projects">{t('nav.game.projects')}</a>
  {:else}
    <div class="head"><div><h1>{t('nav.game.projects')}</h1><p class="muted subtitle">{t('ambitions.graph.intro')}</p></div><Button variant="primary" icon="plus" onclick={()=>editor={kind:'projects',id:null,initial:blankAmbition('',data.projects.length)}}>{t('ambitions.new.projects')}</Button></div>
    {#if !data.projects.length}<div class="empty"><Icon name="graph" size={36}/><h2>{t('ambitions.projectEmpty')}</h2><p class="muted">{t('ambitions.graph.empty')}</p><Button icon="plus" onclick={()=>editor={kind:'projects',id:null,initial:blankAmbition()}}>{t('ambitions.new.projects')}</Button></div>{/if}
    <div class="projects">{#each data.projects.slice().sort((a,b)=>a.position-b.position||a.id-b.id) as p (p.id)}
      {@const sum=data.summaries[p.id]!}
      <a href="#/game/projects/{p.id}" class="project-card"><div class="card-head"><Icon name="graph" size={20}/><h2>{p.title}</h2><Icon name="chevronRight" size={18}/></div>
        {#if p.note}<p class="project-note">{p.note}</p>{/if}
        <div class="card-progress"><ProgressBar value={sum.progress/100} label={t('ambitions.progress')}/><strong>{sum.progress}%</strong></div>
        <p class="small muted">{t('ambitions.counts',{done:sum.done,open:sum.open,overdue:sum.overdue})}</p>
        {#if p.deadline}<p class="small muted">{shortDay(p.deadline,i18n.locale)}</p>{/if}
      </a>
    {/each}</div>
  {/if}
</div>
{#if editor}<Editor {...editor} {data} saving={busy} onsave={save} onclose={()=>editor=null}/>{/if}
<ConfirmDialog open={deleting!==null} title={t('ambitions.deleteTitle')} danger loading={busy} onconfirm={remove} oncancel={()=>deleting=null}><p>{deleting?.kind==='projects'?t('ambitions.deleteProject'):t('ambitions.deleteStep')}</p></ConfirmDialog>

<style>
  .page{display:flex;flex-direction:column;gap:var(--s-4);min-height:0;overflow-y:auto;padding:0 var(--s-1) var(--s-4) 0}.board{height:100%;overflow:hidden}
  .head{display:flex;align-items:flex-start;justify-content:space-between;gap:var(--s-3);flex-wrap:wrap}.head h1{font-size:var(--fs-xl);overflow-wrap:anywhere}.heading{min-width:0;flex:1}.back{display:inline-flex;align-items:center;gap:var(--s-1);font-size:var(--fs-xs);color:var(--c-muted);text-decoration:none;margin-bottom:var(--s-2)}.back:hover{color:var(--c-text)}.subtitle{font-size:var(--fs-sm);margin-top:var(--s-2)}
  .overview{display:flex;align-items:center;gap:var(--s-3);flex-wrap:wrap}.overview>strong{font-size:var(--fs-lg);font-variant-numeric:tabular-nums}.overview>span{font-size:var(--fs-xs);color:var(--c-muted)}.progress{width:160px}
  .workspace{display:grid;grid-template-columns:minmax(0,1fr) 270px;gap:var(--s-4);flex:1;min-height:0}.inspector{display:flex;flex-direction:column;align-items:stretch;gap:var(--s-3);overflow-y:auto;padding:var(--s-4);border:var(--bw) solid var(--c-line);border-radius:var(--radius);background:var(--c-surface);min-width:0}
  .panel-head{display:flex;align-items:center;gap:var(--s-1)}.panel-head .eyebrow,.panel-head h2{margin-right:auto}.eyebrow{font-size:var(--fs-xs);text-transform:uppercase;letter-spacing:.06em;color:var(--c-muted);font-weight:600}
  .inspector h2{font-size:var(--fs-lg);overflow-wrap:anywhere}.inspector h3{font-size:var(--fs-sm)}.note{white-space:pre-wrap;overflow-wrap:anywhere;font-size:var(--fs-sm);line-height:1.6}.small{font-size:var(--fs-xs);overflow-wrap:anywhere}.inspector p.small{display:flex;align-items:center;gap:var(--s-1)}
  .status{align-self:flex-start;font-size:var(--fs-xs);padding:var(--s-1) var(--s-2);border-radius:var(--radius-sm);background:var(--c-surface-2);color:var(--c-muted)}.status.success{background:var(--c-green-soft);color:var(--c-green)}
  .dependencies,.next{display:flex;flex-direction:column;gap:var(--s-2);padding-top:var(--s-4);border-top:var(--bw) solid var(--c-line)}.requirement{display:flex;align-items:center;gap:var(--s-1)}.text-link{flex:1;text-align:left;border:0;background:none;padding:0;overflow-wrap:anywhere;font-size:var(--fs-sm);cursor:pointer}.text-link:hover{text-decoration:underline}
  .next-step{display:flex;align-items:flex-start;gap:var(--s-1);text-align:left;padding:var(--s-2) 0;border:0;background:none;cursor:pointer;font-size:var(--fs-sm)}.next-step:hover{color:var(--c-green)}.next-step span{overflow-wrap:anywhere}
  .composer{display:flex;flex-direction:column;gap:var(--s-3);padding-bottom:var(--s-4);border-bottom:var(--bw) solid var(--c-line)}.composer h2{font-size:var(--fs-sm)}.rule{font-size:var(--fs-xs);color:var(--c-muted);margin-top:auto}.rule summary{cursor:pointer}.rule p{padding-top:var(--s-2);line-height:1.6}
  .link-banner{display:flex;align-items:center;gap:var(--s-2);padding:var(--s-2) var(--s-3);background:var(--c-amber-soft);border:var(--bw) solid var(--c-amber);border-radius:var(--radius-sm);font-size:var(--fs-sm)}.link-banner span{flex:1}
  .projects{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(300px,100%),1fr));gap:var(--s-4)}.project-card{display:flex;flex-direction:column;gap:var(--s-3);padding:var(--s-4);border:var(--bw) solid var(--c-border);border-radius:var(--radius-sm);box-shadow:var(--shadow-sm);color:inherit;text-decoration:none;background:var(--c-surface);transition:transform var(--dur-fast) var(--ease-out)}.project-card:hover{transform:translate(-1px,-1px)}
  .card-head{display:flex;align-items:flex-start;gap:var(--s-2)}.card-head h2{flex:1;font-size:var(--fs-md);overflow-wrap:anywhere}.card-progress{display:grid;grid-template-columns:1fr auto;align-items:center;gap:var(--s-3);font-size:var(--fs-sm)}.project-note{font-size:var(--fs-sm);color:var(--c-muted);overflow:hidden;display:-webkit-box;-webkit-line-clamp:2;line-clamp:2;-webkit-box-orient:vertical;overflow-wrap:anywhere}
  .state,.empty{display:flex;flex-direction:column;align-items:center;gap:var(--s-3);padding:var(--s-7) var(--s-4);text-align:center}.empty h2{font-size:var(--fs-lg)}.empty p{max-width:400px;font-size:var(--fs-sm)}.error{color:var(--c-red)}
  @media(max-width:1000px){.workspace{grid-template-columns:minmax(0,1fr) 240px}}
  @media(max-height:700px){.board{overflow-y:auto}.workspace{height:440px;flex:none}}
  @media(max-width:760px){.board{overflow-y:auto}.workspace{display:flex;flex-direction:column;flex:none}.workspace :global(.graph){height:440px;flex:none}.inspector{max-height:none;overflow:visible}.heading{min-width:180px}.overview{gap:var(--s-2)}}
</style>
