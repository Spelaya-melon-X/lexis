<script lang="ts">
  import { onMount, tick, untrack } from 'svelte'
  import type { Ambitions, BigProject, ProjectStep } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import { t } from '../../../lib/i18n/i18n.svelte'
  import { dependencyPath, GRAPH_NODE_H, GRAPH_NODE_W, layoutProjectGraph } from '../../../lib/projectGraph'

  let { project, steps, data, selected, busy, preview, linking, onselect, ontoggle, onadd }: {
    project: BigProject; steps: ProjectStep[]; data: Ambitions; selected: number | null; busy: boolean
    preview: ProjectStep | null; linking: boolean; onselect: (id: number) => void
    ontoggle: (step: ProjectStep) => void; onadd: (parent: number | null) => void
  } = $props()
  const marker = $props.id()
  const shown = $derived(preview ? [...steps,preview] : steps)
  const layout = $derived(layoutProjectGraph(shown))
  const byID = $derived(new Map(shown.map(s => [s.id,s])))
  const placed = $derived(new Map(layout.nodes.map(n => [n.id,n])))
  let zoom = $state(1)
  let dependencies = $state(true)
  let viewport: HTMLDivElement | undefined = $state()
  let dragging = $state<{x:number;y:number;left:number;top:number} | null>(null)

  const hasPreview = $derived(preview !== null)
  onMount(() => reveal(0))
  $effect(() => { if (hasPreview) untrack(() => { void tick().then(() => reveal(-1)) }) })

  function fit() {
    if (!viewport) return
    zoom = Math.max(.25,Math.min(1, (viewport.clientWidth-24)/layout.width, (viewport.clientHeight-24)/layout.height))
    viewport.scrollTo({left:0,top:0})
  }
  export function reveal(id:number) {
    const n=placed.get(id)
    if (!n || !viewport) return
    viewport.scrollTo({left:(n.x+GRAPH_NODE_W/2)*zoom-viewport.clientWidth/2,top:(n.y+GRAPH_NODE_H/2)*zoom-viewport.clientHeight/2,behavior:'smooth'})
  }
  function pan(e:PointerEvent) {
    if (!viewport || e.button !== 0 || !(e.target instanceof Element) || e.target.closest('button,input')) return
    dragging={x:e.clientX,y:e.clientY,left:viewport.scrollLeft,top:viewport.scrollTop}
    viewport.setPointerCapture(e.pointerId)
  }
  function move(e:PointerEvent) {
    if (dragging && viewport) { viewport.scrollLeft=dragging.left+dragging.x-e.clientX;viewport.scrollTop=dragging.top+dragging.y-e.clientY }
  }
</script>

