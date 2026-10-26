import test from 'node:test'
import assert from 'node:assert/strict'
import { consumeFindJump, matchRanges, needleOf, paintFindHighlight, scrollFindRange } from '../src/findHighlight.ts'

test('关键词不区分大小写，多个空白折成一个', () => {
  assert.equal(needleOf('  Hello   World '), 'hello world')
  assert.equal(needleOf('   '), '')
})

test('返回所有不重叠的命中区间', () => {
  assert.deepEqual(matchRanges('Foo foo FOO', 'foo'), [[0, 3], [4, 7], [8, 11]])
  assert.deepEqual(matchRanges('aaaa', 'aa'), [[0, 2], [2, 4]])
})

test('正文空白与原始 UTF-16 区间一致', () => {
  const text = '😀Hello\n\t  World'
  const ranges = matchRanges(text, needleOf('hello world'))
  assert.deepEqual(ranges, [[2, text.length]])
  assert.equal(text.slice(...ranges[0]), 'Hello\n\t  World')
})

test('跨 Markdown 和着色节点构造一个原文 Range', () => {
  const previous = { CSS: globalThis.CSS, Highlight: globalThis.Highlight, document: globalThis.document, NodeFilter: globalThis.NodeFilter }
  const registered = new Map()
  const parent = { closest: selector => selector === '[data-part-index]' ? { getAttribute: () => '7' } : null }
  const nodes = ['Hel', 'lo  ', '\nWorld'].map(nodeValue => ({ nodeValue, parentElement: parent }))
  const block = { hasAttribute: () => false, closest: parent.closest }
  globalThis.CSS = { highlights: registered }
  globalThis.Highlight = class { constructor(...ranges) { this.ranges = ranges } }
  globalThis.NodeFilter = { SHOW_TEXT: 4 }
  globalThis.document = {
    createTreeWalker: () => { let i = 0; return { nextNode: () => nodes[i++] ?? null } },
    createRange: () => ({ setStart(node, offset) { this.start = [node, offset] }, setEnd(node, offset) { this.end = [node, offset] } }),
  }
  try {
    const result = paintFindHighlight({ querySelectorAll: () => [block] }, 'hello world', 7)
    assert.equal(result.count, 1)
    const range = registered.get('ad-find-current').ranges[0]
    assert.equal(result.first, range)
    assert.deepEqual(range.start, [nodes[0], 0])
    assert.deepEqual(range.end, [nodes[2], 6])
  } finally { Object.assign(globalThis, previous) }
})

test('Range 定位先滚代码横纵轴，再按新几何位置滚正文，重画不重复跳', () => {
  const previous = globalThis.getComputedStyle
  globalThis.getComputedStyle = () => ({ overflowX: 'auto', overflowY: 'auto' })
  const body = { contains: () => true, scrollTop: 0, clientTop: 0, clientHeight: 500, getBoundingClientRect: () => ({ top: 0 }) }
  const code = { parentElement: body, scrollTop: 0, scrollLeft: 0, clientTop: 0, clientLeft: 0, clientWidth: 300, clientHeight: 200, scrollWidth: 1400, scrollHeight: 2000, getBoundingClientRect: () => ({ top: 700, left: 0 }) }
  const range = { startContainer: { parentElement: code }, getClientRects: () => [{ left: 1200 - code.scrollLeft, right: 1250 - code.scrollLeft, top: 1700 - code.scrollTop, bottom: 1720 - code.scrollTop, width: 50, height: 20 }] }
  try {
    const intent = { query: 'const value', part: 4 }, holder = { current: intent }
    assert.equal(consumeFindJump(holder, intent, () => scrollFindRange(body, range)), true)
    assert.equal(code.scrollLeft, 950)
    assert.equal(code.scrollTop, 820)
    assert.equal(body.scrollTop, 640)
    assert.equal(consumeFindJump(holder, intent, () => { throw new Error('不应重复执行') }), false)
  } finally { globalThis.getComputedStyle = previous }
})

test('关闭/清空/手动导航/刷新取消意图，旧帧不能消费新跳转，未就绪可等待下一帧', () => {
  const first = { query: 'a', part: 1 }, next = { query: 'b', part: 2 }
  const holder = { current: first }
  assert.equal(consumeFindJump(holder, first, () => false), false)
  assert.equal(holder.current, first)
  holder.current = next
  assert.equal(consumeFindJump(holder, first, () => { throw new Error('旧帧') }), false)
  holder.current = null
  assert.equal(consumeFindJump(holder, next, () => { throw new Error('已取消') }), false)
})

test('长普通段定位具体 Range；内部已可见时不滚代码；过期 Range 不滚动', () => {
  const body = { contains: () => true, scrollTop: 0, clientTop: 0, clientHeight: 400, getBoundingClientRect: () => ({ top: 100 }) }
  const range = { startContainer: { parentElement: body }, getClientRects: () => [{ top: 5100, bottom: 5120, width: 30, height: 20 }] }
  assert.equal(scrollFindRange(body, range), true)
  assert.equal(body.scrollTop, 4810)
  body.contains = () => false
  assert.equal(scrollFindRange(body, range), false)
  assert.equal(body.scrollTop, 4810)
})

test('中文与无命中', () => {
  assert.deepEqual(matchRanges('更新页与搜索，更新', '更新'), [[0, 2], [7, 9]])
  assert.deepEqual(matchRanges('abc', 'x'), [])
  assert.deepEqual(matchRanges('', 'x'), [])
  assert.deepEqual(matchRanges('abc', ''), [])
})

test('小写化改变长度的字符不让区间错位', () => {
  assert.deepEqual(matchRanges('İx abc', needleOf('İX')), [[0, 2]])
  const text = 'İx abc'
  for (const [start, end] of matchRanges(text, 'abc')) assert.equal(text.slice(start, end), 'abc')
})
