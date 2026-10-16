// AgentDeck 图标体系 —— Agent 标识注册表(唯一数据源)
//
// 每个 Agent 一条身份记录:展示名、识别色、真实品牌图标。
//   icon  指向 public/agents/<key>.png,由 tools/gen_agenticons.ps1 从本机
//         已安装的对应产品的图标资源里提取(见 icons/README.md)。
//         icon 为 null 表示本机没有该产品的官方图标资源,界面走兜底色块。
//   color 兜底色块底色,同时用于无图标时的首字母徽标。有真实图标时它只是
//         兜底,不参与渲染。
//
// 新增 Agent:在 AGENT_LIST 里加一条;若本机装了对应产品,同时在
// tools/gen_agenticons.ps1 的 $sources 里追加候选路径并重跑。

export type AgentIdentity = {
  key: string
  label: string
  color: string
  icon: string | null
}

// KNOWN:本机已提取到真实品牌图标的 Agent。
const KNOWN: AgentIdentity[] = [
  { key: 'claude', label: 'Claude Code', color: '#D97757', icon: '/agents/claude.png' },
  { key: 'claude-desktop', label: 'Claude Desktop', color: '#D97757', icon: '/agents/claude-desktop.png' },
  { key: 'codex', label: 'Codex', color: '#412991', icon: '/agents/codex.png' },
  { key: 'zcode', label: 'ZCode', color: '#4C7DF0', icon: '/agents/zcode.png' },
  { key: 'hermes', label: 'Hermes', color: '#B08A2E', icon: '/agents/hermes.png' },
  { key: 'workbuddy', label: 'WorkBuddy', color: '#0FA37F', icon: '/agents/workbuddy.png' },
  { key: 'qoder-cn', label: 'Qoder CN', color: '#6E56CF', icon: '/agents/qoder-cn.png' },
  // OpenCode:本机既无桌面端也无官方图标资源(仅有一个装完后就没再用的
  // 更新器安装包),不拿安装包自带图标冒充品牌标志,先走兜底色块。
  { key: 'opencode', label: 'OpenCode', color: '#B06AD4', icon: null },
]

// 其余支持的 Agent:本机未安装(或未见过会话),没有可提取的官方图标,
// 用识别色 + 首字母兜底。这些颜色只用于区分,不代表产品配色。
const OTHERS: AgentIdentity[] = [
  { key: 'gemini', label: 'Gemini', color: '#4285F4', icon: null },
  { key: 'grok', label: 'Grok', color: '#1F1F1F', icon: null },
  { key: 'kimi', label: 'Kimi', color: '#2B2B45', icon: null },
  { key: 'minimax-code', label: 'MiniMax Code', color: '#E8455F', icon: null },
  { key: 'cursor', label: 'Cursor', color: '#0A0A0A', icon: null },
  { key: 'pi', label: 'Pi', color: '#5661A8', icon: null },
  { key: 'pi-desktop', label: 'Pi Desktop', color: '#5661A8', icon: null },
]

export const AGENT_LIST: AgentIdentity[] = [...KNOWN, ...OTHERS]

const BY_KEY = new Map(AGENT_LIST.map((a) => [a.key, a]))

// agentMeta 查注册表;未登记的 Agent 用 key 本身作展示名 + 中性色,
// 界面上仍能一眼区分(首字母不同)。
export function agentIdentity(agent: string): AgentIdentity {
  const hit = BY_KEY.get(agent)
  if (hit) return hit
  return {
    key: agent,
    label: agent,
    color: '#8A8A8A',
    icon: null,
  }
}
