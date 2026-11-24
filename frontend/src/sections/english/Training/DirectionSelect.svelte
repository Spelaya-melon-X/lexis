<script lang="ts">
  import type { TrainingDirection } from '../../../lib/api/types'
  import { t } from '../../../lib/i18n/i18n.svelte'

  let { value = $bindable(), disabled }: { value: TrainingDirection; disabled: boolean } = $props()
</script>

<label>
  <span>{t('training.direction')}</span>
  <select class="input" bind:value {disabled}>
    <option value="en_ru">{t('training.directions.en_ru')}</option>
    <option value="ru_en">{t('training.directions.ru_en')}</option>
  </select>
</label>

<style>
  label {
    display: grid;
    gap: var(--s-1);
    max-width: 350px;
    font-weight: 600;
  }
</style>
