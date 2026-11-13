// 时间与徽标的展示辅助 + Agent 品牌视觉。
// Agent 的展示名/识别色/图标统一由 icons/agents.ts 提供(图标体系唯一数据源),
// 这里只做转发,避免两处各写一份。
import { agentIdentity } from './icons/agents'

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
  return agentIdentity(agent).label
}

// Agent 品牌视觉:查 icons/agents.ts 的注册表(含真实品牌图标)。
// 颜色是静态展示值,不代表能力状态(能力看诊断页)。
export function agentMeta(agent: string): { label: string; color: string; abbr: string } {
  const id = agentIdentity(agent)
  return { label: id.label, color: id.color, abbr: (id.label[0] ?? '?').toUpperCase() }
}
