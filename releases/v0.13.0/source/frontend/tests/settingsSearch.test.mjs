import test from 'node:test'
import assert from 'node:assert/strict'
import { matchesSetting, settingSearchKeywords } from '../src/settingsSearch.ts'

test('设置搜索支持跨标题与说明的多关键词，所有词都必须匹配', () => {
  assert.equal(matchesSetting('启动 托盘', '开机启动', '登录后在系统托盘运行'), true)
  assert.equal(matchesSetting('启动 删除', '开机启动', '登录后在系统托盘运行'), false)
})

test('设置搜索统一全角、大小写和空白，不改变原始文案', () => {
  const title = 'PowerShell 7+'
  assert.equal(matchesSetting('  ＰＯＷＥＲＳＨＥＬＬ　7  ', title), true)
  assert.equal(title, 'PowerShell 7+')
  assert.equal(matchesSetting('  ', title), true)
})

test('终端与主题选项可按实际名称搜索，不依赖精简后的说明文案', () => {
  for (const query of ['PowerShell 7', '7+', 'Windows PowerShell', '5.1', '自定义']) {
    assert.equal(matchesSetting(query, settingSearchKeywords.terminalShell), true, query)
  }
  assert.equal(matchesSetting('深色', settingSearchKeywords.theme), true)
})
