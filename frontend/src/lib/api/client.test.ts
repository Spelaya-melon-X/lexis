import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, errorFromResponse, qs } from './client'

describe('errorFromResponse', () => {
  it.each([
    {
      name: 'our error format',
      status: 409,
      body: '{"error":{"code":"duplicate_word","message":"exists"}}',
      want: { status: 409, code: 'duplicate_word', message: 'exists' },
    },
    {
      name: 'non-JSON 502 from Vite proxy → internal',
      status: 502,
      body: 'Bad Gateway',
      want: { status: 502, code: 'internal', message: 'Bad Gateway' },
    },
    {
      name: 'JSON but not our shape → bad_response',
      status: 400,
      body: '{"oops":true}',
      want: { status: 400, code: 'bad_response', message: '{"oops":true}' },
    },
    {
      name: 'error.code is not a string → bad_response',
      status: 400,
      body: '{"error":{"code":42}}',
      want: { status: 400, code: 'bad_response', message: '{"error":{"code":42}}' },
    },
    {
      name: 'empty body',
      status: 404,
      body: '',
      want: { status: 404, code: 'bad_response', message: 'HTTP 404' },
    },
  ])('$name', ({ status, body, want }) => {
    const err = errorFromResponse(status, body)
    expect(err).toBeInstanceOf(ApiError)
    expect({ status: err.status, code: err.code, message: err.message }).toEqual(want)
  })
})

describe('qs', () => {
  it.each([
    { params: {}, want: '' },
    { params: { group: 3 }, want: '?group=3' },
    { params: { group: 3, level: undefined }, want: '?group=3' },
    { params: { day: '', range: null }, want: '' },
    { params: { from: '2026-11-01', to: '2026-11-02' }, want: '?from=2026-11-01&to=2026-11-02' },
    { params: { q: 'a b&c' }, want: '?q=a+b%26c' },
  ])('$params → "$want"', ({ params, want }) => {
    expect(qs(params)).toBe(want)
  })
})


describe('training start contract', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('sends either direction and includes a word selection only when provided', async () => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({sessionId:1,cards:[]}))))
    vi.stubGlobal('fetch',fetch)
    await api.training.start(7,'en_ru')
    expect(JSON.parse(fetch.mock.calls[0]![1].body)).toEqual({groupId:7,strategy:'en_ru',free:false})
    await api.training.start(7,'ru_en',true,[2,4])
    expect(JSON.parse(fetch.mock.calls[1]![1].body)).toEqual({groupId:7,strategy:'ru_en',free:true,wordIds:[2,4]})
    await api.training.start(7,'ru_en',false,[])
    expect(JSON.parse(fetch.mock.calls[2]![1].body).wordIds).toEqual([])
  })
})

describe('study review history contract', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('keeps the same answer ID on retry and sends period selection', async () => {
    const fetch = vi.fn().mockImplementation((url: string) => Promise.resolve(
      url.includes('/reviews') ? new Response(null, {status: 204}) : new Response(JSON.stringify({days: [], total: 0, known: 0, activeDays: 0, bestStreak: 0})),
    ))
    vi.stubGlobal('fetch', fetch)
    const input = {id: 'a1111111-1111-4111-8111-111111111111', cardId: 12, known: true, reverse: false}
    await api.study.review(input)
    await api.study.review(input)
    expect(fetch.mock.calls[0]![0]).toBe('/api/study/reviews')
    expect(JSON.parse(fetch.mock.calls[0]![1].body)).toEqual(input)
    expect(fetch.mock.calls[0]![1].body).toBe(fetch.mock.calls[1]![1].body)
    await api.study.stats(90)
    expect(fetch.mock.calls[2]![0]).toBe('/api/study/stats?days=90')
  })
})
