<script lang="ts">
  // «Граф дела» (§18.3, B2): дерево шагов своим SVG. Выполненные — светло-серые, невыполненные — белые.
  // Раскладка — lib/taskTree.layoutTree; клик по узлу открывает дело/шаг в диалоге правки.
  import { api } from '../../../lib/api/client'
  import type { Task } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Modal from '../../../lib/components/Modal.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { layoutTree } from '../../../lib/taskTree'
  import { blocksText } from '../format'

  interface Props {
    taskId: number | null // любой узел дерева (null — закрыт)
    version: number // меняется после правок — перечитать дерево
    onclose: () => void
    onopen: (t: Task) => void
  }
  let { taskId, version, onclose, onopen }: Props = $props()

  const NODE_W = 168
  const NODE_H = 52
  const GAP_X = 20
  const GAP_Y = 44
  const PAD = 8

  let tree = $state<Task[] | null>(null)
  let error = $state<string | null>(null)

  $effect(() => {
    void version
    if (taskId === null) {
      tree = null
      return
    }
    const id = taskId
    api.tasks
      .tree(id)
      .then((ts) => {
        if (taskId === id) tree = ts
        error = null
      })
      .catch((e: unknown) => (error = errorText(e)))
  })

  const layout = $derived(tree ? layoutTree(tree) : null)
  const byId = $derived(new Map((tree ?? []).map((x) => [x.id, x])))
  const leaves = $derived((tree ?? []).filter((x) => x.leaf))
  const root = $derived(tree?.[0] ?? null)

  const px = (x: number) => PAD + x * (NODE_W + GAP_X)
  const py = (depth: number) => PAD + depth * (NODE_H + GAP_Y)

  /** Название в узле: SVG сам не переносит строки — режем по длине. */
  function clip(s: string, max = 22): string {
    return s.length > max ? s.slice(0, max - 1) + '…' : s
  }
</script>

<Modal open={taskId !== null} title={t('game.graph')} width={960} {onclose}>
  {#if error && !tree}
    <p class="error">{error}</p>
  {:else if !tree || !layout || !root}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="head">
      <strong>{root.title}</strong>
      <span class="muted">
        {t('game.graphProgress', { done: leaves.filter((x) => x.done).length, total: leaves.length })} · Σ {blocksText(root.blocks)}
      </span>
    </div>
    <div class="canvas">
      <svg
        width={PAD * 2 + layout.width * NODE_W + (layout.width - 1) * GAP_X}
        height={PAD * 2 + layout.depth * NODE_H + (layout.depth - 1) * GAP_Y}
        role="tree"
        aria-label={root.title}
      >
        {#each layout.edges as e (e.to)}
          {@const a = layout.nodes.get(e.from)!}
          {@const b = layout.nodes.get(e.to)!}
          {@const x1 = px(a.x) + NODE_W / 2}
          {@const y1 = py(a.depth) + NODE_H}
          {@const x2 = px(b.x) + NODE_W / 2}
          {@const y2 = py(b.depth)}
          <path class="edge" d="M{x1} {y1} C {x1} {(y1 + y2) / 2}, {x2} {(y1 + y2) / 2}, {x2} {y2}" />
        {/each}
        {#each [...layout.nodes.values()] as n (n.id)}
          {@const task = byId.get(n.id)!}
          <g
            class="node"
            class:done={task.done}
            class:leaf={task.leaf}
            transform="translate({px(n.x)} {py(n.depth)})"
            role="treeitem"
            aria-selected="false"
            tabindex="0"
            onclick={() => onopen(task)}
            onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), onopen(task))}
          >
            <title>{task.title}</title>
            <rect width={NODE_W} height={NODE_H} rx="4" />
            <text x="10" y="21" class="t">{clip(task.title)}</text>
            <text x="10" y="40" class="m">{task.done ? '✓ ' : ''}{blocksText(task.blocks)}</text>
          </g>
        {/each}
      </svg>
    </div>
    <p class="legend muted">
      <span class="sw open"></span>{t('game.legendOpen')}
      <span class="sw done"></span>{t('game.legendDone')}
      · {t('game.graphHint')}
    </p>
  {/if}
  {#snippet footer()}
    <Button variant="ghost" onclick={onclose}>{t('common.close')}</Button>
  {/snippet}
</Modal>

<style>
  .state {
    display: grid;
    place-items: center;
    padding: var(--s-6);
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--s-2) var(--s-3);
    margin-bottom: var(--s-3);
  }
  .head span {
    font-size: var(--fs-sm);
  }
  .canvas {
    max-height: 60vh;
    overflow: auto;
    padding: var(--s-2);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
    background: var(--c-surface-2);
  }
  svg {
    display: block;
    margin: 0 auto;
  }
  .edge {
    fill: none;
    stroke: var(--c-border);
    stroke-width: 1.5;
  }
  .node {
    cursor: pointer;
    outline: none;
  }
  .node rect {
    fill: var(--c-surface);
    stroke: var(--c-border);
    stroke-width: 2;
    transition: transform var(--dur-fast) var(--ease-out);
  }
  .node.done rect {
    fill: var(--c-line);
    stroke: var(--c-muted);
  }
  .node:hover rect,
  .node:focus-visible rect {
    stroke: var(--c-focus);
    stroke-width: 3;
  }
  .node:not(.leaf) rect {
    stroke-width: 3;
  }
  .t {
    font-size: 13px;
    font-weight: 700;
    fill: var(--c-text);
  }
  .m {
    font-size: 11px;
    fill: var(--c-muted);
  }
  .node.done .t {
    fill: var(--c-muted);
  }
  .legend {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-1) var(--s-2);
    margin: var(--s-3) 0 0;
    font-size: var(--fs-xs);
  }
  .sw {
    width: 14px;
    height: 10px;
    border: var(--bw) solid var(--c-border);
    border-radius: 2px;
  }
  .sw.open {
    background: var(--c-surface);
  }
  .sw.done {
    background: var(--c-line);
    border-color: var(--c-muted);
  }
  .error {
    color: var(--c-red);
  }
</style>
