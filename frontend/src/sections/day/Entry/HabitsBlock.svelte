<script lang="ts">
  // Блок «Привычки» на странице дня (Q9): что отмечено, без редактирования — ссылка на «Отметки».
  import { latest } from '../../../lib/async'
  import { api } from '../../../lib/api/client'
  import type { Checkin, Habit } from '../../../lib/api/types'
  import Card from '../../../lib/components/Card.svelte'
  import { formatAmount } from '../../../lib/habits'
  import { i18n, t } from '../../../lib/i18n/i18n.svelte'

  let { day, isToday }: { day: string; isToday: boolean } = $props()

  let data = $state<{ habits: Habit[]; checkins: Checkin[] } | null>(null)
  const fetchDay = latest((d: string) => Promise.all([api.habits.list(true), api.habits.checkins(d, d)]))

  $effect(() => {
    void fetchDay(day)
      .then((r) => !r.stale && (data = { habits: r.value[0], checkins: r.value[1] }))
      .catch(() => (data = null)) // блок вспомогательный: ошибка — просто не показываем его
  })

  const byId = $derived(new Map((data?.habits ?? []).map((h) => [h.id, h])))
  const rows = $derived((data?.checkins ?? []).flatMap((c) => (byId.has(c.habitId) ? [{ c, h: byId.get(c.habitId)! }] : [])))
  const hasHabits = $derived((data?.habits ?? []).some((h) => !h.archived) || rows.length > 0)
</script>

{#if data && hasHabits}
  <Card class="habits">
    <h2>
      {t('habits.dayBlock')}
      <a class="edit" href="#/habits/check{isToday ? '' : `/${day}`}">{t('habits.dayBlockEdit')} →</a>
    </h2>
    {#if rows.length === 0}
      <p class="muted empty">{t('habits.dayBlockEmpty')}</p>
    {:else}
      <ul>
        {#each rows as { c, h } (h.id)}
          <li class:slip={h.kind === 'negative'}>
            <span class="dot" style:background={h.color ?? 'transparent'}></span>
            <span class="name">{h.name}</span>
            <span class="what">{t(h.kind === 'negative' ? 'habits.slip' : 'habits.done')}</span>
            <span class="muted vals">
              {#if c.amount !== null}{formatAmount(c.amount, i18n.locale)} {h.unit}{/if}
              {#if c.rating !== null}★{c.rating}{/if}
              {#if c.trigger}«{c.trigger}»{/if}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </Card>
{/if}

<style>
  h2 {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--s-2);
    margin-bottom: var(--s-3);
    font-size: var(--fs-md);
  }
  .edit {
    font-size: var(--fs-sm);
    font-weight: 600;
    color: var(--c-muted);
  }
  .edit:hover {
    color: var(--c-text);
  }
  .empty {
    font-size: var(--fs-sm);
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  li {
    display: flex;
    align-items: baseline;
    gap: var(--s-2);
    font-size: var(--fs-sm);
  }
  .dot {
    width: 10px;
    height: 10px;
    flex: none;
    align-self: center;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .name {
    font-weight: 700;
  }
  .what {
    color: var(--c-green);
    font-weight: 600;
  }
  .slip .what {
    color: var(--c-red);
  }
  .vals {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
</style>
