// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { renderMarkdown } from './markdown'
function dom(text: string): HTMLDivElement {
  const el = document.createElement('div')
  el.innerHTML = renderMarkdown(text)
  return el
}
describe('safe study Markdown', () => {
  it('renders Markdown, tables, breaks and math on either side', () => {
    const el = dom(
      '# Definition\n\n**Norm** $\\lVert x \\rVert$\nnext line\n\n$$\\sum_{i=1}^n x_i$$\n\n| A | B |\n|---|---|\n| 1 | 2 |',
    )
    expect(el.querySelector('h1')?.textContent).toBe('Definition')
    expect(el.querySelector('strong')?.textContent).toBe('Norm')
    expect(el.querySelectorAll('.katex')).toHaveLength(2)
    expect(el.querySelector('math')).not.toBeNull()
    expect(el.querySelector('table')).not.toBeNull()
    expect(el.querySelector('br')).not.toBeNull()
  })
  it('does not execute HTML or allow unsafe URLs', () => {
    const el = dom(
      '<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n[x](javascript:alert%281%29)\n\n[y](https://example.com)\n\n![z](javascript:alert%281%29)',
    )
    expect(el.querySelector('script')).toBeNull()
    expect(el.querySelector('[onerror]')).toBeNull()
    expect(el.querySelector('[href^="javascript:"]')).toBeNull()
    expect(el.querySelector('[src^="javascript:"]')).toBeNull()
    expect(el.querySelector('a[href="https://example.com"]')).not.toBeNull()
    expect(el.textContent).toContain('<script>')
  })
  it('disables trusted TeX commands and macro loops', () => {
    const el = dom(
      '$\\href{javascript:alert(1)}{click}$\n\n$\\htmlClass{evil}{x}$\n\n$\\def\\a{\\a}\\a$',
    )
    expect(el.querySelector('a')).toBeNull()
    expect(el.querySelector('.evil')).toBeNull()
    expect(el.querySelector('.katex-error')).not.toBeNull()
  })
  it('leaves math and markers inside code unchanged', () => {
    const el = dom('`$x$`\n\n```text\n$$y$$\n---\n[type: theorem]\n```')
    expect(el.querySelector('.katex')).toBeNull()
    expect(el.querySelector('pre')?.textContent).toContain('[type: theorem]')
  })
  it('supports bracket delimiters and does not share macros across cards', () => {
    expect(
      dom('\\(x^2\\)\n\n\\[\\frac{1}{2}\\]').querySelectorAll('.katex'),
    ).toHaveLength(2)
    dom('$\\gdef\\userMacro{x}\\userMacro$')
    expect(dom('$\\userMacro$').textContent).toContain('\\userMacro')
  })
})
