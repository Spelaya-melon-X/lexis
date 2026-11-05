<script lang="ts">
  // ⚙ → Данные (§17.10): полный бэкап, восстановление, «Очистить всё».
  // Импорт списков слов (CSV/JSON) — в настройках английского.
  import { api } from '../../lib/api/client'
  import Button from '../../lib/components/Button.svelte'
  import Card from '../../lib/components/Card.svelte'
  import ConfirmDialog from '../../lib/components/ConfirmDialog.svelte'
  import { errorText, i18n, t } from '../../lib/i18n/i18n.svelte'
  import { reminders } from '../../lib/state/reminders.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'

  let fileInput: HTMLInputElement | undefined = $state()
  let pending = $state<File | null>(null) // бэкап, ждущий подтверждения
  let restoring = $state(false)
  let confirmReset = $state(false)
  let resetting = $state(false)

  function clearInput() {
    if (fileInput) fileInput.value = '' // тот же файл можно выбрать снова
  }

  // Бэкап — JSON-объект ({ … }); всё остальное сюда не относится.
  async function onfile() {
    const file = fileInput?.files?.[0]
    if (!file) return
    const head = (await file.slice(0, 64).text()).replace(/^﻿/, '').trimStart()
    if (head.startsWith('{')) {
      pending = file
    } else {
      toasts.error(t('globalSettings.notBackup'))
      clearInput()
    }
  }

  async function restore() {
    if (!pending) return
    restoring = true
    try {
      const r = await api.data.import(pending)
      if (r.kind === 'backup') {
        const c = r.restored
        toasts.success(t('settings.restored', { words: c.words, groups: c.groups, sessions: c.sessions, days: c.days, habits: c.habits }))
        // Язык мог прийти из бэкапа — применяем сразу.
        i18n.locale = (await api.settings.get()).language
        reminders.refresh()
      }
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      restoring = false
      pending = null
      clearInput()
    }
  }

  async function resetAll() {
    resetting = true
    try {
      await api.data.reset('all')
      toasts.success(t('globalSettings.resetAllDone'))
      reminders.refresh()
    } catch (e) {
      toasts.error(errorText(e))
    } finally {
      resetting = false
      confirmReset = false
    }
  }
</script>

<div class="settings scroll">
  <h1>{t('nav.settings.data')}</h1>

  <Card>
    <h3>{t('globalSettings.backup')}</h3>
    <p class="muted hint">{t('globalSettings.backupHint')}</p>
    <a class="link" href={api.data.exportUrl('json')} download>⬇ {t('globalSettings.exportJson')}</a>

    <h4>{t('globalSettings.restore')}</h4>
    <p class="muted hint">{t('globalSettings.restoreHint')}</p>
    <input bind:this={fileInput} class="sr-only" type="file" accept=".json,application/json" onchange={onfile} />
    <Button icon="upload" loading={restoring} onclick={() => fileInput?.click()}>{t('settings.chooseFile')}</Button>
  </Card>

  <Card class="danger-zone">
    <h3>{t('globalSettings.danger')}</h3>
    <p class="muted hint">{t('globalSettings.resetAllHint')}</p>
    <Button variant="danger" icon="trash" onclick={() => (confirmReset = true)}>{t('globalSettings.resetAll')}</Button>
  </Card>
</div>

<ConfirmDialog
  open={pending !== null}
  title={t('settings.restoreTitle')}
  danger
  confirmText={t('common.confirm')}
  loading={restoring}
  onconfirm={restore}
  oncancel={() => {
    pending = null
    clearInput()
  }}
>
  {t('globalSettings.restoreHint')}
</ConfirmDialog>
<ConfirmDialog
  open={confirmReset}
  title={t('globalSettings.resetAllTitle')}
  danger
  confirmText={t('globalSettings.resetAll')}
  loading={resetting}
  onconfirm={resetAll}
  oncancel={() => (confirmReset = false)}
>
  {t('globalSettings.resetAllBody')}
</ConfirmDialog>

<style>
  .settings {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
    max-width: 640px;
    width: 100%;
    margin: 0 auto;
    padding: 2px 6px 6px 2px;
  }
  h1 {
    font-size: var(--fs-xl);
    letter-spacing: -0.02em;
  }
  h3 {
    font-size: var(--fs-md);
    margin-bottom: var(--s-2);
  }
  h4 {
    font-size: var(--fs-sm);
    margin: var(--s-4) 0 var(--s-1);
  }
  .hint {
    font-size: var(--fs-sm);
    margin-bottom: var(--s-2);
  }
  .link {
    font-weight: 600;
    color: var(--c-text);
  }
  :global(.danger-zone) {
    border-color: var(--c-red) !important;
  }
</style>
