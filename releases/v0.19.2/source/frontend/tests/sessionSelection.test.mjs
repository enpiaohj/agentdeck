import test from 'node:test'
import assert from 'node:assert/strict'
import { selectSession, pruneSelection } from '../src/sessionSelection.ts'

const order = ['a', 'b', 'c', 'd', 'e']
const empty = () => ({ keys: new Set(), anchor: null, focus: null })
const keys = state => [...state.keys].sort()

test('单击只选择目标，并记录范围锚点，不修改旧集合', () => {
  const old = { keys: new Set(['a', 'b']), anchor: 'a', focus: 'b' }
  const next = selectSession(old, order, 'd')
  assert.deepEqual(keys(next), ['d'])
  assert.equal(next.anchor, 'd')
  assert.equal(next.focus, 'd')
  assert.deepEqual(keys(old), ['a', 'b'])
})

test('Ctrl 切换目标，不影响其他项，并可取消最后一项', () => {
  let state = selectSession(empty(), order, 'a')
  state = selectSession(state, order, 'c', { toggle: true })
  assert.deepEqual(keys(state), ['a', 'c'])
  state = selectSession(state, order, 'a', { toggle: true })
  assert.deepEqual(keys(state), ['c'])
  state = selectSession(state, order, 'c', { toggle: true })
  assert.equal(state.keys.size, 0)
})

test('Shift 以固定锚点按当前显示顺序扩展、缩小或反向连选', () => {
  let state = selectSession(empty(), order, 'c')
  state = selectSession(state, order, 'e', { range: true })
  assert.deepEqual(keys(state), ['c', 'd', 'e'])
  state = selectSession(state, order, 'd', { range: true })
  assert.deepEqual(keys(state), ['c', 'd'])
  state = selectSession(state, order, 'a', { range: true })
  assert.deepEqual(keys(state), ['a', 'b', 'c'])
  assert.equal(state.anchor, 'c')
})

test('Ctrl+Shift 将连续区间加入原选择，排序和筛选决定范围', () => {
  const state = { keys: new Set(['e', 'b']), anchor: 'b', focus: 'b' }
  const next = selectSession(state, ['e', 'd', 'c', 'b', 'a'], 'd', { toggle: true, range: true })
  assert.deepEqual(keys(next), ['b', 'c', 'd', 'e'])
  assert.deepEqual(keys(selectSession(state, ['b', 'd'], 'd', { range: true })), ['b', 'd'])
})

test('刷新移除隐藏选择和失效锚点；没有锚点时 Shift 从目标开始', () => {
  const state = { keys: new Set(['a', 'c']), anchor: 'a', focus: 'a' }
  const next = pruneSelection(state, ['b', 'c'])
  assert.deepEqual(keys(next), ['c'])
  assert.equal(next.anchor, null)
  assert.equal(next.focus, 'c')
  assert.deepEqual(keys(selectSession(empty(), order, 'd', { range: true })), ['d'])
  assert.deepEqual(keys(selectSession(state, ['c'], 'missing')), ['c'])
})
