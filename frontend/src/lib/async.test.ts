import { describe, expect, it } from 'vitest'
import { latest } from './async'

// Управляемый промис: тест сам решает, в каком порядке «приходят ответы».
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('latest', () => {
  it('drops a slow older response that arrives after a newer one', async () => {
    const calls: ReturnType<typeof deferred<string>>[] = []
    const fn = latest((_text: string) => {
      const d = deferred<string>()
      calls.push(d)
      return d.promise
    })

    const first = fn('ap')
    const second = fn('apple')
    calls[1]!.resolve('apple-result') // новый ответ пришёл первым
    calls[0]!.resolve('ap-result') // старый — позже

    expect(await second).toEqual({ stale: false, value: 'apple-result' })
    expect(await first).toEqual({ stale: true })
  })

  it('in-order responses: only the last is fresh', async () => {
    const fn = latest(async (n: number) => n * 2)
    const a = fn(1)
    const b = fn(2)
    expect(await a).toEqual({ stale: true })
    expect(await b).toEqual({ stale: false, value: 4 })
  })

  it('error of the latest call is thrown, of a stale one — swallowed', async () => {
    const calls: ReturnType<typeof deferred<number>>[] = []
    const fn = latest(() => {
      const d = deferred<number>()
      calls.push(d)
      return d.promise
    })
    const old = fn()
    const cur = fn()
    calls[0]!.reject(new Error('old failed'))
    calls[1]!.reject(new Error('current failed'))
    expect(await old).toEqual({ stale: true })
    await expect(cur).rejects.toThrow('current failed')
  })

  it('single call is fresh', async () => {
    const fn = latest(async () => 'ok')
    expect(await fn()).toEqual({ stale: false, value: 'ok' })
  })
})

describe('latest.cancel', () => {
  it('makes in-flight calls stale', async () => {
    let resolve!: (v: string) => void
    const fn = latest(() => new Promise<string>((r) => (resolve = r)))
    const pending = fn()
    fn.cancel()
    resolve('late')
    expect(await pending).toEqual({ stale: true })
  })

  it('calls after cancel are fresh again', async () => {
    const fn = latest(async () => 1)
    fn.cancel()
    expect(await fn()).toEqual({ stale: false, value: 1 })
  })
})
