// 会话内查找的正文高亮：只负责「文本里哪几段是关键词」和「把它们交给浏览器的
// CSS Custom Highlight API」，不改 React 渲染出来的 DOM，所以不影响选中、复制与折叠。

// 与后端 sessions.NeedleOf 同一口径：空白折成一个空格，不区分大小写。
const spaces = /\p{White_Space}+/u
const lower = (value: string) => Array.from(value, char => char === 'İ' ? 'i' : char.toLowerCase()).join('')
export const needleOf = (query: string) => lower(query.split(spaces).filter(Boolean).join(' '))

// matchRanges 返回 text 里所有不重叠的命中区间 [start, end)。
// 折叠空白并保留 UTF-16 原文映射，跨节点 Range 也使用原始偏移。
export function matchRanges(text: string, needle: string): [number, number][] {
  if (!needle || !text) return []
  let haystack = ''
  const starts: number[] = [], ends: number[] = []
  let offset = 0
  for (const char of text) {
    const start = offset
    offset += char.length
    const folded = spaces.test(char) ? ' ' : lower(char)
    if (folded === ' ' && haystack.endsWith(' ')) { ends[ends.length - 1] = offset; continue }
    haystack += folded
    for (let i = 0; i < folded.length; i++) { starts.push(start); ends.push(offset) }
  }
  const out: [number, number][] = []
  for (let from = 0; from <= haystack.length - needle.length;) {
    const at = haystack.indexOf(needle, from)
    if (at < 0) break
    out.push([starts[at], ends[at + needle.length - 1]])
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
// 用更醒目的「当前」样式，同时返回当前段的首个命中供一次性定位。
export function paintFindHighlight(root: HTMLElement, query: string, currentPart: number): { count: number; first: Range | null } {
  clearFindHighlight()
  const reg = registry()
  const needle = needleOf(query)
  if (!reg || !needle) return { count: 0, first: null }
  const HighlightCtor = (globalThis as unknown as { Highlight: new (...ranges: Range[]) => unknown }).Highlight
  const others: Range[] = []
  const current: Range[] = []
  // 在同一正文块内连接文本节点；代码行间补换行，排除按钮与行号。
  const blocks = root.querySelectorAll<HTMLElement>('.ad-markdown, [data-find-code]')
  for (const block of blocks) {
    if (!block.hasAttribute('data-find-code') && block.closest('[data-find-code]')) continue
    const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT)
    const nodes: { node: Node; start: number; end: number }[] = []
    let text = '', line: Element | null = null
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const parent = node.parentElement
      if (!parent || parent.closest('script, style, button, [data-find-ignore]')) continue
      if (!block.hasAttribute('data-find-code') && parent.closest('[data-find-code]')) continue
      const nextLine = parent.closest('[data-code-line], p, li, h1, h2, h3, h4, blockquote')
      if (nodes.length && nextLine !== line) text += '\n'
      line = nextLine
      const start = text.length
      text += node.nodeValue ?? ''
      nodes.push({ node, start, end: text.length })
    }
    const isCurrent = block.closest('[data-part-index]')?.getAttribute('data-part-index') === String(currentPart)
    for (const [start, end] of matchRanges(text, needle)) {
      if (others.length + current.length >= LIMIT) break
      const first = nodes.find(item => item.end > start)
      const last = nodes.find(item => item.end >= end)
      if (!first || !last) continue
      const range = document.createRange()
      range.setStart(first.node, start - first.start)
      range.setEnd(last.node, end - last.start)
      ;(isCurrent ? current : others).push(range)
    }
  }
  if (others.length) reg.set(MATCH, new HighlightCtor(...others))
  if (current.length) reg.set(CURRENT, new HighlightCtor(...current))
  return { count: others.length + current.length, first: current[0] ?? null }
}

// 先滚动命中所在的内部代码/表格容器，再用更新后的几何位置滚正文。
export function scrollFindRange(body: HTMLElement, range: Range): boolean {
  if (!body.contains(range.startContainer)) return false
  const rect = () => range.getClientRects()[0] ?? range.getBoundingClientRect()
  const delta = (start: number, end: number, low: number, high: number) =>
    start < low ? start - low : end > high ? Math.min(start - low, end - high) : 0
  let parent = range.startContainer.parentElement
  for (; parent && parent !== body; parent = parent.parentElement) {
    const style = getComputedStyle(parent)
    const box = parent.getBoundingClientRect()
    const hit = rect()
    if (/(auto|scroll)/.test(style.overflowX) && parent.scrollWidth > parent.clientWidth)
      parent.scrollLeft += delta(hit.left, hit.right, box.left + parent.clientLeft, box.left + parent.clientLeft + parent.clientWidth)
    if (/(auto|scroll)/.test(style.overflowY) && parent.scrollHeight > parent.clientHeight)
      parent.scrollTop += delta(hit.top, hit.bottom, box.top + parent.clientTop, box.top + parent.clientTop + parent.clientHeight)
  }
  const hit = rect(), box = body.getBoundingClientRect()
  if (!hit.width && !hit.height) return false
  body.scrollTop += (hit.top + hit.bottom) / 2 - (box.top + body.clientTop + body.clientHeight / 2)
  return true
}

// 对象身份让排队中的旧帧无法消费新意图；成功消费后重画也不会再次跳转。
export type FindJump = { query: string; part: number }
export function consumeFindJump(holder: { current: FindJump | null }, expected: FindJump, action: () => boolean): boolean {
  if (holder.current !== expected || !action()) return false
  holder.current = null
  return true
}
