import test from 'node:test'
import assert from 'node:assert/strict'
import { indexStatusFromEvent } from '../src/eventPayload.ts'

const status = { state: 'ready', sessions: 155, docs: 45584, total: 155 }

test('单参数事件：载荷是对象本身（Wails 不包数组）', () => {
  assert.deepEqual(indexStatusFromEvent({ name: 'index:ready', data: status }), status)
})

test('多参数事件：载荷是数组，取第一个', () => {
  assert.deepEqual(indexStatusFromEvent({ name: 'index:ready', data: [status] }), status)
})

test('没有载荷或载荷为空时返回 null，不抛异常', () => {
  assert.equal(indexStatusFromEvent({ name: 'index:progress', data: null }), null)
  assert.equal(indexStatusFromEvent({ name: 'index:progress' }), null)
  assert.equal(indexStatusFromEvent(null), null)
  assert.equal(indexStatusFromEvent(undefined), null)
  assert.equal(indexStatusFromEvent({ data: [] }), null)
})
