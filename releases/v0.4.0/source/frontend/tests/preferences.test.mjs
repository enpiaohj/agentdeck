import test from 'node:test'
import assert from 'node:assert/strict'
import { parsePreferences, defaultPreferences } from '../src/preferences.ts'

test('外观与阅读设置从本机数据恢复，未知字段不进入设置', () => {
  assert.deepEqual(parsePreferences(JSON.stringify({ theme:'dark',fontSize:16,spacing:'compact',readingStart:'start',foldProcesses:false,readingWidth:'wide',unknown:'x' })), {theme:'dark',fontSize:16,spacing:'compact',readingStart:'start',foldProcesses:false,readingWidth:'wide'})
})
test('损坏或越界设置安全降级，默认展示最新问答并折叠过程记录', () => {
  for (const raw of [null,'bad','null','[]',JSON.stringify({theme:'strange',fontSize:1000,spacing:'x',readingStart:'x',foldProcesses:'no',readingWidth:'x'})]) assert.deepEqual(parsePreferences(raw),defaultPreferences)
  const first=parsePreferences(null)
  first.fontSize=18
  assert.equal(parsePreferences(null).fontSize,15)
})
