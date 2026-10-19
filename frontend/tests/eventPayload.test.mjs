import test from 'node:test'
import assert from 'node:assert/strict'
import { indexStatusFromEvent, storageProgressFromEvent } from '../src/eventPayload.ts'

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

test('迁移进度事件：形状与 Go 侧一致，缺失载荷返回 null', () => {
  const progress = { stage: 'copy', done: 1024, total: 4096, files: 12, done_files: 3, message: 'a.json' }
  assert.deepEqual(storageProgressFromEvent({ name: 'storage:progress', data: progress }), progress)
  assert.deepEqual(storageProgressFromEvent({ name: 'storage:progress', data: [progress] }), progress)
  assert.equal(storageProgressFromEvent({ name: 'storage:progress', data: null }), null)
  assert.equal(storageProgressFromEvent(undefined), null)
})
