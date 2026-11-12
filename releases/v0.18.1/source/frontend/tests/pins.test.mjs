import assert from 'node:assert/strict'
import test from 'node:test'
import { isPinned, parsePins, pinLimit, pinnedFirst, removePins, togglePins } from '../src/pins.ts'

test('批量置顶：全部已置顶才取消，否则补齐', () => {
  assert.deepEqual(togglePins([], ['a', 'b']), ['a', 'b'])
  assert.deepEqual(togglePins(['a'], ['a', 'b']), ['a', 'b'])
  assert.deepEqual(togglePins(['a', 'b', 'c'], ['a', 'b']), ['c'])
  assert.deepEqual(togglePins(['a'], []), ['a'])
})

test('项目路径大小写不敏感', () => {
  assert.equal(isPinned(['D:\\Work\\Alpha'], 'd:\\work\\alpha', true), true)
  assert.equal(isPinned(['D:\\Work\\Alpha'], 'd:\\work\\alpha'), false)
  assert.deepEqual(togglePins(['D:\\Work\\Alpha'], ['d:\\work\\alpha'], true), [])
})

test('置顶的排在前面，各自保持原有顺序', () => {
  const items = ['a', 'b', 'c', 'd']
  assert.deepEqual(pinnedFirst(items, x => x, ['d', 'b']), ['b', 'd', 'a', 'c'])
  assert.deepEqual(pinnedFirst(items, x => x, []), items)
  assert.deepEqual(pinnedFirst(['D:\\A', 'D:\\B'], x => x, ['d:\\b'], true), ['D:\\B', 'D:\\A'])
})

test('删除会话时移除置顶', () => {
  assert.deepEqual(removePins(['a', 'b', 'c'], ['b']), ['a', 'c'])
})

test('解析偏好：只留非空字符串、去重、限量', () => {
  assert.deepEqual(parsePins(['a', ' a ', '', 3, null, 'b']), ['a', 'b'])
  assert.deepEqual(parsePins('x'), [])
  const many = Array.from({ length: pinLimit + 5 }, (_, i) => `k${i}`)
  assert.equal(parsePins(many).length, pinLimit)
  assert.equal(parsePins(many)[0], 'k5', '超出上限时保留最近加入的')
})
