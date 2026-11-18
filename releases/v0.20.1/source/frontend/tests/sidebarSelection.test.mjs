import test from 'node:test'
import assert from 'node:assert/strict'
import { agentRange } from '../src/sidebarSelection.ts'

const order = ['codex', 'hermes', 'zcode', 'claude-desktop', 'claude']

test('Shift 连选按侧栏顺序取锚点到点中项的一段，方向无关', () => {
  assert.deepEqual(agentRange(order, 'hermes', ['hermes'], 'claude-desktop', false), ['hermes', 'zcode', 'claude-desktop'])
  assert.deepEqual(agentRange(order, 'claude', ['claude'], 'zcode', false), ['zcode', 'claude-desktop', 'claude'])
  assert.deepEqual(agentRange(order, 'zcode', ['zcode'], 'zcode', false), ['zcode'])
})

test('Ctrl+Shift 把这一段并入已选，不重复', () => {
  assert.deepEqual(agentRange(order, 'zcode', ['codex', 'zcode'], 'claude', true), ['codex', 'zcode', 'claude-desktop', 'claude'])
})

test('锚点缺失或已不在列表里时，退回已选的第一个，再退回点中的项', () => {
  assert.deepEqual(agentRange(order, null, ['hermes'], 'zcode', false), ['hermes', 'zcode'])
  assert.deepEqual(agentRange(order, 'gone', [], 'zcode', false), ['zcode'])
  assert.deepEqual(agentRange(order, null, ['gone'], 'hermes', false), ['hermes'])
})
