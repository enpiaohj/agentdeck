// Agent 头像:品牌色圆角方块 + 缩写字符。列表/详情/诊断统一身份标识。
import { agentMeta } from '../util'

type Props = { agent: string; size?: number }

export function AgentAvatar({ agent, size = 20 }: Props) {
  const m = agentMeta(agent)
  return (
    <span
      title={m.label}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        borderRadius: Math.max(4, size / 4),
        backgroundColor: m.color,
        color: '#fff',
        fontSize: Math.round(size * 0.55),
        fontWeight: 700,
        lineHeight: 1,
        flexShrink: 0,
        userSelect: 'none',
      }}
    >
      {m.abbr}
    </span>
  )
}
