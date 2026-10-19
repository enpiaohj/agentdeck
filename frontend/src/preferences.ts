import { cliAgent, type CliAgent } from './agentOptions.ts'

// navWidth 为 0 表示跟随布局默认（窄窗口下会自己收窄）；用户拖过分隔条后
// 记具体像素值，此时不再被媒体查询改写。
export type Preferences = { defaultAgent: CliAgent; theme: 'system' | 'light' | 'dark'; fontSize: number; spacing: 'comfortable' | 'compact'; readingStart: 'latest' | 'start'; foldProcesses: boolean; readingWidth: 'balanced' | 'wide'; navWidth: number; recentProjects: string[] }
export const defaultPreferences: Preferences = { defaultAgent:'claude', theme:'system', fontSize:15, spacing:'comfortable', readingStart:'latest', foldProcesses:true, readingWidth:'balanced', navWidth:0, recentProjects:[] }
export const preferencesKey = 'agentdeck.preferences.v1'
export const navWidthRange = { min: 148, max: 420 } as const
export const recentProjectsLimit = 5
export function parsePreferences(raw: string | null): Preferences {
  let value: Record<string, unknown>
  try { const parsed = JSON.parse(raw ?? '{}'); value = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {} } catch { value = {} }
  const width = typeof value.navWidth === 'number' && Number.isFinite(value.navWidth) ? Math.round(value.navWidth) : 0
  return {
    defaultAgent: cliAgent(value.defaultAgent),
    theme: value.theme === 'light' || value.theme === 'dark' ? value.theme : 'system',
    fontSize: typeof value.fontSize === 'number' && [13,14,15,16,18].includes(value.fontSize) ? value.fontSize : 15,
    spacing: value.spacing === 'compact' ? 'compact' : 'comfortable',
    readingStart: value.readingStart === 'start' ? 'start' : 'latest',
    foldProcesses: typeof value.foldProcesses === 'boolean' ? value.foldProcesses : true,
    readingWidth: value.readingWidth === 'wide' ? 'wide' : 'balanced',
    navWidth: width >= navWidthRange.min && width <= navWidthRange.max ? width : 0,
    recentProjects: parseRecentProjects(value.recentProjects),
  }
}

// parseRecentProjects 只留非空字符串，越界的整项丢弃（不截断路径：截过的
// 路径点开必然不存在，比不记更糟）。
function parseRecentProjects(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const out: string[] = []
  for (const item of value) {
    if (typeof item !== 'string') continue
    const path = item.trim()
    if (!path || out.some(existing => existing.toLowerCase() === path.toLowerCase())) continue
    out.push(path)
    if (out.length >= recentProjectsLimit) break
  }
  return out
}

// rememberProject 把最近用过的目录挪到最前：同路径（大小写不敏感）只留一条，
// 超出上限的从尾部丢掉。
export function rememberProject(list: string[], path: string): string[] {
  return parseRecentProjects([path, ...list])
}