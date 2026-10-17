// Agent 身份的唯一数据源；品牌 SVG 的固定来源见 icons/README.md。
export type AgentIdentity = {
  key: string
  label: string
  color: string
  icon: string | null
  background?: string
  glyph?: boolean
}

export const AGENT_LIST: AgentIdentity[] = [
  { key: 'claude', label: 'Claude Code', color: '#D97757', icon: '/agents/claude-color.svg', glyph: true },
  { key: 'claude-desktop', label: 'Claude Desktop', color: '#D97757', icon: '/agents/claude-color.svg', glyph: true },
  { key: 'codex', label: 'Codex', color: '#3143FF', icon: '/agents/codex.png' },
  { key: 'zcode', label: 'ZCode', color: '#4C7DF0', icon: '/agents/zcode.png' },
  { key: 'hermes', label: 'Hermes', color: '#B08A2E', icon: '/agents/hermes.png' },
  { key: 'workbuddy', label: 'WorkBuddy', color: '#0FA37F', icon: '/agents/workbuddy.png' },
  { key: 'qoder-cn', label: 'Qoder CN', color: '#6E56CF', icon: '/agents/qoder-color.svg', glyph: true },
  { key: 'qoder', label: 'Qoder', color: '#6E56CF', icon: '/agents/qoder-color.svg', glyph: true },
  { key: 'opencode', label: 'OpenCode', color: '#363A43', icon: '/agents/opencode.svg', glyph: true },
  { key: 'gemini', label: 'Gemini CLI', color: '#4285F4', icon: '/agents/gemini-color.svg', glyph: true },
  { key: 'grok', label: 'Grok', color: '#242B38', icon: '/agents/grok.svg', glyph: true },
  { key: 'kimi', label: 'Kimi', color: '#2B2B45', background: '#151515', icon: '/agents/kimi-color.svg', glyph: true },
  { key: 'minimax-code', label: 'MiniMax Code', color: '#E8455F', icon: '/agents/minimax-color.svg', glyph: true },
  { key: 'cursor', label: 'Cursor', color: '#242B38', icon: '/agents/cursor.svg', glyph: true },
  { key: 'cline', label: 'Cline', color: '#53617A', icon: '/agents/cline.svg', glyph: true },
  { key: 'codebuddy', label: 'CodeBuddy', color: '#1677FF', icon: '/agents/codebuddy-color.svg', glyph: true },
  // Pi Coding Agent 与 Inflection Pi 是不同产品，不借用同名品牌图标。
  { key: 'pi', label: 'Pi', color: '#5661A8', icon: null },
  { key: 'pi-desktop', label: 'Pi Desktop', color: '#5661A8', icon: null },
  { key: 'omp', label: 'oh-my-pi', color: '#5661A8', icon: null },
  { key: 'dsh', label: 'dsh', color: '#53617A', icon: null },
  { key: 'alma', label: 'Alma', color: '#7B6AC8', icon: null },
]

const BY_KEY = new Map(AGENT_LIST.map((a) => [a.key, a]))

export function agentIdentity(agent: string): AgentIdentity {
  return BY_KEY.get(agent) ?? { key: agent, label: agent, color: '#758198', icon: null }
}
