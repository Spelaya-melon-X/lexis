<script lang="ts">
  // ⚙ → Общие (§17.1): настройки всего приложения. Пока — язык интерфейса.
  import { api } from '../../lib/api/client'
  import type { Locale } from '../../lib/api/types'
  import Card from '../../lib/components/Card.svelte'
  import Segmented from '../../lib/components/Segmented.svelte'
  import { errorText, i18n, t } from '../../lib/i18n/i18n.svelte'
  import { toasts } from '../../lib/state/toasts.svelte'

  async function setLocale(l: Locale) {
    const prev = i18n.locale
    i18n.locale = l // сразу — интерфейс не ждёт сеть
    try {
      await api.settings.update({ language: l })
    } catch (e) {
      i18n.locale = prev
      toasts.error(errorText(e))
    }
  }
</script>

<div class="settings scroll">
  <h1>{t('nav.settings.general')}</h1>
  <Card>
    <h3>{t('globalSettings.language')}</h3>
    <Segmented
      label={t('globalSettings.language')}
      value={i18n.locale}
      onchange={setLocale}
      options={[
        { value: 'ru', label: 'Русский' },
        { value: 'en', label: 'English' },
      ]}
    />
    <p class="muted hint">{t('globalSettings.languageHint')}</p>
  </Card>
</div>

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
    margin-bottom: var(--s-3);
  }
  .hint {
    font-size: var(--fs-sm);
    margin-top: var(--s-2);
  }
</style>
