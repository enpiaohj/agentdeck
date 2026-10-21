type SessionSummary = { title: string; project: string; agent: string; tags: string[]; last: string; account?: string }
export type SessionOrder = 'latest' | 'oldest'

const normalise = (value: string) => value.normalize('NFKC').toLocaleLowerCase().trim()

export function sessionMonth(value: string): string {
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}` : ''
}

export function sessionDateGroup(value: string, now = new Date()): string {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return '时间未知'
  const day = (d: Date) => Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())
  const days = (day(now) - day(date)) / 86400000
  if (days <= 0) return '今天'
  if (days === 1) return '昨天'
  if (date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth()) return days < 7 ? '本周' : '本月'
  return `${date.getFullYear()}年${date.getMonth() + 1}月`
}

// 当前列表的纯展示筛选，不触碰上游存储，也不修改后端返回的数组。
export function presentSessions<T extends SessionSummary>(rows: readonly T[], query: string, order: SessionOrder, account = ''): T[] {
  const terms = normalise(query).split(/\s+/).filter(Boolean)
  return rows.filter(row => {
    const haystack = normalise([row.title, row.project, row.agent, row.account ?? '', ...row.tags].join(' '))
    return (!account || row.account === account) && terms.every(term => haystack.includes(term))
  }).sort((a, b) => {
    const first = Date.parse(a.last), second = Date.parse(b.last)
    if (!Number.isFinite(first)) return Number.isFinite(second) ? 1 : 0
    if (!Number.isFinite(second)) return -1
    return order === 'latest' ? second - first : first - second
  })
}

// 原 Agent 已删除、本机文件仍在的会话：列表与详情只做低调提示，不改变可读性。
export const UPSTREAM_DELETED_HINT = '已在原 Agent 中删除；本机文件仍在，可照常阅读、检索和导出'
