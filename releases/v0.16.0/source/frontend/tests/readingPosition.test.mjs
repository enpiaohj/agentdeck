import test from 'node:test'
import assert from 'node:assert/strict'
import { canReusePrompt, readingRevealPart, refreshPosition } from '../src/readingPosition.ts'

test('恢复时展开阅读锚点，不将搜索焦点作为恢复目标；段中部提问必须重新载入开头', () => {
 assert.equal(readingRevealPart(36, 12), 36)
 assert.equal(readingRevealPart(undefined, 12), 12)
 assert.equal(readingRevealPart(0, -1), 0)
 assert.equal(canReusePrompt(6000), false)
 assert.equal(canReusePrompt(undefined), false)
 assert.equal(canReusePrompt(0), true)
})
test('同轮全局跳转使旧 displayedDestination 失效，刷新不能覆盖目标偏移', () => {
 const oldDestination = { at: 3 }, target = { at: 40 }
 let displayedDestination = oldDestination
 const oldPosition = { at: 3, before: false, scroll: 821, anchor: 3 }
 displayedDestination = null
 assert.equal(refreshPosition(3, 4, displayedDestination === oldDestination, oldPosition), null)
 assert.equal(refreshPosition(3, 4, displayedDestination === target, oldPosition), null)
})
test('同一会话刷新保留原始段锚点、像素偏移和滚动位置', () => {
 const position = { at: -1, before: false, scroll: 821, anchor: 36, offset: -27 }
 const saved = refreshPosition(3, 4, true, position)
 assert.deepEqual(saved, position)
 position.scroll = 0
 assert.equal(saved.scroll, 821)
})
test('显式搜索导航优先，初始与普通重绘不触发刷新恢复', () => {
 const position = { at: 0, before: false, scroll: 123 }
 assert.equal(refreshPosition(3, 4, false, position), null)
 assert.equal(refreshPosition(3, 3, true, position), null)
})
