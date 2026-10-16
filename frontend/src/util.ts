// 时间与徽标的展示辅助 + Agent 品牌视觉。

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
    case 'claude': return 'Claude Code'
    case 'codex': return 'Codex'
    case 'zcode': return 'ZCode'
    case 'opencode': return 'OpenCode'
    default: return agent
  }
}

// Agent 品牌视觉:每个 agent 一个识别色与缩写字符,列表/详情/诊断
// 统一使用。颜色是静态展示值,不代表能力状态(能力看诊断页)。
export function agentMeta(agent: string): { label: string; color: string; abbr: string } {
  switch (agent) {
    case 'claude': return { label: 'Claude Code', color: '#D97757', abbr: 'C' }
    case 'codex': return { label: 'Codex', color: '#10A37F', abbr: 'X' }
    case 'zcode': return { label: 'ZCode', color: '#4C7DF0', abbr: 'Z' }
    case 'opencode': return { label: 'OpenCode', color: '#B06AD4', abbr: 'O' }
    default: return { label: agent, color: '#8A8A8A', abbr: agent.slice(0, 1).toUpperCase() || '?' }
  }
}
