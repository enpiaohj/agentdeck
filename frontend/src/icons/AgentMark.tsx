import { useState } from 'react'
import { CodeBlockRegular } from '@fluentui/react-icons'
import { tokens } from '@fluentui/react-components'
import { agentIdentity } from './agents'

type Props = { agent: string; size?: number; title?: string }

// 统一轮廓和光学留白。故障状态与资源路径绑定，切换 Agent 不沿用旧失败。
export function AgentMark({ agent, size = 20, title }: Props) {
  const id = agentIdentity(agent)
  const [brokenIcon, setBrokenIcon] = useState<string | null>(null)
  const showImage = id.icon !== null && brokenIcon !== id.icon
  const label = title ?? id.label
  return (
    <span
      role="img" aria-label={label} title={label}
      style={{
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        width: size, height: size, flexShrink: 0,
        borderRadius: Math.max(3, Math.round(size * 0.24)),
        backgroundColor: showImage ? (id.background ?? '#ffffff') : tokens.colorNeutralBackground3,
        boxShadow: showImage ? `inset 0 0 0 1px ${tokens.colorNeutralStroke3}, 0 1px 2px #1425410a` : undefined,
        color: id.color, overflow: 'hidden', userSelect: 'none',
      }}
    >
      {showImage ? (
        <img
          src={id.icon!} alt="" width={size} height={size}
          style={{ width: size, height: size, objectFit: 'contain', display: 'block', padding: id.glyph ? Math.max(1, Math.round(size * 0.1)) : 0 }}
          onError={() => setBrokenIcon(id.icon)}
        />
      ) : <CodeBlockRegular aria-hidden="true" fontSize={Math.round(size * 0.66)} />}
    </span>
  )
}
