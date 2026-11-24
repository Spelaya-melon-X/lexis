import type { Example, Span } from './api/types'

export interface Segment {
  text: string
  highlight: boolean
}

/**
 * Режет предложение на куски по выделениям. Смещения с бэкенда — в рунах (code points),
 * а строки JS — UTF-16: "🍎".length === 2. Поэтому режем по Array.from (по code points),
 * иначе эмодзи или редкий иероглиф в предложении сдвинул бы все выделения.
 * Кривые спаны (вне строки, пересекающиеся) обрезаются/пропускаются — показать текст важнее.
 */
export function segments(sentence: string, spans: Span[] = []): Segment[] {
  const chars = Array.from(sentence)
  const out: Segment[] = []
  let pos = 0
  for (const s of [...spans].sort((a, b) => a.start - b.start)) {
    const start = Math.max(s.start, pos)
    const end = Math.min(s.end, chars.length)
    if (start >= end) continue
    if (start > pos) out.push({ text: chars.slice(pos, start).join(''), highlight: false })
    out.push({ text: chars.slice(start, end).join(''), highlight: true })
    pos = end
  }
  if (pos < chars.length) out.push({ text: chars.slice(pos).join(''), highlight: false })
  return out
}

/** "go_went_gone" → "go – went – gone" (§7: так показываем 3 формы). */
export function displayTerm(term: string, forms?: string[]): string {
  return forms && forms.length === 3 ? forms.join(' – ') : term
}

/** Сохраняем ручную разметку; для обычного текста ищем слово или форму целиком. */
export function exampleHighlights(example: Example, term: string, forms: string[] = []): Span[] {
  if (example.highlights?.length) return example.highlights
  const escaped = [...new Set([term, ...forms])].filter(Boolean)
    .sort((a, b) => b.length - a.length)
    .map((form) => form.replace(/[.*+?^{}()|[\]\\]/g, '\\$&'))
  if (!escaped.length) return []
  const pattern = new RegExp('(?<![\\p{L}\\p{M}\\p{N}])(?:' + escaped.join('|') + ')(?![\\p{L}\\p{M}\\p{N}])', 'giu')
  return Array.from(example.sentence.matchAll(pattern), (match) => {
    const start = Array.from(example.sentence.slice(0, match.index)).length
    return { start, end: start + Array.from(match[0]).length }
  })
}
