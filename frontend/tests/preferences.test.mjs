import test from 'node:test'
import assert from 'node:assert/strict'
import { parsePreferences, defaultPreferences, rememberProject, readerWidthCss, clampPaneWidth, paneWidthCss, navWidthRange, listWidthRange, infoWidthRange } from '../src/preferences.ts'
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

test('会话列表与会话详情栏宽度同样越界回落，缺省跟随布局', () => {
  assert.equal(defaultPreferences.listWidth,0)
  assert.equal(defaultPreferences.infoWidth,0)
  assert.equal(parsePreferences(JSON.stringify({listWidth:420,infoWidth:300})).listWidth,420)
  assert.equal(parsePreferences(JSON.stringify({listWidth:420,infoWidth:300})).infoWidth,300)
  for (const bad of [100,900,'420',null,Number.NaN,Infinity]) assert.equal(parsePreferences(JSON.stringify({listWidth:bad})).listWidth,0, String(bad))
  for (const bad of [100,900,'300',null,Number.NaN,Infinity]) assert.equal(parsePreferences(JSON.stringify({infoWidth:bad})).infoWidth,0, String(bad))
})

test('拖动宽度受上下限、窗口份额与阅读区下限三重约束，不会把阅读区挤没', () => {
  assert.equal(clampPaneWidth(10, listWidthRange, 1600), listWidthRange.min)
  // 窗口够宽时上限是 range.max
  assert.equal(clampPaneWidth(5000, listWidthRange, 2000), listWidthRange.max)
  // 窗口窄时按份额收窄：1000px 宽的窗口下列表最多 450px
  assert.equal(clampPaneWidth(560, listWidthRange, 1000), 450)
  assert.equal(clampPaneWidth(10, navWidthRange, 1000), navWidthRange.min)
  // reserved 是另两栏实际宽度加阅读区下限：可用空间不够时进一步收紧
  assert.equal(clampPaneWidth(560, listWidthRange, 1280, 192 + 268 + 360), 460)
  // 但永远不会低于最小宽度，避免栏被压成一条缝
  assert.equal(clampPaneWidth(300, listWidthRange, 600, 560), listWidthRange.min)
  assert.equal(clampPaneWidth(900, infoWidthRange, 1600, 192 + 320 + 360), infoWidthRange.max)
  assert.match(paneWidthCss(420, listWidthRange), /^min\(420px, 560px\)$/)
  assert.match(paneWidthCss(300, infoWidthRange), /^min\(300px, 480px\)$/)
})

test('阅读宽度三档：自适应保留下限与上限，缺省与未知值回退自适应', () => {
  for (const w of ['balanced','wide','adaptive']) assert.equal(parsePreferences(JSON.stringify({readingWidth:w})).readingWidth,w)
  assert.equal(parsePreferences('{"readingWidth":"huge"}').readingWidth,'adaptive')
  assert.equal(parsePreferences('{}').readingWidth,'adaptive')
  assert.equal(defaultPreferences.readingWidth,'adaptive')
  assert.equal(readerWidthCss('balanced'),'820px')
  assert.equal(readerWidthCss('wide'),'1120px')
  const css = readerWidthCss('adaptive')
  assert.match(css,/80%/); assert.match(css,/1400px/); assert.match(css,/820px/); assert.match(css,/100%/)
})

test('工作台偏好：缺省值、合法值保留、非法值回退', () => {
  const d = defaultPreferences
  assert.deepEqual([d.navCollapsed,d.infoPanel,d.listOrder,d.hideEmptyProjects],[false,'open','latest',false])
  const p = parsePreferences(JSON.stringify({navCollapsed:true,infoPanel:'closed',listOrder:'oldest',hideEmptyProjects:true}))
  assert.deepEqual([p.navCollapsed,p.infoPanel,p.listOrder,p.hideEmptyProjects],[true,'closed','oldest',true])
  const bad = parsePreferences(JSON.stringify({navCollapsed:'yes',infoPanel:'x',listOrder:1,hideEmptyProjects:'1'}))
  assert.deepEqual([bad.navCollapsed,bad.infoPanel,bad.listOrder,bad.hideEmptyProjects],[false,'open','latest',false])
  // 侧栏分区折叠与「显示全部项目」：缺省展开、只列前几个；非布尔值回退缺省
  assert.deepEqual([d.agentsCollapsed,d.projectsCollapsed,d.showAllProjects],[false,false,false])
  const s = parsePreferences(JSON.stringify({agentsCollapsed:true,projectsCollapsed:true,showAllProjects:true}))
  assert.deepEqual([s.agentsCollapsed,s.projectsCollapsed,s.showAllProjects],[true,true,true])
  const sb = parsePreferences(JSON.stringify({agentsCollapsed:1,projectsCollapsed:'y',showAllProjects:null}))
  assert.deepEqual([sb.agentsCollapsed,sb.projectsCollapsed,sb.showAllProjects],[false,false,false])
  // 提问导航条缺省显示，只认布尔值
  assert.equal(d.promptRail, true)
  assert.equal(parsePreferences(JSON.stringify({promptRail:false})).promptRail, false)
  assert.equal(parsePreferences(JSON.stringify({promptRail:'off'})).promptRail, true)
})
