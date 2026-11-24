// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, beforeAll, expect, it, vi } from 'vitest'
import { api } from '../../lib/api/client'
import type { GrammarRule } from '../../lib/api/types'
import { t, type TKey } from '../../lib/i18n/i18n.svelte'
import CardEditor from './CardEditor.svelte'
import BulkEditor from './BulkEditor.svelte'
import GrammarReader from './GrammarReader.svelte'

const rule: GrammarRule = {
  id: 2, groupId: 1, kind: 'grammar_rule', front: 'Should + verb', back: '**Advice**\n\n- Use the base form.',
  content: {
    formula: 'Subject + should + base verb',
    examples: [{ text: 'You should study.', translation: 'Тебе следует заниматься.' }],
    mistakes: [{ incorrect: 'You should to study.', correct: 'You should study.' }],
  },
}

let view: ReturnType<typeof mount> | undefined

beforeAll(() => {
  // jsdom implements the dialog element without the browser modality methods.
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value(this: HTMLDialogElement) { this.open = true },
  })
})

afterEach(async () => {
  if (view) await unmount(view)
  vi.restoreAllMocks()
})

function button(key: TKey) {
  return Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === t(key))!
}

it('shows the complete rule immediately and pages with buttons and arrow keys', () => {
  const close = vi.fn()
  const next: GrammarRule = {
    ...rule, id: 3, front: 'Although', back: 'Expresses contrast.', content: { formula: '' },
  }
  view = mount(GrammarReader, { target: document.body, props: { rules: [rule, next], initialIndex: 0, onclose: close } })
  flushSync()

  const article = document.querySelector('article')!
  expect(article.querySelector('h1')?.textContent).toBe(rule.front)
  expect(article.querySelector('li')?.textContent).toBe('Use the base form.')
  for (const text of ['Subject + should + base verb', 'Тебе следует заниматься.', 'You should to study.']) {
    expect(article.textContent).toContain(text)
  }
  expect(button('grammar.previous').getAttribute('aria-disabled')).toBe('true')
  button('grammar.next').click()
  flushSync()
  expect(document.querySelector('h1')?.textContent).toBe('Although')
  expect(button('grammar.next').getAttribute('aria-disabled')).toBe('true')
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft' }))
  flushSync()
  expect(document.querySelector('h1')?.textContent).toBe(rule.front)
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  expect(close).toHaveBeenCalledOnce()
})

it('adds structured example and mistake rows and keeps the editor draft separate', () => {
  const save = vi.fn()
  view = mount(CardEditor, {
    target: document.body,
    props: {
      initial: rule, groups: [{ id: 1, title: 'Modal verbs', parentId: null, grammar: true }],
      busy: false, onsave: save, onclose: vi.fn(),
    },
  })
  flushSync()
  button('grammar.addExample').click()
  button('grammar.addMistake').click()
  flushSync()

  const fields = document.querySelectorAll<HTMLTextAreaElement>('fieldset textarea')
  expect(fields).toHaveLength(8)
  for (const [field, value] of [
    [fields[2]!, 'We should leave.'],
    [fields[3]!, 'Нам следует уйти.'],
    [fields[6]!, 'We should leaves.'],
    [fields[7]!, 'We should leave.'],
  ] as const) {
    field.value = value
    field.dispatchEvent(new Event('input', { bubbles: true }))
  }
  flushSync()
  document.querySelector('form')!.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }))
  expect(save).toHaveBeenCalledWith({
    ...rule,
    content: {
      formula: rule.content.formula,
      examples: [rule.content.examples![0], { text: 'We should leave.', translation: 'Нам следует уйти.' }],
      mistakes: [rule.content.mistakes![0], { incorrect: 'We should leaves.', correct: 'We should leave.' }],
    },
  })
  expect(rule.content.examples).toHaveLength(1)
  expect(rule.content.mistakes).toHaveLength(1)
})

it('previews a batch and saves it to the chosen grammar topic', async () => {
  const groups = [
    { id: 1, title: 'Modal verbs', parentId: null, grammar: true },
    { id: 8, title: 'Exam rules', parentId: null, grammar: true },
  ]
  const another: GrammarRule = { ...rule, id: 3, front: 'Although' }
  const source = t('grammar.bulkExample')
  const preview = vi.spyOn(api.study, 'parse').mockResolvedValue({
    cards: [{ ...rule, record: 1 }, { ...another, record: 2 }], errors: [],
  })
  const saved = { groups, cards: [rule, another] }
  const bulk = vi.spyOn(api.study, 'bulk').mockResolvedValue(saved)
  const onsave = vi.fn()
  view = mount(BulkEditor, {
    target: document.body, props: { groupId: 1, groups, grammar: true, onsave, onclose: vi.fn() },
  })
  flushSync()
  const input = document.querySelector('textarea')!
  input.value = source
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await vi.waitFor(() => expect(document.querySelectorAll('article h4')).toHaveLength(2))
  expect(preview).toHaveBeenCalledExactlyOnceWith(source, true)

  const topic = document.querySelector('select')!
  topic.value = '8'
  topic.dispatchEvent(new Event('change', { bubbles: true }))
  document.querySelector<HTMLButtonElement>('.actions button')!.click()
  await vi.waitFor(() => expect(onsave).toHaveBeenCalledExactlyOnceWith(saved, 8))
  expect(bulk).toHaveBeenCalledExactlyOnceWith(8, source)
})
