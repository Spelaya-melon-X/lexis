<script lang="ts">
  import { onMount } from 'svelte'
  import { api } from '../../../lib/api/client'
  import Button from '../../../lib/components/Button.svelte'
  import Card from '../../../lib/components/Card.svelte'
  import ConfirmDialog from '../../../lib/components/ConfirmDialog.svelte'
  import { errorText, t } from '../../../lib/i18n/i18n.svelte'
  import { toasts } from '../../../lib/state/toasts.svelte'

  let limit = $state(10)
  let savingLimit = $state(false)
  let fileInput: HTMLInputElement | undefined = $state()
  let importing = $state(false)
  let confirmReset = $state(false)
  let resetting = $state(false)

  async function loadSettings() {
    try {
      const s = await api.settings.get()
      limit = s.newWordsLimit
    } catch (e) {
      toasts.error(errorText(e))
    }
  }
  onMount(loadSettings)

  async function saveLimit(e: SubmitEvent) {
    e.preventDefault()
    savingLimit = true
    try {
      limit = (await api.settings.update({ newWordsLimit: Number(limit) })).newWordsLimit
      toasts.success(t('settings.saved'))
    } catch (err) {
      toasts.error(errorText(err))
    } finally {
      savingLimit = false
    }
  }

  // Здесь — только списки слов (CSV / JSON-массив). Бэкап (JSON-объект) восстанавливается
  // в ⚙ → Данные: так из настроек английского нельзя случайно заменить вообще все данные.
  async function onfile() {
    const file = fileInput?.files?.[0]
    if (!file) return
    const head = (await file.slice(0, 64).text()).replace(/^\uFEFF/, '').trimStart()
    if (head.startsWith('{')) {
      toasts.error(t('settings.isBackup'))
      if (fileInput) fileInput.value = ''
      return
    }
    await upload(file)
  }

  async function upload(file: File) {
    importing = true
    try {
      const r = await api.data.import(file)
      if (r.kind === 'words') {
        toasts.success(t('settings.imported', { created: r.created ?? 0, merged: r.merged ?? 0 }))
        if (r.errors?.length) toasts.info(t('settings.importErrors', { count: r.errors.length }))
      }
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      importing = false
      if (fileInput) fileInput.value = '' // тот же файл можно выбрать снова
    }
  }

  async function reset() {
    resetting = true
    try {
      await api.data.reset('english')
      toasts.success(t('settings.resetDone'))
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      resetting = false
      confirmReset = false
    }
  }
</script>

<div class="settings scroll">
  <Card>
    <h3>{t('settings.limit')}</h3>
    <form class="inline" onsubmit={saveLimit}>
      <input class="input num" type="number" min="0" max="100" required bind:value={limit} aria-label={t('settings.limit')} />
      <Button type="submit" variant="primary" loading={savingLimit}>{t('common.save')}</Button>
    </form>
    <p class="muted hint">{t('settings.limitHint')}</p>
  </Card>

  <Card>
    <h3>{t('settings.data')}</h3>
    <h4>{t('settings.import')}</h4>
    <p class="muted hint">{t('settings.importHint')}</p>
    <input bind:this={fileInput} class="sr-only" type="file" accept=".csv,.json,text/csv,application/json" onchange={onfile} />
    <Button icon="upload" loading={importing} onclick={() => fileInput?.click()}>{t('settings.chooseFile')}</Button>
    <h4>{t('settings.export')}</h4>
    <div class="inline">
        <a class="link" href={api.data.exportUrl('csv')} download>⬇ {t('settings.exportCsv')}</a>
    </div>
    <p class="muted hint"><a class="link" href="#/settings/data">⚙ {t('settings.backupMoved')} →</a></p>
  </Card>

  <Card class="danger-zone">
    <h3>{t('settings.danger')}</h3>
    <p class="muted hint">{t('settings.resetHint')}</p>
    <Button variant="danger" icon="trash" onclick={() => (confirmReset = true)}>{t('settings.reset')}</Button>
  </Card>
</div>

<ConfirmDialog open={confirmReset} title={t('settings.resetTitle')} danger confirmText={t('settings.reset')} loading={resetting}
  onconfirm={reset} oncancel={() => (confirmReset = false)}>
  {t('settings.resetBody')}
</ConfirmDialog>

<style>
  .settings {
    height: 100%;
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    max-width: 640px;
    width: 100%;
    margin: 0 auto;
    padding: 2px 6px 6px 2px;
  }
  h3 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-3);
  }
  h4 {
    font-size: var(--fs-sm);
    margin: var(--s-3) 0 var(--s-1);
  }
  .hint {
    font-size: var(--fs-sm);
    margin: var(--s-2) 0;
  }
  .inline {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s-3);
  }
  .num {
    width: 100px;
  }
  .link {
    font-weight: 600;
    color: var(--c-text);
  }
  :global(.danger-zone) {
    border-color: var(--c-red) !important;
  }
</style>
