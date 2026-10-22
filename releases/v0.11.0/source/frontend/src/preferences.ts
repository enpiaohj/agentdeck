import { cliAgent, type CliAgent } from './agentOptions.ts'

// navWidth 为 0 表示跟随布局默认（窄窗口下会自己收窄）；用户拖过分隔条后
// 记具体像素值，此时不再被媒体查询改写。listWidth / infoWidth 同一约定。
export type Preferences = { defaultAgent: CliAgent; theme: 'system' | 'light' | 'dark'; fontSize: number; spacing: 'comfortable' | 'compact'; readingStart: 'latest' | 'start'; foldProcesses: boolean; readingWidth: 'balanced' | 'wide' | 'adaptive'; navWidth: number; listWidth: number; infoWidth: number; recentProjects: string[]; navCollapsed: boolean; infoPanel: 'open' | 'closed'; listOrder: 'latest' | 'oldest'; hideEmptyProjects: boolean; agentsCollapsed: boolean; projectsCollapsed: boolean; showAllProjects: boolean; promptRail: boolean }
// readerWidthCss 是消息列的宽度上限（max-width 的值）。自适应档取阅读区的约 80%、
// 封顶 1400px，但不窄于适中档的 820px，也不超过阅读区本身：小窗口退回适中，
// 大屏随阅读区放宽，代码块与表格在列内因此一同放宽。
export function readerWidthCss(width: Preferences['readingWidth']): string {
  if (width === 'wide') return '1120px'
  if (width === 'adaptive') return 'min(100%, max(820px, min(80%, 1400px)))'
  return '820px'
}

export const defaultPreferences: Preferences = { defaultAgent:'claude', theme:'system', fontSize:15, spacing:'comfortable', readingStart:'latest', foldProcesses:true, readingWidth:'adaptive', navWidth:0, listWidth:0, infoWidth:0, recentProjects:[], navCollapsed:false, infoPanel:'open', listOrder:'latest', hideEmptyProjects:false, agentsCollapsed:false, projectsCollapsed:false, showAllProjects:false, promptRail:true }
export const preferencesKey = 'agentdeck.preferences.v1'
// maxShare 是这一栏最多占窗口宽度的比例，作为「窗口里其他栏很窄」时的兜底上限；
// reserved 是另两栏的实际宽度加阅读区下限，用它算出的上限更贴合真实布局。
export type PaneWidthRange = { min: number; max: number; maxShare: number }
export const navWidthRange: PaneWidthRange = { min: 148, max: 420, maxShare: 0.3 }
export const listWidthRange: PaneWidthRange = { min: 240, max: 560, maxShare: 0.45 }
export const infoWidthRange: PaneWidthRange = { min: 220, max: 480, maxShare: 0.34 }
// clampPaneWidth 把目标宽度收进三重约束：该栏上下限、窗口份额、以及
// 「窗口宽度 − 另两栏实际宽度 − 阅读区下限」。reserved 为 0 时只用前两重。
export function clampPaneWidth(value: number, range: PaneWidthRange, viewportWidth: number, reserved = 0): number {
  const byShare = Math.round(viewportWidth * range.maxShare)
  const bySpace = reserved > 0 ? viewportWidth - reserved : range.max
  const ceiling = Math.max(range.min, Math.min(range.max, byShare, bySpace))
  return Math.min(ceiling, Math.max(range.min, Math.round(value)))
}
// paneWidthCss 让已保存的像素宽度不超过该栏上下限（CSS 侧再兜一次底），
// 拖动时的动态上限由 clampPaneWidth 在 JS 里算，因为要读另两栏的实际宽度。
export function paneWidthCss(width: number, range: PaneWidthRange): string {
  return `min(${width}px, ${range.max}px)`
}
export const recentProjectsLimit = 5
export function parsePreferences(raw: string | null): Preferences {
  let value: Record<string, unknown>
  try { const parsed = JSON.parse(raw ?? '{}'); value = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {} } catch { value = {} }
  return {
    defaultAgent: cliAgent(value.defaultAgent),
    theme: value.theme === 'light' || value.theme === 'dark' ? value.theme : 'system',
    fontSize: typeof value.fontSize === 'number' && [13,14,15,16,18].includes(value.fontSize) ? value.fontSize : 15,
    spacing: value.spacing === 'compact' ? 'compact' : 'comfortable',
    readingStart: value.readingStart === 'start' ? 'start' : 'latest',
    foldProcesses: typeof value.foldProcesses === 'boolean' ? value.foldProcesses : true,
    readingWidth: value.readingWidth === 'balanced' || value.readingWidth === 'wide' || value.readingWidth === 'adaptive' ? value.readingWidth : 'adaptive',
    navWidth: parsePaneWidth(value.navWidth, navWidthRange),
    listWidth: parsePaneWidth(value.listWidth, listWidthRange),
    infoWidth: parsePaneWidth(value.infoWidth, infoWidthRange),
    recentProjects: parseRecentProjects(value.recentProjects),
    navCollapsed: typeof value.navCollapsed === 'boolean' ? value.navCollapsed : false,
    infoPanel: value.infoPanel === 'closed' ? 'closed' : 'open',
    listOrder: value.listOrder === 'oldest' ? 'oldest' : 'latest',
    hideEmptyProjects: typeof value.hideEmptyProjects === 'boolean' ? value.hideEmptyProjects : false,
    // 侧栏分区折叠与「显示全部项目」：只是展示偏好，缺省全部展开、项目只列前几个
    agentsCollapsed: typeof value.agentsCollapsed === 'boolean' ? value.agentsCollapsed : false,
    projectsCollapsed: typeof value.projectsCollapsed === 'boolean' ? value.projectsCollapsed : false,
    showAllProjects: typeof value.showAllProjects === 'boolean' ? value.showAllProjects : false,
    // 阅读区右侧的提问导航条：缺省显示，可在阅读导航栏一键收起
    promptRail: typeof value.promptRail === 'boolean' ? value.promptRail : true,
  }
}

// parsePaneWidth 只接受落在该栏上下限内的像素值，其余（缺省、字符串、越界）
// 一律回到 0，表示跟随布局默认。
function parsePaneWidth(value: unknown, range: PaneWidthRange): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  const width = Math.round(value)
  return width >= range.min && width <= range.max ? width : 0
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