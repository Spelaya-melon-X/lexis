import { describe, expect, it } from 'vitest'
import { ApiError, errorFromResponse, qs } from './client'

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
