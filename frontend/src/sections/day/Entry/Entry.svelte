<script lang="ts">
  // Страница дня (§17.2): #/day/entry[/YYYY-MM-DD], без даты — сегодня.
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Day, DayPatch } from '../../../lib/api/types'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import DatePager from '../../../lib/components/DatePager.svelte'
  import RatingPicker from '../../../lib/components/RatingPicker.svelte'
  import Spinner from '../../../lib/components/Spinner.svelte'
  import { isDay, today } from '../../../lib/dates'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { reminders } from '../../../lib/state/reminders.svelte'
  import { route } from '../../../lib/state/route.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'
  import DopamineScale from './DopamineScale.svelte'
  import HabitsBlock from './HabitsBlock.svelte'
  import Story from './Story.svelte'
  import WinsList from './WinsList.svelte'

  const max = today() // страницу не держат открытой сутками; после полуночи хватит перезагрузки
  // Мусор и будущие даты в адресе — показываем сегодня (писать в будущее бэк всё равно не даст).
  const day = $derived(isDay(route.rest) && route.rest <= max ? route.rest : max)

  let data = $state<Day | null>(null)
  let error = $state<string | null>(null)
  const fetchDay = latest(api.journal.day)

  function load(d: string) {
    error = null
    void fetchDay(d)
      .then((r) => !r.stale && (data = r.value))
      .catch((e) => (error = errorText(e)))
  }

  $effect(() => {
    data = null // не показывать прошлый день под новой датой, пока грузится
    load(day)
  })

  function go(d: string) {
    route.go('day', 'entry', d === max ? '' : d)
  }

  /**
   * Оптимистичная запись: интерфейс меняется сразу, ответ сервера его подтверждает.
   * Ошибка — тост и перечитывание дня с сервера (вернуть как было).
   * story ответа на экран не попадает — им владеет Story (см. там).
   */
  async function save(optimistic: (d: Day) => void, request: (d: string) => Promise<Day>) {
    if (!data) return
    const d = data.day
    optimistic(data)
    try {
      const fresh = await request(d)
      if (data?.day === d) data = { ...fresh, story: data.story }
      reminders.refresh() // оценка могла снять точку «день не оценён»
    } catch (e) {
      toasts.error(errorText(e))
      if (data?.day === d) load(d)
    }
  }

  const patch = (p: DayPatch, optimistic: (d: Day) => void) => save(optimistic, (d) => api.journal.update(d, p))

  async function setDopamine(slot: number, text: string) {
    await save(
      (d) => {
        d.dopamine[slot] = text || null
        d.dopamineLevel = d.dopamine.filter((s) => s !== null).length
      },
      (d) => api.journal.dopamine(d, slot, text),
    )
  }
</script>

<div class="page">
  <DatePager {day} {max} onchange={go} />

  {#if error && !data}
    <div class="state"><p>{error}</p><Button icon="refresh" onclick={() => load(day)}>{t('common.retry')}</Button></div>
  {:else if !data}
    <div class="state"><Spinner size={28} /></div>
  {:else}
    <div class="grid scroll">
      <!-- Две независимые колонки: высота карточки слева не растягивает соседнюю справа -->
      <div class="col">
        <Card>
          <h2>{t('journal.rating')}</h2>
          <p class="muted sub">{t('journal.ratingHint')}</p>
          <RatingPicker value={data.rating} label={t('journal.rating')} onchange={(v) => patch({ rating: v }, (d) => (d.rating = v))} />
        </Card>

        <Card>
          <h2>{t('journal.wins')}</h2>
          <WinsList wins={data.wins} onchange={(w) => patch({ wins: w }, (d) => (d.wins = w))} />
        </Card>
      </div>

      <div class="col">
        <Card>
          <h2>
            {t('journal.dopamine')}
            <span class="level" class:max={data.dopamineLevel === 7}>{t('journal.dopamineLevel', { level: data.dopamineLevel })}</span>
          </h2>
          <DopamineScale slots={data.dopamine} onsave={setDopamine} />
        </Card>

        <HabitsBlock day={data.day} isToday={data.day === max} />
      </div>

      <Card class="story">
        <h2>{t('journal.story')}</h2>
        <!-- новый день — новый Story: старый при уничтожении дописывает своё -->
        {#key data.day}
          <Story day={data.day} initial={data.story} />
        {/key}
      </Card>
    </div>
  {/if}
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-height: 0;
    flex: 1;
  }
  .state {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--s-3);
  }
  .grid {
    display: grid;
    /* minmax(0, …): иначе длинный контент распирает колонку шире половины */
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    align-items: start;
    gap: var(--s-4);
    padding: 0 var(--s-1) var(--s-4) 0; /* место под тень карточек у края скролла */
  }
  .col {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    min-width: 0;
  }
  .grid :global(.story) {
    grid-column: 1 / -1;
  }
  h2 {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s-2);
    margin-bottom: var(--s-3);
    font-size: var(--fs-md);
  }
  .sub {
    margin: calc(-1 * var(--s-2)) 0 var(--s-3);
    font-size: var(--fs-sm);
  }
  .level {
    font-size: var(--fs-sm);
    font-weight: 600;
    color: var(--c-muted);
  }
  .level.max {
    color: var(--c-green);
  }

  @media (max-width: 960px) {
    .grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
