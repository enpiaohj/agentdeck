import test from 'node:test'
import assert from 'node:assert/strict'
import { groupSessionRows } from '../src/sessionHierarchy.ts'

const main = { key: 'codex:main', role: 'main', group_key: '' }
const children = Array.from({ length: 13 }, (_, index) => ({ key: `codex:child-${index}`, role: 'subagent', group_key: main.key }))
const other = { key: 'codex:other', role: 'main', group_key: '' }

test('同一会话的 13 个子 Agent 默认收起，展开后紧跟主会话，保留源顺序和数据', () => {
  const rows = [children[0], main, ...children.slice(1), other]
  const before = structuredClone(rows)
  const folded = groupSessionRows(rows, new Set())
  assert.deepEqual(folded.visible.map(row => row.key), [main.key, other.key])
  assert.equal(folded.roots.length, 2)
  assert.equal(folded.children.get(main.key).length, 13)
  const expanded = groupSessionRows(rows, new Set([main.key]))
  assert.deepEqual(expanded.visible.map(row => row.key), [main.key, ...children.map(row => row.key), other.key])
  assert.equal(expanded.childKeys.size, 13)
  assert.deepEqual(rows, before)
})

test('收藏或关键词筛选只命中子会话时仍显示，未知关系与其他主会话不误合并', () => {
  const orphan = { key: 'codex:orphan', role: 'subagent', group_key: 'codex:missing' }
  const unknown = { key: 'codex:unknown', role: 'unknown', group_key: main.key }
  const rows = [children[0], other, orphan, unknown]
  assert.deepEqual(groupSessionRows(rows, new Set()).visible, rows)
})

test('无父子关系的旧数据保持兼容，自环或相互引用不隐藏记录', () => {
  const rows = [{ key: 'old' }, { key: 'self', role: 'subagent', group_key: 'self' },
    { key: 'a', role: 'subagent', group_key: 'b' }, { key: 'b', role: 'subagent', group_key: 'a' }]
  assert.deepEqual(groupSessionRows(rows, new Set()).visible, rows)
})
