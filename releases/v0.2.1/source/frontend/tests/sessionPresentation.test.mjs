import test from 'node:test'
import assert from 'node:assert/strict'
import { presentSessions, sessionDateGroup, sessionMonth } from '../src/sessionPresentation.ts'

const rows = [
  { title: '修复 Windows 打包', project: 'AgentDeck', agent: 'codex', tags: ['build'], last: '2026-10-16T12:00:00Z' },
  { title: '远程连接功能', project: 'ExampleProject', agent: 'claude', tags: ['P2P'], last: '2026-10-17T12:00:00Z' },
  { title: '历史导入', project: '', agent: 'kimi', tags: [], last: '' },
]

test('标题、项目和标签可搜索，忽略大小写与全角字符', () => {
  assert.equal(presentSessions(rows, ' ＢＵＩＬＤ ', 'latest')[0].title, rows[0].title)
  assert.equal(presentSessions(rows, '远程', 'latest')[0].title, rows[1].title)
  assert.equal(presentSessions(rows, 'ExampleProject', 'latest')[0].title, rows[1].title)
  assert.deepEqual(presentSessions(rows, '不存在', 'latest'), [])
})

test('最新/最早排序均将未知时间放在最后，不修改原列表', () => {
  const original = rows.map(row => ({ ...row }))
  assert.deepEqual(presentSessions(rows, '', 'latest').map(row => row.title), [rows[1].title, rows[0].title, rows[2].title])
  assert.deepEqual(presentSessions(rows, '', 'oldest').map(row => row.title), [rows[0].title, rows[1].title, rows[2].title])
  assert.deepEqual(rows, original)
})

test('历史会话按实际年月分组，月初不会将上个月误标为本月', () => {
  const now = new Date(2026, 9, 2, 12)
  assert.equal(sessionDateGroup(new Date(2026, 9, 2, 10).toISOString(), now), '今天')
  assert.equal(sessionDateGroup(new Date(2026, 9, 1, 10).toISOString(), now), '昨天')
  assert.equal(sessionDateGroup(new Date(2026, 8, 30, 10).toISOString(), now), '2026年9月')
  assert.equal(sessionDateGroup(new Date(2025, 9, 1, 10).toISOString(), now), '2025年10月')
  assert.equal(sessionMonth(new Date(2026, 8, 30, 10).toISOString()), '2026-09')
  assert.equal(sessionMonth('bad'), '')
  assert.equal(sessionDateGroup(''), '时间未知')
})

test('账号名称可检索，账号筛选不会混入其他账号或本地未标注记录', () => {
  const accounts = [{ ...rows[0], account: '工作账号' }, { ...rows[1], account: '个人账号' }, rows[2]]
  assert.deepEqual(presentSessions(accounts, '', 'latest', '工作账号').map(row => row.title), [rows[0].title])
  assert.equal(presentSessions(accounts, '个人账号', 'latest')[0].title, rows[1].title)
})
