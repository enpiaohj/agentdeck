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
  const preview = (text.split(/\r?\n/).find(line => line.trim() && !/^[\s{}\[\],]*$/.test(line)) || '').replace(/\s+/g, ' ').trim().slice(0, 100)
  return { label, preview, lines: text ? text.split(/\r?\n/).length : 0, monospace }
}

export function isLongCode(text: string): boolean {
  return text.replace(/^\n+|\n+$/g, '').split('\n').length >= 30
}

// react-markdown 为代码节点补一个换行；只移除它，保留原文的空行。
export function markdownCodeText(text: string): string {
  return text.endsWith('\n') ? text.slice(0, -1) : text
}
