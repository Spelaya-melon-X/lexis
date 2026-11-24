import { describe, expect, it } from 'vitest'
import { taskTags } from './taskTags'

describe('taskTags', () => {
  it.each([
    ['Разобрать конспект #учёба #go-backend', ['учёба', 'go-backend']],
    ['#Go #go (#GO), #ВШЭ_2 #вшэ_2 #2026', ['go', 'вшэ_2', '2026']],
    ['#первый\n#второй', ['первый', 'второй']],
    ['# ##два хеш#внутри https://example.com/#section', []],
    ['[link](https://example.com/?q=(#fragment)) www.example.com/#section #Go', ['go']],
    ['Без тегов', []],
  ])('extracts tags from %j', (note, tags) => {
    expect(taskTags(note)).toEqual(tags)
  })
})
