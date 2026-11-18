import assert from 'node:assert/strict'
import test from 'node:test'
import { applyMarkPatch, eventKey, inMarkScope } from '../src/markPatch.ts'

const row = (key, over = {}) => ({ key, favorite: false, archived: false, tags: [], note: '', title: key, groupKey: 'g', ...over })

test('只替换标注字段，其余字段保留', () => {
  const rows = [row('a'), row('b')]
  const next = applyMarkPatch(rows, [{ key: 'b', favorite: true, archived: false, tags: ['x'], note: 'n', title: '新名' }], {})
  assert.equal(next[1].favorite, true)
  assert.equal(next[1].title, '新名')
  assert.equal(next[1].groupKey, 'g')
  assert.equal(next[0], rows[0], '未变的行保持同一引用，避免无谓重渲染')
})

test('变得不在当前范围的行被移除', () => {
  const rows = [row('a', { favorite: true }), row('b', { favorite: true })]
  const next = applyMarkPatch(rows, [{ key: 'a', favorite: false, archived: false, tags: [], note: '', title: 'a' }], { favorites: true })
  assert.deepEqual(next.map(r => r.key), ['b'])
  const archived = applyMarkPatch([row('a')], [{ key: 'a', favorite: false, archived: true, tags: [], note: '', title: 'a' }], {})
  assert.deepEqual(archived, [], '默认视图不含归档会话')
})

test('列表里没有、却新进入当前范围的行：要求整表重载', () => {
  const fresh = [{ key: 'z', favorite: true, archived: false, tags: [], note: '', title: 'z' }]
  assert.equal(applyMarkPatch([row('a')], fresh, { favorites: true }), null)
  assert.deepEqual(applyMarkPatch([row('a')], fresh, { archived: true }), [row('a')], '不在范围内的陌生行直接忽略')
})

test('范围判定与后端一致', () => {
  assert.equal(inMarkScope({ favorite: false, archived: true }, { archived: true }), true)
  assert.equal(inMarkScope({ favorite: false, archived: false }, { archived: true }), false)
  assert.equal(inMarkScope({ favorite: false, archived: false }, { favorites: true }), false)
})

test('事件载荷取第一个字符串 key', () => {
  assert.equal(eventKey('claude:1'), 'claude:1')
  assert.equal(eventKey(['claude:1']), 'claude:1')
  assert.equal(eventKey(''), '')
  assert.equal(eventKey(undefined), '')
})
