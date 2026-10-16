// AgentMark —— Agent 的统一标识:真实品牌图标铺满圆角方片。
//
// 图标体系约定(见 icons/README.md):
//   圆角半径 = size / 4,与 AgentDeck 自身应用图标的圆角比例一致;
//   图标本身是方片(品牌自带的底色/留白),我们用 CSS 圆角把它收进同一套
//   形状语言里,不在图标上再加边框或描边。
//
// 兜底:注册表里没有图标、或 PNG 加载失败(资源缺失/路径变了)时,退化为
// 识别色方片 + 首字母,界面不会出现破图。
import { useState } from 'react'
import { agentIdentity } from './agents'

type Props = {
  agent: string
  size?: number
  title?: string
}

export function AgentMark({ agent, size = 20, title }: Props) {
  const id = agentIdentity(agent)
  const [broken, setBroken] = useState(false)
  const radius = Math.max(4, Math.round(size / 4))
  const label = title ?? id.label

  if (id.icon && !broken) {
    return (
      <img
        src={id.icon}
        alt={label}
        title={label}
        width={size}
        height={size}
        style={{
          width: size,
          height: size,
          borderRadius: radius,
          objectFit: 'cover',
          flexShrink: 0,
          userSelect: 'none',
          display: 'block',
        }}
        onError={() => setBroken(true)}
      />
    )
  }

  return (
    <span
      title={label}
      aria-label={label}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        borderRadius: radius,
        backgroundColor: id.color,
        color: '#fff',
        fontSize: Math.round(size * 0.55),
        fontWeight: 700,
        lineHeight: 1,
        flexShrink: 0,
        userSelect: 'none',
      }}
    >
      {(id.label[0] ?? '?').toUpperCase()}
    </span>
  )
}
