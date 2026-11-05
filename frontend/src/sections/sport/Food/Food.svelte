<script lang="ts">
  // «Еда» (§18.4, C2): что съел за день и примерные ккал. Подсказки — из ранее введённого (с ккал прошлого раза).
  // Рядом — сожжено на тренировках, без «баланса»: часы дают только активные ккал (C4).
  // Список сохраняется целиком и оптимистично, как «Что классного» в дневнике.
  import { onMount } from 'svelte'
  import { fade } from 'svelte/transition'
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { FoodDay, Meal } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import DatePager from '../../../lib/components/DatePager.svelte'
  import Icon from '../../../lib/components/Icon.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { isDay, today as todayOf } from '../../../lib/dates'
  import { errorText, i18n, t } from '../../../lib/i18n/i18n.svelte'
  import { findMeal, sumKcal } from '../../../lib/sport'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'

  const today = todayOf()
  const day = $derived(isDay(route.rest) && route.rest <= today ? route.rest : today)

  let data = $state<FoodDay | null>(null)
  let error = $state<string | null>(null)
  let suggestions = $state<Meal[]>([])
  let text = $state('')
  let kcal = $state('')
  let textInput: HTMLInputElement | undefined = $state()

  const fetchDay = latest(api.sport.food)

  function load(d: string) {
    error = null
    void fetchDay(d)
      .then((r) => !r.stale && (data = r.value))
      .catch((e: unknown) => (error = errorText(e)))
  }
  $effect(() => {
    data = null
    load(day)
  })

  function loadSuggestions() {
    api.sport.mealSuggestions().then((x) => (suggestions = x)).catch(() => {})
  }
  onMount(loadSuggestions)

  function go(d: string) {
    route.go('sport', 'food', d === today ? '' : d)
  }

  const parseKcal = (s: string): number | null | 'invalid' => {
    const v = s.trim()
    if (v === '') return null
    return /^\d+$/.test(v) && Number(v) <= 10000 ? Number(v) : 'invalid'
  }

  /** Оптимистично: экран сразу, ответ сервера подтверждает; ошибка — тост и перечитывание дня. */
  async function save(meals: Meal[]) {
    if (!data) return
    const d = data.day
    data = { ...data, meals, eatenKcal: sumKcal(meals) }
    try {
      const fresh = await api.sport.saveFood(d, meals)
      if (data?.day === d) data = fresh
      loadSuggestions()
    } catch (e) {
      toasts.error(errorText(e))
      if (data?.day === d) load(d)
    }
  }

  // Выбрал знакомое из подсказок — подставить его ккал, если своё ещё не вписал.
  function onText() {
    const known = findMeal(suggestions, text)
    if (known?.kcal != null && kcal.trim() === '') kcal = String(known.kcal)
  }

  function add(e: SubmitEvent) {
    e.preventDefault()
    if (!data || !text.trim()) return
    const k = parseKcal(kcal)
    if (k === 'invalid') {
      toasts.error(t('errors.invalid_kcal'))
      return
    }
    void save([...data.meals, { text: text.trim(), kcal: k }])
    text = ''
    kcal = ''
    textInput?.focus()
  }

  function edit(i: number, patch: Partial<{ text: string; kcal: string }>) {
    if (!data) return
    const cur = data.meals[i]!
    const next = { ...cur }
    if (patch.text !== undefined) next.text = patch.text.trim()
    if (patch.kcal !== undefined) {
      const k = parseKcal(patch.kcal)
      if (k === 'invalid') {
        toasts.error(t('errors.invalid_kcal'))
        load(data.day) // вернуть поле к сохранённому
        return
      }
      next.kcal = k
    }
    if (next.text === cur.text && next.kcal === cur.kcal) return
    // Пустой текст — удалить пункт (как в «Что классного»).
    void save(next.text ? data.meals.map((m, j) => (j === i ? next : m)) : data.meals.filter((_, j) => j !== i))
  }

  const n = (v: number) => v.toLocaleString(i18n.locale)