<div class="graph" class:linking>
  <div class="toolbar">
    <span class="graph-label"><Icon name="graph" size={16} />{t('ambitions.graph.title')}</span>
    <label class="dependency-toggle"><input type="checkbox" bind:checked={dependencies} />{t('ambitions.graph.dependencies')}</label>
    <div class="zoom">
      <Button variant="ghost" size="sm" aria-label={t('ambitions.graph.zoomOut')} title={t('ambitions.graph.zoomOut')} disabled={zoom<=.25} onclick={()=>zoom=Math.max(.25,zoom-.15)}>−</Button>
      <span>{Math.round(zoom*100)}%</span>
      <Button variant="ghost" size="sm" icon="plus" aria-label={t('ambitions.graph.zoomIn')} title={t('ambitions.graph.zoomIn')} disabled={zoom>=1.75} onclick={()=>zoom=Math.min(1.75,zoom+.15)} />
      <Button variant="ghost" size="sm" onclick={fit}>{t('ambitions.graph.fit')}</Button>
    </div>
  </div>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrollable canvas needs keyboard focus for native arrow-key scrolling.) -->
  <div class="viewport" bind:this={viewport} role="region" aria-label={t('ambitions.graph.canvas')} tabindex="0"
    class:panning={dragging !== null} onpointerdown={pan} onpointermove={move} onpointerup={()=>dragging=null} onpointercancel={()=>dragging=null}>
    <div class="scaled" style:width="{layout.width*zoom}px" style:height="{layout.height*zoom}px">
      <svg width={layout.width} height={layout.height} style:transform="scale({zoom})" role="group" aria-label={project.title}>
        <defs><marker id={marker} markerWidth="7" markerHeight="7" refX="4" refY="3.5" orient="auto"><path d="M0 0 L7 3.5 L0 7" class="arrowhead" /></marker></defs>
        {#each layout.edges as e (`${e.from}-${e.to}`)}<path class="edge" class:preview-edge={e.to === -1} d={e.path} />{/each}
        {#if dependencies}
          {#each data.dependencies.filter(d=>placed.has(d.stepId)&&placed.has(d.requiresId)) as d (`${d.stepId}-${d.requiresId}`)}
            <path class="dependency" class:highlight={selected===d.stepId||selected===d.requiresId} d={dependencyPath(placed.get(d.stepId)!,placed.get(d.requiresId)!)} marker-end="url(#{marker})"><title>{byID.get(d.stepId)?.title} → {byID.get(d.requiresId)?.title}</title></path>
          {/each}
        {/if}
        {#each layout.nodes as n (n.id)}
          {@const step=byID.get(n.id)}
          {@const state=data.states[n.id]}
          <g class="placed" style:transform="translate({n.x}px,{n.y}px)">
            <foreignObject width={GRAPH_NODE_W} height={GRAPH_NODE_H} overflow="visible">
              <div class="node" role="group" aria-label={step?.title ?? project.title} onpointerdown={e=>{e.stopPropagation();if(e.target instanceof Element && !e.target.closest('button,input') && n.id>=0)onselect(n.id)}} class:root={n.id===0} class:chosen={(selected ?? 0)===n.id} class:done={state?.done} class:preview={n.id===-1} class:blocked={state && !state.done && !state.ready && state.leaf}>
                <div class="node-top">
                  {#if step && state?.leaf}<input type="checkbox" checked={state.done} disabled={busy||(!state.done&&!state.ready)} onclick={e=>{e.preventDefault();ontoggle(step)}} aria-label={state.done?t('ambitions.reopen'):t('ambitions.complete')} />{:else}<span class="node-icon"><Icon name={n.id===0?'target':'steps'} size={16}/></span>{/if}
                  <button class="node-title" title={step?.title ?? project.title} disabled={n.id===-1} onclick={()=>onselect(n.id)}>{step?.title ?? project.title}</button>
                </div>
                <div class="node-bottom">
                  {#if n.id===-1}<span>{t('ambitions.graph.preview')}</span>
                  {:else if n.id===0}<span>{t('ambitions.graph.goal')}</span><strong>{data.summaries[project.id]?.progress ?? 0}%</strong>
                  {:else if state}<span>{state.done?'✓ ':''}{state.done?t('ambitions.done'):state.leaf?(state.ready?t('ambitions.ready'):t('ambitions.blocked')):t('ambitions.stage')}</span>{#if !state.leaf}<strong>{Math.round(state.progress*100)}%</strong>{/if}{/if}
                </div>
                {#if n.id!==-1}<button class="add" disabled={busy||!!step?.done||linking} onclick={()=>onadd(step?.id ?? null)} aria-label={t('ambitions.graph.addTo',{title:step?.title ?? project.title})} title={t('ambitions.addStep')}><Icon name="plus" size={16}/></button>{/if}
              </div>
            </foreignObject>
          </g>
        {/each}
      </svg>
    </div>
  </div>
  <div class="legend"><span><i></i>{t('ambitions.graph.treeEdge')}</span><span><i class="dashed"></i>{t('ambitions.graph.waitEdge')}</span><span class="hint">{linking?t('ambitions.graph.pickDependency'):t('ambitions.graph.hint')}</span></div>
</div>

<style>
  .graph{display:flex;flex-direction:column;min-width:0;min-height:420px;height:100%;border:var(--bw) solid var(--c-border);border-radius:var(--radius);background:var(--c-surface);overflow:hidden}
  .toolbar{display:flex;align-items:center;flex-wrap:wrap;gap:var(--s-2) var(--s-4);padding:var(--s-2) var(--s-3);border-bottom:var(--bw) solid var(--c-line)}
  .graph-label{display:flex;align-items:center;gap:var(--s-2);font-size:var(--fs-sm);font-weight:700}.zoom{display:flex;align-items:center;gap:var(--s-1);margin-left:auto}.zoom>span{font-size:var(--fs-xs);font-variant-numeric:tabular-nums;min-width:36px;text-align:center}
  .dependency-toggle{display:flex;align-items:center;gap:var(--s-1);font-size:var(--fs-xs);color:var(--c-muted)}input{accent-color:var(--c-green)}
  .viewport{flex:1;min-height:320px;overflow:auto;background-color:var(--c-surface-2);background-image:radial-gradient(var(--c-line) 1px,transparent 1px);background-size:20px 20px;cursor:grab;touch-action:pan-x pan-y}
  .viewport.panning{cursor:grabbing;user-select:none}.scaled{position:relative;min-width:100%;min-height:100%;overflow:hidden}svg{position:absolute;left:0;top:0;display:block;transform-origin:0 0;overflow:visible;font-family:var(--font)}
  .edge{fill:none;stroke:var(--c-border);stroke-width:2}.dependency{fill:none;stroke:var(--c-amber);stroke-width:1.5;stroke-dasharray:5 5;opacity:.65}.dependency.highlight{stroke-width:2.5;opacity:1}.arrowhead{fill:var(--c-amber)}.preview-edge{stroke-dasharray:5 5;stroke:var(--c-muted)}
  .placed{transition:transform var(--dur) var(--ease-out)}
  .node{position:relative;display:flex;flex-direction:column;justify-content:space-between;gap:var(--s-2);width:100%;height:100%;padding:var(--s-3);border:var(--bw) solid var(--c-border);border-radius:var(--radius-sm);box-shadow:var(--shadow-sm);background:var(--c-surface);color:var(--c-text);font-family:var(--font);cursor:default}
  .root{background:var(--c-green-soft)}.chosen{outline:var(--bw) solid var(--c-focus);outline-offset:3px}.done{background:var(--c-surface-2);box-shadow:none;color:var(--c-muted)}.preview{border-style:dashed;box-shadow:none;opacity:.75}.blocked{border-color:var(--c-muted)}
  .node-top{display:flex;gap:var(--s-2);align-items:flex-start;padding-right:var(--s-4);min-height:38px}.node-top input{width:16px;height:16px;flex:none;margin:2px 0 0;cursor:pointer}.node-icon{margin-top:2px}
  .node-title{padding:0;border:0;background:none;font:inherit;font-size:var(--fs-sm);font-weight:700;line-height:1.4;text-align:left;overflow-wrap:anywhere;display:-webkit-box;-webkit-line-clamp:2;line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;cursor:pointer;color:inherit}.node-title:hover{text-decoration:underline}
  .node-bottom{display:flex;justify-content:space-between;gap:var(--s-2);font-size:var(--fs-xs);align-items:center}.node-bottom strong{color:var(--c-green)}
  .add{position:absolute;right:var(--s-1);top:var(--s-1);display:grid;place-items:center;width:24px;height:24px;border:0;border-radius:var(--radius-sm);background:none;opacity:0;cursor:pointer;color:var(--c-muted)}.node:hover .add,.node:focus-within .add,.chosen .add{opacity:1}.add:hover{background:var(--c-green-soft);color:var(--c-text)}.add:disabled{opacity:.3;cursor:default}
  .legend{display:flex;flex-wrap:wrap;gap:var(--s-2) var(--s-4);padding:var(--s-2) var(--s-3);font-size:var(--fs-xs);color:var(--c-muted);border-top:var(--bw) solid var(--c-line)}.legend>span{display:flex;align-items:center;gap:var(--s-2)}.legend i{width:20px;border-top:2px solid var(--c-border)}.legend i.dashed{border-top:2px dashed var(--c-amber)}.hint{margin-left:auto}.linking .viewport{cursor:crosshair}
  @media(pointer:coarse){.add{opacity:1}}
</style>
