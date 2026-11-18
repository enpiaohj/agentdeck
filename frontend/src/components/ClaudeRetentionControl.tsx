import { Button, Spinner } from '@fluentui/react-components'
import { useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import type { ClaudeRetention } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { matchesSetting } from '../settingsSearch'

const description = 'Claude Code 每次启动时会删除超过此天数未修改的会话文件。删除之后，AgentDeck 仍可查看保留的副本，也可以重建后继续；增加保留期限可避免这一步。'
export const matchesClaudeRetention = (query: string) => matchesSetting(query, '会话 Claude Code 保留期 保留期限 保留天数 清理 删除 过期 到期 cleanupPeriodDays 30 天 重建 无法继续', description)

// Claude Code 会话保留期：只读。读取它的 settings.json 里的 cleanupPeriodDays（默认 30 天），
// 告诉用户有哪些会话快到期，并给出可以复制的设置片段——AgentDeck 不去改 Claude 的设置文件。
export function ClaudeRetentionControl() {
  const [view, setView] = useState<ClaudeRetention | null>(null)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  useEffect(() => {
    let active = true
    settingsService.ClaudeRetention().then(next => { if (active) setView(next) }, reason => { if (active) setError(String(reason)) })
    return () => { active = false }
  }, [])
  const copy = async () => {
    if (!view) return
    try {
      if (!navigator.clipboard) throw new Error('剪贴板不可用')
      await navigator.clipboard.writeText(view.snippet)
      setCopied(true); window.setTimeout(() => setCopied(false), 1800)
    } catch (reason) { setError(`复制失败：${String(reason)}`) }
  }
  if (error && !view) return <div className="ad-setting-card"><p className="ad-settings-note" role="alert">{error}</p></div>
  if (!view) return <div className="ad-setting-card"><p className="ad-settings-note" role="status"><Spinner size="extra-tiny" /> 正在读取 Claude Code 的设置…</p></div>
  const left = (days: number) => days <= 0 ? '已到期，下次启动 Claude Code 时将被删除' : `还剩 ${days} 天`
  return <div className="ad-setting-card">
    <div className="ad-setting-row">
      <div className="ad-setting-copy"><label>保留期限</label><p>{description}</p></div>
      <div className="ad-setting-control"><strong>{view.days === 0 ? '不保存' : `${view.days} 天`}</strong>{view.explicit ? '' : view.days === 0 ? '' : '（默认）'}</div>
    </div>
    {view.days === 0 ? <p className="ad-update-alert" role="alert">settings.json 中的 cleanupPeriodDays 被设置为 0：Claude Code 不会保存会话记录。若要保留会话，请将其改为正数。</p> : null}
    {view.error ? <p className="ad-update-alert" role="alert">{view.error}。以下按默认的 30 天估算。</p> : null}
    {view.days > 0 ? <div className="ad-setting-row">
      <div className="ad-setting-copy"><label>即将到期的会话</label>
        <p>{view.checked === 0 ? '没有找到本机的 Claude Code 会话。'
          : view.soon === 0 ? `已检查 ${view.checked} 个本机会话，未来 7 天内没有会话到期。`
            : `已检查 ${view.checked} 个本机会话，其中 ${view.soon} 个将在 7 天内到期${view.unsaved ? `，AgentDeck 尚未保存其中 ${view.unsaved} 个的副本` : '，AgentDeck 均已保存副本'}。`}</p>
        {view.risks.length ? <ul className="ad-settings-note" style={{ margin: '6px 0 0', paddingLeft: 18 }}>
          {view.risks.map(risk => <li key={risk.key}>{risk.title}：{left(risk.days_left)}{risk.saved ? '（已保存副本）' : '（尚未保存副本）'}</li>)}
        </ul> : null}
      </div>
    </div> : null}
    <div className="ad-setting-row">
      <div className="ad-setting-copy"><label>增加保留期限</label>
        <p>在 Claude Code 的设置文件 {view.source} 中添加下面一行（该文件已有 JSON 内容时，请按格式放入），保存后下次启动 Claude Code 时生效。请勿设置为 0：这会使 Claude Code 完全不保存会话。AgentDeck 不会修改此文件。</p>
        <p><code>{view.snippet}</code></p></div>
      <div className="ad-setting-control"><Button size="small" onClick={() => void copy()}>{copied ? '已复制' : '复制此行'}</Button></div>
    </div>
    {error ? <p className="ad-update-alert" role="alert">{error}</p> : null}
  </div>
}
