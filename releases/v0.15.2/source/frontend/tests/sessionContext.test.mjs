import test from 'node:test'
import assert from 'node:assert/strict'
import { selectContextSession } from '../src/sessionSelection.ts'

test('右键已选会话保留多选及锚点，仅更新键盘焦点', () => {
  const before={keys:new Set(['a','c']),anchor:'a',focus:'a'}
  const next=selectContextSession(before,['a','b','c'],'c')
  assert.deepEqual([...next.keys],['a','c']);assert.equal(next.anchor,'a');assert.equal(next.focus,'c')
  assert.equal(before.focus,'a')
})
test('右键未选会话只选择目标，隐藏与失效会话不进入菜单操作范围', () => {
  const before={keys:new Set(['a','c','hidden']),anchor:'a',focus:'c'}
  assert.deepEqual([...selectContextSession(before,['a','b','c'],'b').keys],['b'])
  assert.deepEqual([...selectContextSession(before,['a','c'],'missing').keys],['a','c'])
})
