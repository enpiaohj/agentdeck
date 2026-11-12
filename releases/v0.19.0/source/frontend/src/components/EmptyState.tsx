// 空状态：图标 + 一句结论 + 怎么继续。列表区、项目区、搜索结果共用一套排版，
// 免得每种空态各写一版间距和字号。
import type { ReactNode } from 'react'

export function EmptyState({ icon, title, children, compact }: { icon: ReactNode; title: string; children?: ReactNode; compact?: boolean }) {
  return <div className={compact ? 'ad-empty ad-empty-compact' : 'ad-empty'} role="status">
    <span className="ad-empty-icon" aria-hidden="true">{icon}</span>
    <span className="ad-empty-title">{title}</span>
    {children ? <span className="ad-empty-hint">{children}</span> : null}
  </div>
}