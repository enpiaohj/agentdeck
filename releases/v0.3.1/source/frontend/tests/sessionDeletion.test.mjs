import test from 'node:test'
import assert from 'node:assert/strict'
import { deletionUnavailableReason, explicitDeletionTargets, deletedSessionKeys } from '../src/sessionDeletion.ts'

test('仅本机受支持 Agent 的原生或保留记录可删除，read_only 不替代来源判断', () => {
  for (const agent of ['codex', 'claude', 'claude-desktop']) {
    for (const source of ['local', 'retained']) assert.equal(deletionUnavailableReason([{agent, source, read_only:true}]), '')
  }
  for (const source of ['wsl', 'import', 'gateway', '']) assert.ok(deletionUnavailableReason([{agent:'codex',source}]))
  assert.ok(deletionUnavailableReason([{agent:'unknown',source:'local'}]))
  assert.ok(deletionUnavailableReason([]))
  assert.ok(deletionUnavailableReason([{agent:'codex',source:'local'}, {agent:'claude',source:'import'}]))
})

test('删除目标只取当前可见且明确选中的行，选主会话不会隐式扩展子会话', () => {
  const rows=[{key:'main'}, {key:'child',group_key:'main'}, {key:'other'}]
  assert.deepEqual(explicitDeletionTargets(rows,new Set(['main','hidden'])),[rows[0]])
  assert.deepEqual(explicitDeletionTargets(rows,new Set(['child','main'])),[rows[0],rows[1]])
})

test('删除事件接受直接 key 数组和 Wails 参数包装，拒绝无效 payload', () => {
  assert.deepEqual(deletedSessionKeys(['a','b']),['a','b'])
  assert.deepEqual(deletedSessionKeys([['a','b']]),['a','b'])
  assert.deepEqual(deletedSessionKeys({keys:['a']}),[])
  assert.deepEqual(deletedSessionKeys([null,2]),[])
})
