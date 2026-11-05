<script lang="ts">
  // Строка формы «Отметки»: отметить + включённые у привычки поля.
  import { slide } from 'svelte/transition'
  import type { Habit } from '../../../lib/api/types'
  import Icon from '../../../lib/components/Icon.svelte'
  import RatingPicker from '../../../lib/components/RatingPicker.svelte'
  import type { Draft } from '../../../lib/habits'
  import { t } from '../../../lib/i18n/i18n.svelte'

  interface Props {
    habit: Habit
    draft: Draft
    changed: boolean
  }
  let { habit, draft = $bindable(), changed }: Props = $props()

  const negative = $derived(habit.kind === 'negative')
  const hasFields = $derived(habit.trackTrigger || habit.trackRating || habit.trackAmount)
</script>

<li class:done={draft.done} class:negative class:changed>
  <div class="head">
    <span class="dot" style:background={habit.color ?? 'transparent'}></span>
    <span class="name">{habit.name}</span>
    <button class="toggle" aria-pressed={draft.done} onclick={() => (draft.done = !draft.done)}>
      <span class="box">{#if draft.done}<Icon name={negative ? 'x' : 'check'} size={16} />{/if}</span>
      {t(negative ? 'habits.slip' : 'habits.done')}
    </button>
  </div>

  {#if draft.done && hasFields}
    <div class="fields" transition:slide={{ duration: 200 }}>
      {#if habit.trackAmount}
        <label class="amount">
          <span>{t('habits.amount')}</span>
          <span class="with-unit">
            <input class="input" inputmode="decimal" bind:value={draft.amount} placeholder="0" />
            {#if habit.unit}<span class="muted">{habit.unit}</span>{/if}
          </span>
        </label>
      {/if}
      {#if habit.trackTrigger}
        <label class="trigger">
          <span>{t('habits.trigger')}</span>
          <input class="input" bind:value={draft.trigger} maxlength="200" placeholder={t('habits.triggerPlaceholder')} />
        </label>
      {/if}
      {#if habit.trackRating}
        <div class="rating">
          <span>{t('habits.rating')}</span>
          <RatingPicker size="sm" value={draft.rating} label={`${habit.name}: ${t('habits.rating')}`} onchange={(v) => (draft.rating = v)} />
        </div>
      {/if}
    </div>
  {/if}
</li>

<style>
  li {
    padding: var(--s-3) var(--s-4);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius);
    background: var(--c-surface);
    box-shadow: var(--shadow-sm);
    transition:
      background var(--dur) var(--ease-out),
      border-color var(--dur) var(--ease-out);
  }
  li.done {
    background: var(--c-green-soft);
  }
  li.done.negative {
    background: var(--c-red-soft);
  }
  /* Несохранённая правка — пунктирная рамка: видно, что «Сохранить» её заберёт */
  li.changed {
    border-style: dashed;
  }
  .head {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  .dot {
    width: 14px;
    height: 14px;
    flex: none;
    border: var(--bw) solid var(--c-border);
    border-radius: 50%;
  }
  .name {
    flex: 1;
    font-weight: 700;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-1) var(--s-3);
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
    font-weight: 600;
    cursor: pointer;
    transition:
      transform var(--dur-fast) var(--ease-out),
      box-shadow var(--dur-fast) var(--ease-out);
  }
  .toggle:hover {
    transform: translate(-1px, -1px);
    box-shadow: var(--shadow-sm);
  }
  .box {
    display: grid;
    place-items: center;
    width: 20px;
    height: 20px;
    border: var(--bw) solid var(--c-border);
    border-radius: var(--radius-sm);
    background: var(--c-surface);
  }
  .done .box {
    background: var(--c-green);
    color: var(--c-bg);
  }
  .done.negative .box {
    background: var(--c-red);
  }
  .fields {
    display: grid;
    grid-template-columns: 160px 1fr;
    gap: var(--s-3);
    margin-top: var(--s-3);
  }
  .fields label,
  .rating {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    font-size: var(--fs-sm);
    font-weight: 600;
  }
  .rating {
    grid-column: 1 / -1;
  }
  .trigger {
    grid-column: 2;
  }
  .amount:only-child,
  .trigger:only-child {
    grid-column: 1 / -1;
  }
  .with-unit {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-weight: 400;
  }
  .with-unit .input {
    width: 100px;
  }
  @media (max-width: 860px) {
    .fields {
      grid-template-columns: 1fr;
    }
    .trigger {
      grid-column: 1;
    }
  }
</style>
