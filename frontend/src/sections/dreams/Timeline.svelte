<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { api } from '../../lib/api/client'
  import type { TimelineGoal, TimelineInput } from '../../lib/api/types'
  import { emptyAmbitions } from '../../lib/ambitions'
  import { goalsFor, timelineYears } from '../../lib/timeline'
  import { t, errorText, i18n } from '../../lib/i18n/i18n.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'
  import Button from '../../lib/components/Button.svelte'
  import Spinner from '../../lib/components/Spinner.svelte'
  import Modal from '../../lib/components/Modal.svelte'
  import ConfirmDialog from '../../lib/components/ConfirmDialog.svelte'
  let data = $state(emptyAmbitions()),
    loading = $state(true),
    busy = $state(false),
    failure = $state('')
  let year = $state(0),
    month = $state<number | null>(null)
  let editor = $state<TimelineInput | null>(null),
    editing = $state<number | null>(null),
    deleting = $state<number | null>(null)
  let rail = $state<HTMLElement>()
  $effect(() => {
    const selected = year
    const element = rail
    if (!element) return
    void tick().then(() => {
      if (year === selected)
        element
          .querySelector(`[data-year="${selected}"]`)
          ?.scrollIntoView({ block: 'nearest', inline: 'center' })
    })
  })
  const goals = $derived(data.timeline?.goals ?? [])
  const years = $derived(
    timelineYears(data.today, data.timeline?.years ?? [], goals),
  )
  const current = $derived(Number(data.today.slice(0, 4)))
  const annual = $derived(goalsFor(goals, year, null)),
    monthly = $derived(goalsFor(goals, year, month))
  const months = $derived(
    Array.from({ length: 12 }, (_, i) =>
      new Intl.DateTimeFormat(i18n.locale, {
        month: 'long',
        timeZone: 'UTC',
      }).format(new Date(Date.UTC(2000, i, 1))),
    ),
  )
  async function load() {
    loading = true
    failure = ''
    try {
      data = await api.ambitions.get()
      year = Number(data.today.slice(0, 4))
    } catch (e) {
      failure = errorText(e)
    } finally {
      loading = false
    }
  }
  onMount(load)
  async function run(action: () => ReturnType<typeof api.ambitions.get>) {
    if (busy) return false
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
  function open(g?: TimelineGoal) {
    editing = g?.id ?? null
    editor = g
      ? {
          year: g.year,
          month: g.month,
          title: g.title,
          note: g.note,
          dreamId: g.dreamId,
        }
      : { year, month, title: '', note: '', dreamId: null }
  }
  async function save() {
    const v = editor
    if (!v) return
    if (
      await run(() =>
        editing === null
          ? api.timeline.create(v)
          : api.timeline.update(editing, v),
      )
    )
      editor = null
  }
  async function nextYear() {
    const y = Math.max(...years) + 1
    if (y > 9999) return
    if (await run(() => api.timeline.addYear(y))) {
      year = y
      month = null
    }
  }
</script>

<div class="page">
  <div class="head">
    <div>
      <h1>{t('timeline.title')}</h1>
      <p class="muted">{t('timeline.subtitle')}</p>
    </div>
    <Button icon="plus" disabled={loading || busy} onclick={() => open()}
      >{t('timeline.add')}</Button
    >
  </div>
  {#if loading}<Spinner />{:else if failure}<p role="alert">{failure}</p>
    <Button onclick={load}>{t('common.retry')}</Button>{:else}
    <div class="tools">
      <Button
        disabled={busy}
        onclick={() => {
          year = current
          month = null
        }}>{t('timeline.current')}</Button
      ><Button disabled={busy || Math.max(...years) >= 9999} onclick={nextYear}
        >{t('timeline.addYear')}</Button
      >
    </div>
    <nav bind:this={rail} class="years" aria-label={t('timeline.years')}>
      {#each years as y}<button
          data-year={y}
          class:selected={year === y}
          class:current={current === y}
          aria-current={current === y ? 'date' : undefined}
          aria-pressed={year === y}
          onclick={() => {
            year = y
            month = null
          }}
          >{y}<small>{goals.filter((g) => g.year === y).length}</small></button
        >{/each}
    </nav>
    <div class="period">
      <h2>
        {year}{#if year === current}
          <small>{t('timeline.now')}</small>{/if}
      </h2>
      <Button
        disabled={busy}
        onclick={() => {
          month = null
          open()
        }}>{t('timeline.addAnnual')}</Button
      >
    </div>
    <section aria-label={t('timeline.annual')}>
      <h3>{t('timeline.annual')}</h3>
      {#if !annual.length}<p class="empty">{t('timeline.emptyYear')}</p>{/if}
      {#each annual as g (g.id)}{@render goal(g)}{/each}
    </section>
    <nav class="months" aria-label={t('timeline.months')}>
      {#each months as name, i}<button
          class:selected={month === i + 1}
          aria-pressed={month === i + 1}
          onclick={() => (month = i + 1)}
          >{name}<small>{goalsFor(goals, year, i + 1).length}</small></button
        >{/each}
    </nav>
    {#if month !== null}<section>
        <div class="period">
          <h2>{months[month - 1]}</h2>
          <Button disabled={busy} onclick={() => open()}
            >{t('timeline.addMonthly')}</Button
          >
        </div>
        {#if !monthly.length}<p class="empty">
            {t('timeline.emptyMonth')}
          </p>{/if}
        {#each monthly as g (g.id)}{@render goal(g)}{/each}
      </section>{:else}<p class="muted">{t('timeline.chooseMonth')}</p>{/if}
  {/if}
</div>
{#snippet goal(g: TimelineGoal)}
  {@const dream = data.dreams.find((d) => d.id === g.dreamId)}
  {@const project = data.projects.find((p) => p.id === dream?.projectId)}
  <article>
    <div class="content">
      <small class="tag"
        >{g.month === null
          ? t('timeline.annual')
          : t('timeline.monthly')}</small
      >
      <h3>{dream?.title ?? g.title}</h3>
      {#if g.note}<p class="note">{g.note}</p>{/if}
      {#if dream}<a href="#/dreams/list"
          >{t('timeline.linked')} · {t(`ambitions.status.${dream.status}`)}</a
        >{/if}
      {#if project}<a href={'#/game/projects/' + project.id}
          >{project.title} · {data.summaries[project.id]?.progress ?? 0}%</a
        >{/if}
    </div>
    <Button size="sm" disabled={busy} onclick={() => open(g)}
      >{t('common.edit')}</Button
    ><Button size="sm" disabled={busy} onclick={() => (deleting = g.id)}
      >{t('common.delete')}</Button
    >
  </article>
{/snippet}
{#if editor}<Modal
    open
    title={editing === null ? t('timeline.add') : t('timeline.edit')}
    onclose={() => {
      if (!busy) editor = null
    }}
  >
    <form
      class="form"
      onsubmit={(e) => {
        e.preventDefault()
        save()
      }}
    >
      <label
        >{t('timeline.dream')}<select
          class="input"
          bind:value={editor.dreamId}
          disabled={busy}
          ><option value={null}>{t('timeline.independent')}</option
          >{#each data.dreams as d}<option value={d.id}>{d.title}</option
            >{/each}</select
        ></label
      >
      {#if editor.dreamId === null}<label
          >{t('timeline.goalTitle')}<input
            class="input"
            bind:value={editor.title}
            required
            maxlength="200"
            disabled={busy}
          /></label
        >{/if}
      <label
        >{t('timeline.note')}<textarea
          class="input"
          bind:value={editor.note}
          maxlength="20000"
          disabled={busy}></textarea></label
      >
      <label
        >{t('timeline.year')}<input
          class="input"
          type="number"
          min="1"
          max="9999"
          step="1"
          bind:value={editor.year}
          required
          disabled={busy}
        /></label
      >
      <label
        >{t('timeline.period')}<select
          class="input"
          bind:value={editor.month}
          disabled={busy}
          ><option value={null}>{t('timeline.annual')}</option
          >{#each months as name, i}<option value={i + 1}>{name}</option
            >{/each}</select
        ></label
      >
      <p class="muted">{t('timeline.linkHelp')}</p>
      <Button type="submit" loading={busy} variant="primary"
        >{t('common.save')}</Button
      >
    </form></Modal
  >{/if}
<ConfirmDialog
  open={deleting !== null}
  title={t('common.delete')}
  loading={busy}
  danger
  oncancel={() => (deleting = null)}
  onconfirm={async () => {
    const id = deleting
    if (id !== null && (await run(() => api.timeline.remove(id))))
      deleting = null
  }}><p>{t('timeline.deleteHelp')}</p></ConfirmDialog
>

<style>
  .page {
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--s-5);
    padding-bottom: var(--s-7);
  }
  .head,
  .period,
  .tools {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3);
    flex-wrap: wrap;
  }
  .head h1 {
    font-size: var(--fs-xl);
  }
  .head p {
    margin-top: var(--s-2);
  }
  .tools {
    justify-content: flex-start;
  }
  .years {
    display: flex;
    overflow-x: auto;
    flex-shrink: 0;
    border-bottom: var(--bw-strong) solid var(--c-border);
    padding-bottom: var(--s-3);
    gap: var(--s-3);
  }
  button {
    cursor: pointer;
  }
  .years button,
  .months button {
    background: var(--c-surface);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
    padding: var(--s-3);
    display: flex;
    gap: var(--s-2);
    align-items: center;
  }
  .years button {
    min-width: 90px;
    justify-content: center;
  }
  .years button.current {
    border-color: var(--c-green);
  }
  button.selected {
    background: var(--c-green-soft);
    border-color: var(--c-border);
    font-weight: 700;
  }
  .months {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
    gap: var(--s-2);
  }
  .months button {
    justify-content: space-between;
  }
  .empty {
    padding: var(--s-5);
    color: var(--c-muted);
    border: var(--bw) dashed var(--c-line);
    margin-top: var(--s-3);
  }
  article {
    display: flex;
    gap: var(--s-2);
    align-items: flex-start;
    padding: var(--s-4);
    border: var(--bw) solid var(--c-line);
    border-radius: var(--radius-sm);
    margin-top: var(--s-3);
    flex-wrap: wrap;
  }
  .content {
    flex: 1;
    min-width: min(220px, 100%);
    overflow-wrap: anywhere;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .note {
    white-space: pre-wrap;
  }
  .tag {
    color: var(--c-muted);
  }
  a {
    color: var(--c-text);
  }
  .form,
  .form label {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  .form {
    gap: var(--s-4);
  }
</style>
