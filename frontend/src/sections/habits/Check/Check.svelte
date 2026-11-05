<script lang="ts">
  // «Отметки» (§17.4): все привычки на выбранный день → «Сохранить».
  // #/habits/check[/YYYY-MM-DD], без даты — сегодня. Задним числом — можно, в будущее — нет.
  import { api } from '../../../lib/api/client'
  import type { Checkin, Habit } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import DatePager from '../../../lib/components/DatePager.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { isDay, today } from '../../../lib/dates'
  import { draftOf, sameDraft, toInput, type Draft } from '../../../lib/habits'
  import { errorText, i18n, t, type TKey } from '../../../lib/i18n/i18n.svelte'
  import { reminders } from '../../../lib/state/reminders.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import CheckRow from './CheckRow.svelte'

  const max = today()
  const day = $derived(isDay(route.rest) && route.rest <= max ? route.rest : max)

  let habits = $state<Habit[] | null>(null)
  let drafts = $state<Record<number, Draft>>({})
  let saved = $state<Record<number, Draft>>({}) // как в БД — для «есть изменения»
  let error = $state<string | null>(null)
  let saving = $state(false)
  let pendingDay = $state<string | null>(null) // переход с несохранёнными правками ждёт подтверждения

  const changed = $derived((habits ?? []).filter((h) => drafts[h.id] && saved[h.id] && !sameDraft(drafts[h.id]!, saved[h.id]!)))
  const dirty = $derived(changed.length > 0)

  function apply(hs: Habit[], cs: Checkin[]) {
    const byHabit = new Map(cs.map((c) => [c.habitId, c]))
    // Архивная привычка видна, только если в этот день у неё уже есть отметка.
    habits = hs.filter((h) => !h.archived || byHabit.has(h.id))
    const fresh: Record<number, Draft> = {}
    for (const h of habits) fresh[h.id] = draftOf(byHabit.get(h.id), i18n.locale)
    saved = fresh
    drafts = structuredClone(fresh)
  }

  let seq = 0 // быстрые переключения дат: применяем только последний ответ
  async function load(d: string) {
    const my = ++seq
    habits = null
    error = null
    try {
      const [hs, cs] = await Promise.all([api.habits.list(true), api.habits.checkins(d, d)])
      if (my === seq) apply(hs, cs)
    } catch (e) {
      if (my === seq) error = errorText(e)
    }
  }
  $effect(() => void load(day))

  function go(d: string) {
    if (dirty) {
      pendingDay = d
      return
    }
    route.go('habits', 'check', d === max ? '' : d)
  }

  async function save() {
    if (!habits || !dirty || saving) return
    const body = []
    for (const h of changed) {
      const r = toInput(h, drafts[h.id]!)
      if ('error' in r) {
        toasts.error(`${h.name}: ${t(`errors.${r.error}` as TKey)}`)
        return
      }
      body.push(r)
    }
    saving = true
    const d = day
    try {
      const cs = await api.habits.saveDay(d, body)
      // Ответ — все отметки дня; привычки те же, перечитывать их не нужно.
      if (d === day) apply(habits, cs)
      toasts.success(t('habits.saved'))
      reminders.refresh()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      saving = false
    }
  }

  // Закрыть вкладку с несохранёнными отметками — браузер спросит.
  $effect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  })
</script>

<svelte:window
  onkeydown={(e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 's') {
      e.preventDefault()
      void save()
    }
  }}
/>

<div class="page">
  <header>
    <DatePager {day} {max} onchange={go} />
    <div class="save">
      {#if dirty}<span class="muted hint">{t('habits.unsaved')}</span>{/if}
      <Button variant="primary" icon="check" disabled={!dirty} loading={saving} onclick={save} title="⌘/Ctrl + S">{t('habits.save')}</Button>
    </div>
  </header>

  {#if error && !habits}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(day)}>{t('common.retry')}</Button></div>
  {:else if !habits}
    <div class="state"><Spinner size={28} /></div>
  {:else if habits.length === 0}
    <div class="state">
      <p>{t('habits.empty')}</p>
      <p class="muted">{t('habits.emptyHint')}</p>
      <Button variant="primary" icon="plus" onclick={() => route.go('habits', 'manage')}>{t('habits.goManage')}</Button>
    </div>
  {:else}
    <ul class="rows scroll">
      {#each habits as h (h.id)}
        {#if drafts[h.id]}
          <CheckRow habit={h} bind:draft={drafts[h.id]!} changed={changed.includes(h)} />
        {/if}
      {/each}
    </ul>
  {/if}
</div>

<ConfirmDialog
  open={pendingDay !== null}
  title={t('habits.unsaved')}
  confirmText={t('common.confirm')}
  danger
  onconfirm={() => {
    const d = pendingDay!
    pendingDay = null
    saved = drafts // сбросить «грязность», чтобы go() пропустил
    route.go('habits', 'check', d === max ? '' : d)
  }}
  oncancel={() => (pendingDay = null)}
/>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-height: 0;
    flex: 1;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
    flex-wrap: wrap;
  }
  .save {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  .hint {
    font-size: var(--fs-sm);
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .rows {
    margin: 0;
    padding: 0 var(--s-1) var(--s-4) 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
</style>
