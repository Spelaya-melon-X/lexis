/**
 * latest оборачивает async-функцию так, что результат получает только последний вызов:
 * ответы на устаревшие запросы (пользователь уже напечатал дальше) отбрасываются.
 * Без этого медленный ответ на старый текст мог прийти после нового и «откатить» превью.
 *
 * Отброшенный вызов возвращает { stale: true }; ошибки последнего вызова пробрасываются.
 */
export function latest<A extends unknown[], R>(fn: (...args: A) => Promise<R>) {
  let seq = 0
  const call = async (...args: A): Promise<{ stale: false; value: R } | { stale: true }> => {
    const my = ++seq
    try {
      const value = await fn(...args)
      return my === seq ? { stale: false, value } : { stale: true }
    } catch (e) {
      if (my !== seq) return { stale: true }
      throw e
    }
  }
  /** cancel делает устаревшими все вызовы в полёте (поле очистили — старый ответ не нужен). */
  return Object.assign(call, { cancel: () => void seq++ })
}
