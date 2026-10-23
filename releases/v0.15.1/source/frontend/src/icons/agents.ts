// Agent 身份的唯一数据源；品牌 SVG 的固定来源见 icons/README.md。
export type AgentIdentity = {
  key: string
  label: string
  color: string
  icon: string | null
  background?: string
  glyph?: boolean
  /** 单色 SVG：以 CSS mask 着成白色，放在 background 色的满幅底板上，与官方应用图标同形。 */
  mask?: string
  /** 没有可再分发图标时的自绘字母标（系统字体），不仿画品牌标志。 */
  monogram?: string
  /** 单色图形（SVG 用 currentColor）：按主题着色，深色主题下不再是刺眼的白底方块。 */
  mono?: boolean
}

export const AGENT_LIST: AgentIdentity[] = [
  // Claude、Codex 的官方应用图标是「品牌色底板 + 白色图形」，按同样形态呈现；
  // 图形是 Lobe Icons 的单色 SVG，只着色、不改形。
  { key: 'claude', label: 'Claude Code', color: '#D97757', icon: null, background: '#D97757', mask: '/agents/claude.svg' },
  { key: 'claude-desktop', label: 'Claude Desktop', color: '#D97757', icon: null, background: '#D97757', mask: '/agents/claude.svg' },
  // 品牌位图不再随包分发：它们提取自各产品的专有安装包，没有再分发授权。
  // 有 Lobe Icons（MIT）SVG 的用 SVG，其余留 null，由 AgentMark 回退到
  // Fluent 终端图标并保留可访问名称。
  // Codex 桌面版已并入 ChatGPT 应用（MSIX 包 OpenAI.Codex，显示名 ChatGPT）；开始菜单与
  // 任务栏用的是 lightunplated 资源：透明底黑色 OpenAI 标志。蓝色底板只出现在旧式磁贴上。
  { key: 'codex', label: 'Codex', color: '#000000', icon: '/agents/openai.svg', glyph: true, mono: true },
  // ZCode 没有可再分发的图标：深色底板 + 自绘字母标，接近其应用图标的观感但不复制标志。
  { key: 'zcode', label: 'ZCode', color: '#FFFFFF', icon: null, background: '#111214', monogram: 'Z' },
  // Hermes 没有产品自己的开源图标，用出品方 Nous Research 的标识。
  { key: 'hermes', label: 'Hermes', color: '#B08A2E', icon: '/agents/nousresearch.svg', glyph: true, mono: true },
  { key: 'workbuddy', label: 'WorkBuddy', color: '#0FA37F', icon: '/agents/workbuddy-color.svg' },
  { key: 'workbuddy-ai', label: 'WorkBuddy AI', color: '#0FA37F', icon: '/agents/workbuddy-color.svg' },
  { key: 'qoder-cn', label: 'Qoder CN', color: '#6E56CF', icon: '/agents/qoder-color.svg', glyph: true },
  { key: 'qoder', label: 'Qoder', color: '#6E56CF', icon: '/agents/qoder-color.svg', glyph: true },
  { key: 'opencode', label: 'OpenCode', color: '#363A43', icon: '/agents/opencode.svg', glyph: true, mono: true },
  { key: 'gemini', label: 'Gemini CLI', color: '#4285F4', icon: '/agents/gemini-color.svg', glyph: true },
  { key: 'grok', label: 'Grok', color: '#242B38', icon: '/agents/grok.svg', glyph: true, mono: true },
  { key: 'kimi', label: 'Kimi', color: '#2B2B45', background: '#151515', icon: '/agents/kimi-color.svg', glyph: true },
  { key: 'minimax-code', label: 'MiniMax Code', color: '#E8455F', icon: '/agents/minimax-color.svg', glyph: true },
  { key: 'cursor', label: 'Cursor', color: '#242B38', icon: '/agents/cursor.svg', glyph: true, mono: true },
  { key: 'cline', label: 'Cline', color: '#53617A', icon: '/agents/cline.svg', glyph: true, mono: true },
  { key: 'codebuddy', label: 'CodeBuddy', color: '#1677FF', icon: '/agents/codebuddy-color.svg', glyph: true },
  // Pi Coding Agent 与 Inflection Pi 是不同产品，不借用同名品牌图标。
  { key: 'pi', label: 'Pi', color: '#5661A8', icon: null },
  { key: 'pi-desktop', label: 'Pi Desktop', color: '#5661A8', icon: null },
  { key: 'omp', label: 'oh-my-pi', color: '#5661A8', icon: null },
  { key: 'dsh', label: 'DeepSeek Harness', color: '#4D6BFE', icon: '/agents/deepseek-color.svg', glyph: true },
  { key: 'alma', label: 'Alma', color: '#7B6AC8', icon: null },
]

const BY_KEY = new Map(AGENT_LIST.map((a) => [a.key, a]))

export function agentIdentity(agent: string): AgentIdentity {
  return BY_KEY.get(agent) ?? { key: agent, label: agent, color: '#758198', icon: null }
}
