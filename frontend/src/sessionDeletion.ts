type DeletionSource = { agent: string; source: string }

/** 标题用问句，数量写进去。 */
export function deleteTitle(count: number): string {
  return count === 1 ? '要删除此会话吗？' : `要删除这 ${count} 个会话吗？`
}

// 「允许删除导入的聊天记录」设置的当前取值。取值由后端持有并强制检查（界面置灰只是提示），
// 启动时由 App 读入，设置页改动时同步。默认关闭。
let importDeleteOn = false
export function setImportDeleteEnabled(value: boolean): void { importDeleteOn = value }
export function isImportDeleteEnabled(): boolean { return importDeleteOn }

/** 导入的 ChatGPT 会话：它的「原始记录」是 AgentDeck 自己的副本，删除不影响 ChatGPT 上的聊天。 */
export const isChatGPTImport = (item: DeletionSource): boolean => item.source === 'import' && item.agent === 'chatgpt'

/** 正文一句话说清后果；是否可撤销取决于「删除前创建恢复备份」设置。imported 为真时说明只删本机副本。 */
export function deleteSummary(backup: boolean, imported = false): string {
  if (imported) {
    return backup
      ? '此操作只删除 AgentDeck 里导入的副本，以及它的所有版本、收藏、标签和备注；ChatGPT 上的原始聊天和你的导出文件不受影响。重新导入同一份导出，也不会让它回来。删除前将自动创建恢复备份，可在本次运行期间撤销。'
      : '此操作只删除 AgentDeck 里导入的副本，以及它的所有版本、收藏、标签和备注；ChatGPT 上的原始聊天和你的导出文件不受影响。当前未启用删除前备份，删除后无法恢复。'
  }
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
  // 导入的 ChatGPT 会话只有在设置里开启后才能删；其余导入记录（如 Claude 的）始终不支持
  const deletableImport = (item: DeletionSource) => importDeleteOn && isChatGPTImport(item)
  const blockedImports = items.filter(item => item.source === 'import' && !deletableImport(item))
  if (blockedImports.length) {
    return !importDeleteOn && blockedImports.some(isChatGPTImport)
      ? '导入记录默认不能删除。要删除导入的 ChatGPT 会话，请先在“设置”>“会话”里开启“允许删除导入的聊天记录”；Claude 等其他导入记录始终不支持删除。'
      : '只有导入的 ChatGPT 会话可以删除；Claude 等其他导入记录暂不支持删除。'
  }
  if (items.some(item => item.source === 'cache')) return 'Desktop 聊天缓存是云端会话的本机副本，不支持删除原始会话。'
  if (items.some(item => !deletableImport(item) && !['local', 'retained'].includes(item.source))) return '仅支持删除本机原生或永久保留的会话，网关等其他来源暂不支持。'
  if (items.some(item => !deletableImport(item) && !['codex', 'claude', 'claude-desktop'].includes(item.agent))) return '仅支持删除 Codex、Claude Code 和 Claude Desktop 原始会话。'
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
