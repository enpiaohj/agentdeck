type Part = { role: string; kind: string; name?: string; text: string }

export type FoldedContent = { label: string; preview: string; lines: number; monospace: boolean }

// 只折叠过程记录和可确认的结构化数据，普通问答保持展开。
export function foldedContent(part: Part): FoldedContent | null {
  let label = ''
  let monospace = false
  if (part.kind === 'thinking' || part.role === 'thinking') label = '思考过程'
  else if (part.kind === 'tool_use') { label = '工具调用'; monospace = true }
  else if (part.kind === 'tool_result' || part.role === 'tool') { label = '工具结果'; monospace = true }
  else if (part.kind === 'context') label = '上下文记录'
  else if (part.role !== 'user' && /^[\[{]/.test(part.text.trim()) && (part.text.length >= 600 || part.text.split('\n').length >= 6)) {
    try { JSON.parse(part.text); label = '结构化数据'; monospace = true } catch { /* 普通文本和不完整 JSON 保持可见。 */ }
  }
  if (!label) return null
  if (part.name) label += ` · ${part.name}`
  const text = part.text.trim()
  return { label, preview: foldPreview(text), lines: text ? text.split(/\r?\n/).length : 0, monospace }
}

// JSON 正文的摘要取第一个有语义的标量字段（command / file_path / pattern 等），
// 否则首行只是 `{"script":` 这样的外壳，看不出这个工具做了什么。取不到标量
// 时退回顶层键名列表；都不是则退回原文首个有效行（普通文本走这条）。
// 顺序即优先级：越能说明「做了什么」的字段越靠前，name / title 这类通用名押后。
const previewKeys = ['command', 'file_path', 'path', 'pattern', 'query', 'script', 'status', 'error', 'message', 'description', 'tool', 'name', 'title', 'url', 'input']

function jsonPreview(text: string): string {
  let value: unknown
  try { value = JSON.parse(text) } catch { return '' }
  const record = (Array.isArray(value) ? value[0] : value) as Record<string, unknown> | null | undefined
  if (!record || typeof record !== 'object') return typeof value === 'string' ? value : ''
  for (const key of previewKeys) {
    const field = record[key]
    if (typeof field === 'string' && field.trim()) return `${key}: ${field.trim()}`
    if (typeof field === 'number' || typeof field === 'boolean') return `${key}: ${field}`
  }
  const keys = Object.keys(record).filter(key => record[key] !== null && record[key] !== undefined)
  return keys.length ? `字段：${keys.slice(0, 4).join('、')}${keys.length > 4 ? ' …' : ''}` : ''
}

function foldPreview(text: string): string {
  if (/^[\[{]/.test(text)) {
    const summary = jsonPreview(text)
    if (summary) return summary.replace(/\s+/g, ' ').trim().slice(0, 100)
  }
  return (text.split(/\r?\n/).find(line => line.trim() && !/^[\s{}\[\],]*$/.test(line)) || '').replace(/\s+/g, ' ').trim().slice(0, 100)
}

// 仅按相邻位置分组，不推断工具调用与结果的配对关系。
export function transcriptGroups<T extends Part>(parts: T[], start = 0): { process: boolean; start: number; parts: T[] }[] {
  const groups: { process: boolean; start: number; parts: T[] }[] = []
  parts.forEach((part, index) => {
    const process = foldedContent(part) !== null
    const previous = groups.at(-1)
    if (process && previous?.process) previous.parts.push(part)
    else groups.push({ process, start: start + index, parts: [part] })
  })
  return groups
}

export function processSummary(parts: Part[]): string {
  const counts = new Map<string, number>()
  for (const part of parts) {
    const label = foldedContent(part)?.label.split(' · ')[0]
    if (label) counts.set(label, (counts.get(label) ?? 0) + 1)
  }
  return [...counts].map(([label, count]) => `${label} ${count}`).join(' · ')
}

export function isLongCode(text: string): boolean {
  return text.replace(/^\n+|\n+$/g, '').split('\n').length >= 30
}

// react-markdown 为代码节点补一个换行；只移除它，保留原文的空行。
export function markdownCodeText(text: string): string {
  return text.endsWith('\n') ? text.slice(0, -1) : text
}