</script>

<datalist id="meal-suggestions">{#each suggestions as m (m.text)}<option value={m.text}>{m.kcal !== null ? `${m.kcal} ${t('sport.kcalShort')}` : ''}</option>{/each}</datalist>

<div class="page scroll">
  <DatePager {day} max={today} onchange={go} />

  {#if error && !data}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(day)}>{t('common.retry')}</Button></div>
  {:else if !data}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="totals">
      <div class="tile eaten"><span class="muted">{t('sport.food.eaten')}</span><strong>{n(data.eatenKcal)}</strong><small class="muted">{t('sport.kcalShort')}</small></div>
      <div class="tile burned"><span class="muted">{t('sport.food.burned')}</span><strong>{n(data.burnedKcal)}</strong><small class="muted">{t('sport.kcalShort')}</small></div>
    </div>
    <p class="muted hint">{t('sport.food.hint')}</p>

    <Card>
      <form class="add" onsubmit={add}>
        <input class="input text" list="meal-suggestions" bind:this={textInput} bind:value={text} oninput={onText} maxlength="200"
          placeholder={t('sport.food.placeholder')} aria-label={t('sport.food.what')} />
        <input class="input kcal" inputmode="numeric" bind:value={kcal} placeholder={t('sport.kcalShort')} aria-label={t('sport.food.kcal')} />
        <Button type="submit" variant="primary" icon="plus" disabled={!text.trim()}>{t('sport.food.add')}</Button>
      </form>

      {#if data.meals.length === 0}
        <p class="muted empty">{t('sport.food.empty')}</p>
      {:else}
        <ul>
          {#each data.meals as m, i (`${i}-${m.text}`)}
            <li in:fade={{ duration: 200 }}>
              <input class="inline text" value={m.text} maxlength="200" aria-label={t('sport.food.what')}
                onchange={(e) => edit(i, { text: e.currentTarget.value })} />
              <input class="inline kcal" inputmode="numeric" value={m.kcal ?? ''} placeholder="—" aria-label={t('sport.food.kcal')}
                onchange={(e) => edit(i, { kcal: e.currentTarget.value })} />
              <button class="icon-btn" aria-label={t('common.delete')} title={t('common.delete')} onclick={() => save(data!.meals.filter((_, j) => j !== i))}>
                <Icon name="x" size={16} />
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </Card>
  {/if}
</div>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    max-width: 760px;
    padding: 2px 6px var(--s-4) 2px;
  }
  .state {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
    padding: var(--s-6);
  }
  .totals {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--s-3);
  }
  .tile {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-sm);
  }
  .tile.eaten {
    background: var(--c-amber-soft);
  }
  .tile.burned {
    background: var(--c-green-soft);
  }
  .tile span {
    font-size: var(--fs-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .tile strong {
    font-size: var(--fs-2xl);
    line-height: 1.1;
    font-variant-numeric: tabular-nums;
  }
  .hint {
    margin: 0;
    font-size: var(--fs-sm);
  }
  .add {
    display: flex;
    gap: var(--s-2);
    margin-bottom: var(--s-3);
  }
  .add .text {
    flex: 1;
    min-width: 0;
  }
  .kcal {
    width: 90px;
    text-align: right;
  }
  .empty {
    margin: 0;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-1) 0;
    border-bottom: 1px solid var(--c-line);
  }
  .inline {
    padding: var(--s-1) var(--s-2);
    border: var(--bw) solid transparent;
    border-radius: var(--radius-sm);
    background: none;
    font: inherit;
  }
  .inline:hover,
  .inline:focus {
    border-color: var(--c-line);
    background: var(--c-surface);
    outline: none;
  }
  .inline.text {
    flex: 1;
    min-width: 0;
    font-weight: 600;
  }
  .inline.kcal {
    font-variant-numeric: tabular-nums;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--c-muted);
    cursor: pointer;
  }
  .icon-btn:hover {
    background: var(--c-surface-2);
    color: var(--c-text);
  }
</style>
