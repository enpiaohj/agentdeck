// 新建会话仅开放后端支持的 CLI 平台；Desktop 保留在浏览列表中。
export const CLI_AGENTS = [
  { value: 'claude', label: 'Claude Code CLI' },
  { value: 'codex', label: 'Codex CLI' },
  { value: 'zcode', label: 'ZCode CLI' },
  { value: 'opencode', label: 'OpenCode CLI' },
  { value: 'qoder', label: 'Qoder CLI' },
  { value: 'qoder-cn', label: 'Qoder CN CLI' },
  { value: 'grok', label: 'Grok CLI' },
  { value: 'droid', label: 'Droid CLI' },
  { value: 'cursor', label: 'Cursor Agent CLI' },
  { value: 'hermes', label: 'Hermes CLI' },
] as const
export type CliAgent = typeof CLI_AGENTS[number]['value']
export function isCliAgent(value: unknown): value is CliAgent {
  return typeof value === 'string' && CLI_AGENTS.some(option => option.value === value)
}
export function cliAgent(value: unknown, fallback: CliAgent = 'claude'): CliAgent {
  return value === 'cursor-agent' ? 'cursor' : isCliAgent(value) ? value : fallback
}
export function platformLabel(value: string): string {
  return CLI_AGENTS.find(option => option.value === (value === 'cursor-agent' ? 'cursor' : value))?.label ?? value
}
