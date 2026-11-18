import { useState } from 'react'
import { CodeBlockRegular } from '@fluentui/react-icons'
import { tokens } from '@fluentui/react-components'
import { agentIdentity } from './agents'

type Props = { agent: string; size?: number; title?: string }

// 统一轮廓和光学留白。故障状态与资源路径绑定，切换 Agent 不沿用旧失败。
export function AgentMark({ agent, size = 20, title }: Props) {
  const id = agentIdentity(agent)
  const [brokenIcon, setBrokenIcon] = useState<string | null>(null)
  const label = title ?? id.label
  const tileStyle = {
    display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
    width: size, height: size, flexShrink: 0,
    borderRadius: Math.max(3, Math.round(size * 0.24)),
    overflow: 'hidden', userSelect: 'none',
  } as const
  // 满幅品牌底板：单色图形着白，或自绘字母标
  if (id.background && (id.mask || id.monogram)) {
    return (
      <span role="img" aria-label={label} title={label} style={{ ...tileStyle, backgroundColor: id.background }}>
        {id.mask ? (
          <span aria-hidden="true" style={{
            width: Math.round(size * 0.66), height: Math.round(size * 0.66), backgroundColor: '#ffffff',
            maskImage: `url(${id.mask})`, WebkitMaskImage: `url(${id.mask})`,
            maskRepeat: 'no-repeat', WebkitMaskRepeat: 'no-repeat',
            maskPosition: 'center', WebkitMaskPosition: 'center',
            maskSize: 'contain', WebkitMaskSize: 'contain',
          }} />
        ) : (
          <span aria-hidden="true" style={{ color: id.color, fontSize: Math.round(size * 0.62), fontWeight: 800, fontStyle: 'italic', lineHeight: 1, fontFamily: 'Segoe UI, system-ui, sans-serif' }}>{id.monogram}</span>
        )}
      </span>
    )
  }
  // 单色图形：底板与图形都取主题色板（浅色为白底深色图形，深色为深底浅色图形）
  if (id.mono && id.icon) {
    return (
      <span role="img" aria-label={label} title={label} style={{
        ...tileStyle, backgroundColor: tokens.colorNeutralBackground1,
        boxShadow: `inset 0 0 0 1px ${tokens.colorNeutralStroke3}, 0 1px 2px #1425410a`,
      }}>
        <span aria-hidden="true" style={{
          width: Math.round(size * 0.78), height: Math.round(size * 0.78), backgroundColor: tokens.colorNeutralForeground1,
          maskImage: `url(${id.icon})`, WebkitMaskImage: `url(${id.icon})`,
          maskRepeat: 'no-repeat', WebkitMaskRepeat: 'no-repeat',
          maskPosition: 'center', WebkitMaskPosition: 'center',
          maskSize: 'contain', WebkitMaskSize: 'contain',
        }} />
      </span>
    )
  }
  const showImage = id.icon !== null && brokenIcon !== id.icon
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
