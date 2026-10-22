import assert from 'node:assert/strict'
import test from 'node:test'
import { inverseAction, markDoneMessage, markPartialMessage, undoFailureMessage } from '../src/markFeedback.ts'

test('每个操作的反向操作互为逆', () => {
  for (const a of ['favorite', 'unfavorite', 'archive', 'unarchive']) assert.equal(inverseAction(inverseAction(a)), a)
  assert.equal(inverseAction('archive'), 'unarchive')
})

test('提示文案', () => {
  assert.equal(markDoneMessage('archive', 3), '已归档 3 个会话')
  assert.equal(markDoneMessage('unarchive', 1), '已取消归档 1 个会话')
  assert.equal(markDoneMessage('favorite', 2), '已收藏 2 个会话')
  assert.equal(markPartialMessage('archive', 2, 1), '已归档 2 个会话，另有 1 个未完成')
})

test('撤销全部成功时不再提示', () => {
  assert.equal(undoFailureMessage(0), '')
  assert.match(undoFailureMessage(2), /2 个会话没有恢复/)
})
