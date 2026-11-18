import type { ReactNode } from 'react'

export function SettingRow({ id, title, description, children }: {
  id: string; title: string; description: string; children: ReactNode
}) {
  return <div className="ad-setting-row">
    <div className="ad-setting-copy"><label htmlFor={id}>{title}</label><p id={`${id}-description`}>{description}</p></div>
    <div className="ad-setting-control">{children}</div>
  </div>
}
