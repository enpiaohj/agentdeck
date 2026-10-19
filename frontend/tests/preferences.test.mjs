import test from 'node:test'
import assert from 'node:assert/strict'
import { parsePreferences, defaultPreferences, rememberProject } from '../src/preferences.ts'
import { CLI_AGENTS, cliAgent, platformLabel } from '../src/agentOptions.ts'

test('默认平台持久化与旧别名兼容，Desktop 和命令注入值回退', () => {
  for (const {value} of CLI_AGENTS) assert.equal(parsePreferences(JSON.stringify({defaultAgent:value})).defaultAgent,value)
  assert.equal(cliAgent('cursor-agent'),'cursor')
  assert.equal(parsePreferences('{"defaultAgent":"claude-desktop"}').defaultAgent,'claude')
  assert.equal(cliAgent('codex; bad'),'claude')
  assert.equal(platformLabel('claude-desktop'),'claude-desktop')
})

test('可启动清单含 dsh（DeepSeek Harness），命令名与后端映射一致', () => {
  assert.ok(CLI_AGENTS.some(o => o.value === 'dsh'))
  assert.equal(cliAgent('dsh'),'dsh')
  assert.equal(platformLabel('dsh'),'DeepSeek Harness CLI')
})

test('外观与阅读设置从本机数据恢复，未知字段不进入设置', () => {
  assert.deepEqual(parsePreferences(JSON.stringify({ theme:'dark',fontSize:16,spacing:'compact',readingStart:'start',foldProcesses:false,readingWidth:'wide',unknown:'x' })), {...defaultPreferences,theme:'dark',fontSize:16,spacing:'compact',readingStart:'start',foldProcesses:false,readingWidth:'wide'})
})
test('损坏或越界设置安全降级，默认展示最新问答并折叠过程记录', () => {
  for (const raw of [null,'bad','null','[]',JSON.stringify({theme:'strange',fontSize:1000,spacing:'x',readingStart:'x',foldProcesses:'no',readingWidth:'x'})]) assert.deepEqual(parsePreferences(raw),defaultPreferences)
  const first=parsePreferences(null)
  first.fontSize=18
  assert.equal(parsePreferences(null).fontSize,15)
})

test('导航宽度越界回落到布局默认，最近项目去重且限量', () => {
  assert.equal(parsePreferences('{"navWidth":200}').navWidth,200)
  for (const bad of [10,500,'200',null,Number.NaN]) assert.equal(parsePreferences(JSON.stringify({navWidth:bad})).navWidth,0)
  assert.deepEqual(parsePreferences(JSON.stringify({recentProjects:[' D:\\A ','d:\\a','',42,'D:\\B']})).recentProjects,['D:\\A','D:\\B'])
  assert.deepEqual(parsePreferences(JSON.stringify({recentProjects:['1','2','3','4','5','6']})).recentProjects,['1','2','3','4','5'])
  assert.deepEqual(rememberProject(['D:\\A','D:\\B'],'d:\\b'),['d:\\b','D:\\A'])
  assert.deepEqual(rememberProject([],'D:\\A'),['D:\\A'])
})
