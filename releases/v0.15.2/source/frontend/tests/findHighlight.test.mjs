import test from 'node:test'
import assert from 'node:assert/strict'
import { matchRanges, needleOf } from '../src/findHighlight.ts'

test('关键词不区分大小写，多个空白折成一个', () => {
  assert.equal(needleOf('  Hello   World '), 'hello world')
  assert.equal(needleOf('   '), '')
})

test('返回所有不重叠的命中区间', () => {
  assert.deepEqual(matchRanges('Foo foo FOO', 'foo'), [[0, 3], [4, 7], [8, 11]])
  assert.deepEqual(matchRanges('aaaa', 'aa'), [[0, 2], [2, 4]])
})

test('中文与无命中', () => {
  assert.deepEqual(matchRanges('更新页与搜索，更新', '更新'), [[0, 2], [7, 9]])
  assert.deepEqual(matchRanges('abc', 'x'), [])
  assert.deepEqual(matchRanges('', 'x'), [])
  assert.deepEqual(matchRanges('abc', ''), [])
})

test('小写化改变长度的字符不让区间错位', () => {
  // 'İ'.toLowerCase() 是两个码元：放弃折叠，只做原样匹配。
  const text = 'İx abc'
  for (const [start, end] of matchRanges(text, 'abc')) assert.equal(text.slice(start, end), 'abc')
})
