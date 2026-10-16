// 时间与徽标的展示辅助。
export function fmtTime(rfc3339: string): string {
  if (!rfc3339) return '时间未知'
  const t = new Date(rfc3339)
  if (isNaN(t.getTime())) return '时间未知'
  return t.toLocaleString('zh-CN')
}

export function relTime(rfc3339: string): string {
  if (!rfc3339) return '时间未知'
  const t = new Date(rfc3339)
  if (isNaN(t.getTime())) return '时间未知'
  const s = (Date.now() - t.getTime()) / 1000
  if (s < 60) return '刚刚'
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} 天前`
  return t.toLocaleDateString('zh-CN')
}

export function agentLabel(agent: string): string {
  switch (agent) {
    case 'claude': return 'Claude'
    case 'codex': return 'Codex'
    case 'zcode': return 'ZCode'
    default: return agent
  }
}
