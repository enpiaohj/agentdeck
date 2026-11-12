type DeletionSource = { agent: string; source: string }

/** 标题用问句，数量写进去。 */
export function deleteTitle(count: number): string {
  return count === 1 ? '要删除此会话吗？' : `要删除这 ${count} 个会话吗？`
}

/** 正文一句话说清后果；是否可撤销取决于「删除前创建恢复备份」设置。 */
export function deleteSummary(backup: boolean): string {
  return backup
    ? '此操作将删除原始会话记录，以及 AgentDeck 中的所有版本、收藏、标签和备注。删除前将自动创建恢复备份，可在本次运行期间撤销。'
    : '此操作将永久删除原始会话记录，以及 AgentDeck 中的所有版本、收藏、标签和备注。当前未启用删除前备份，删除后无法恢复。'
}

/** 不可撤销时才需要的那一次勾选；可撤销时不增加额外步骤。 */
export const irreversibleAck = '我了解此操作无法撤销'

/** 会话列表最多直接列出 limit 个标题，其余折成「以及另外 N 个」。 */
export function deletionTitles(titles: readonly string[], limit = 5): { shown: string[]; more: number } {
  const named = titles.map(title => title || '未命名会话')
  return { shown: named.slice(0, limit), more: Math.max(0, named.length - limit) }
}

export function deletionUnavailableReason(items: readonly DeletionSource[]): string {
  if (!items.length) return '请先选择会话。'
  if (items.some(item => item.source === 'wsl')) return '暂不支持删除 WSL 原始记录，请在 WSL 中管理。'
  if (items.some(item => item.source === 'import')) return '导入记录暂不支持删除原始会话。'
  if (items.some(item => item.source === 'cache')) return 'Desktop 聊天缓存是云端会话的本机副本，不支持删除原始会话。'
  if (items.some(item => !['local', 'retained'].includes(item.source))) return '仅支持删除本机原生或永久保留的会话，网关等其他来源暂不支持。'
  if (items.some(item => !['codex', 'claude', 'claude-desktop'].includes(item.agent))) return '仅支持删除 Codex、Claude Code 和 Claude Desktop 原始会话。'
  return ''
}

// Explicit visible selection is the entire initial scope. Child expansion requires
// a separate opt-in and a fresh preview from the backend.
export function explicitDeletionTargets<T extends { key: string }>(visibleRows: readonly T[], selected: ReadonlySet<string>): T[] {
  return visibleRows.filter(row => selected.has(row.key))
}

export function deletedSessionKeys(payload: unknown): string[] {
  const value = Array.isArray(payload) && payload.length === 1 && Array.isArray(payload[0]) ? payload[0] : payload
  return Array.isArray(value) && value.every(key => typeof key === 'string') ? value : []
}
