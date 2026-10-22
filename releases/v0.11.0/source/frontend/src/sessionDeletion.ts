type DeletionSource = { agent: string; source: string }

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
