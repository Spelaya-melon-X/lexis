// @vitest-environment jsdom
import { flushSync, mount, unmount } from 'svelte'
import { afterEach, expect, it, vi } from 'vitest'
import { api } from '../../../lib/api/client'
import type { TrainingCard, TrainingDirection } from '../../../lib/api/types'
import { session } from '../../../lib/state/session.svelte'
import Session from './Session.svelte'
import WordCard from './WordCard.svelte'

const card: TrainingCard = {
  wordId: 1, term: 'go_went_gone', forms: ['go', 'went', 'gone'], translation: 'идти', level: 2,
  examples: [{ text: 'I [went] home.', sentence: 'I went home.', highlights: [{ start: 2, end: 6 }] }],
}
let view: ReturnType<typeof mount> | undefined

afterEach(async () => {
  if (view) await unmount(view)
  vi.restoreAllMocks()
})

function visibleFace(word: TrainingCard, revealed: boolean, strategy: TrainingDirection = 'en_ru') {
  view = mount(WordCard, { target: document.body, props: { card: word, revealed, strategy } })
  flushSync()
  return document.querySelector<HTMLElement>('.face[aria-hidden="false"]')!
}

it('exposes only the term and makes the answer face inert before reveal', () => {
  const front = visibleFace(card, false)
  expect(front.textContent?.trim()).toBe('go – went – gone')
  expect(document.querySelector<HTMLElement>('.back')?.inert).toBe(true)
})

it('shows only the translation on the front in Russian to English mode', () => {
  const front = visibleFace(card, false, 'ru_en')
  expect(front.textContent?.trim()).toBe('идти')
  expect(front.querySelector('h2')?.lang).toBe('ru')
})

it('shows the term, highlighted example and translation in order on the answer face', () => {
  const back = visibleFace(card, true)
  expect([...back.children].map((element) => element.textContent?.trim()))
    .toEqual(['go – went – gone', 'I went home.', 'идти'])
  expect(back.querySelector('mark')?.textContent).toBe('went')
  expect(document.activeElement).toBe(back.querySelector('h2'))
})

it('shows the word and translation when it has no examples', () => {
  const back = visibleFace({ wordId: 2, term: 'plain', translation: 'простой', level: 1 }, true)
  expect([...back.children].map((element) => element.textContent?.trim())).toEqual(['plain', 'простой'])
})

it('focuses the answer input for each card, reveals on Enter and accepts a keyboard rating', async () => {
  const answer = vi.spyOn(api.training, 'answer').mockResolvedValue({
    levelBefore: 2, levelAfter: 2, firstInSession: true,
  })
  session.begin({ sessionId: 12, groupId: 1, groupName: 'Verbs', strategy: 'ru_en', free: false }, [
    card, { wordId: 2, term: 'plain', translation: 'простой', level: 1 },
  ])
  view = mount(Session, { target: document.body, props: { active: session.active! } })
  flushSync()
  const input = document.querySelector<HTMLInputElement>('.answer-input')!
  expect(document.activeElement).toBe(input)
  expect(document.querySelector('.front[aria-hidden="false"]')?.textContent?.trim()).toBe('идти')
  input.value = 'go'
  input.dispatchEvent(new Event('input', { bubbles: true }))
  input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  flushSync()
  expect(document.querySelector('.back')?.getAttribute('aria-hidden')).toBe('false')
  window.dispatchEvent(new KeyboardEvent('keydown', { key: '1' }))
  await vi.waitFor(() => {
    expect(document.querySelector('.front[aria-hidden="false"]')?.textContent?.trim()).toBe('простой')
    expect(document.activeElement).toBe(input)
    expect(input.value).toBe('')
  })
  expect(answer).toHaveBeenCalledExactlyOnceWith(12, 1, 'good')
  session.reset()
})
