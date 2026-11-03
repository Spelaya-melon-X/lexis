// Работа со строками textarea по номерам строк парсера (1-based, как LineError.line).

/** Разбивает как бэкенд (strings.Split по "\n"); \r остаётся в строке — бэк его тоже обрезает trim-ом. */
function split(text: string): string[] {
  return text.split('\n')
}

/** Оставляет только строки с указанными номерами (после сохранения — исправлять ошибочные). */
export function keepLines(text: string, lines: number[]): string {
  const want = new Set(lines)
  return split(text)
    .filter((_, i) => want.has(i + 1))
    .join('\n')
}

/** Смещения [start, end) строки line в text — для выделения в textarea. null — нет такой строки. */
export function lineRange(text: string, line: number): [number, number] | null {
  const all = split(text)
  if (line < 1 || line > all.length) return null
  let start = 0
  for (let i = 0; i < line - 1; i++) start += all[i]!.length + 1 // +1 за "\n"
  return [start, start + all[line - 1]!.length]
}
