// 会话内查找的正文高亮：只负责「文本里哪几段是关键词」和「把它们交给浏览器的
// CSS Custom Highlight API」，不改 React 渲染出来的 DOM，所以不影响选中、复制与折叠。

// 与后端 sessions.NeedleOf 同一口径：空白折成一个空格，不区分大小写。
export const needleOf = (query: string) => query.trim().split(/\s+/).filter(Boolean).join(' ').toLowerCase()

// matchRanges 返回 text 里所有不重叠的命中区间 [start, end)。
// 小写化会改变长度的字符（如带点的 İ）直接放弃折叠，宁可漏高亮也不让区间错位。
export function matchRanges(text: string, needle: string): [number, number][] {
  if (!needle || !text) return []
  const lowered = text.toLowerCase()
  const haystack = lowered.length === text.length ? lowered : text
  const out: [number, number][] = []
  for (let from = 0; from <= haystack.length - needle.length;) {
    const at = haystack.indexOf(needle, from)
    if (at < 0) break
    out.push([at, at + needle.length])
    from = at + needle.length
  }
  return out
}

type HighlightRegistry = { set(name: string, value: unknown): void; delete(name: string): void }
const registry = (): HighlightRegistry | null => {
  const css = (globalThis as { CSS?: { highlights?: HighlightRegistry } }).CSS
  return css?.highlights && typeof (globalThis as { Highlight?: unknown }).Highlight === 'function' ? css.highlights : null
}

export const MATCH = 'ad-find-match'
export const CURRENT = 'ad-find-current'
// 一次最多标记这么多处：超长工具输出里的高频词不值得把页面拖慢。
const LIMIT = 1500

export function clearFindHighlight() {
  const reg = registry()
  reg?.delete(MATCH)
  reg?.delete(CURRENT)
}

// paintFindHighlight 在 root 里标出所有命中；落在 currentPart（data-part-index）内的
// 用更醒目的「当前」样式。返回标出的数量。
export function paintFindHighlight(root: HTMLElement, query: string, currentPart: number): number {
  clearFindHighlight()
  const reg = registry()
  const needle = needleOf(query)
  if (!reg || !needle) return 0
  const HighlightCtor = (globalThis as unknown as { Highlight: new (...ranges: Range[]) => unknown }).Highlight
  const others: Range[] = []
  const current: Range[] = []
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  for (let node = walker.nextNode(); node && others.length + current.length < LIMIT; node = walker.nextNode()) {
    const parent = node.parentElement
    if (!parent || parent.closest('script, style')) continue
    const ranges = matchRanges(node.nodeValue ?? '', needle)
    if (!ranges.length) continue
    const isCurrent = currentPart >= 0 && parent.closest('[data-part-index]')?.getAttribute('data-part-index') === String(currentPart)
    for (const [start, end] of ranges) {
      const range = document.createRange()
      range.setStart(node, start)
      range.setEnd(node, end)
      ;(isCurrent ? current : others).push(range)
    }
  }
  if (others.length) reg.set(MATCH, new HighlightCtor(...others))
  if (current.length) reg.set(CURRENT, new HighlightCtor(...current))
  return others.length + current.length
}
