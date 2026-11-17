import { Button, Spinner } from '@fluentui/react-components'
import { useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import type { ClaudeRetention } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { matchesSetting } from '../settingsSearch'

const description = 'Claude Code 每次启动会清理超过这个天数没有改动的会话文件。清理之后 AgentDeck 仍能查看保留的副本，也能重建后继续，但调大保留期可以免去这一步。'
export const matchesClaudeRetention = (query: string) => matchesSetting(query, '会话 Claude Code 保留期 保留天数 清理 过期 cleanupPeriodDays 30 天 重建 已清理 无法继续', description)

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
  const left = (days: number) => days <= 0 ? '已到期，下次启动 Claude Code 时会被清理' : `还剩 ${days} 天`
  return <div className="ad-setting-card">
    <div className="ad-setting-row">
      <div className="ad-setting-copy"><label>保留期</label><p>{description}</p></div>
      <div className="ad-setting-control"><strong>{view.days === 0 ? '不保存' : `${view.days} 天`}</strong>{view.explicit ? '' : view.days === 0 ? '' : '（默认）'}</div>
    </div>
    {view.days === 0 ? <p className="ad-update-alert" role="alert">settings.json 里把 cleanupPeriodDays 设成了 0：Claude Code 不会保存会话记录。想保留会话，请把它改成一个正数。</p> : null}
    {view.error ? <p className="ad-update-alert" role="alert">{view.error}。下面按默认的 30 天估算。</p> : null}
    {view.days > 0 ? <div className="ad-setting-row">
      <div className="ad-setting-copy"><label>快到期的会话</label>
        <p>{view.checked === 0 ? '没有找到本机的 Claude Code 会话。'
          : view.soon === 0 ? `检查了 ${view.checked} 个本机会话，7 天内没有会话到期。`
            : `检查了 ${view.checked} 个本机会话，其中 ${view.soon} 个会在 7 天内到期${view.unsaved ? `，AgentDeck 还没有保存其中 ${view.unsaved} 个的副本` : '，AgentDeck 都已保存副本'}。`}</p>
        {view.risks.length ? <ul className="ad-settings-note" style={{ margin: '6px 0 0', paddingLeft: 18 }}>
          {view.risks.map(risk => <li key={risk.key}>{risk.title}：{left(risk.days_left)}{risk.saved ? '（已保存副本）' : '（尚未保存副本）'}</li>)}
        </ul> : null}
      </div>
    </div> : null}
    <div className="ad-setting-row">
      <div className="ad-setting-copy"><label>调大保留期</label>
        <p>在 Claude Code 的设置文件 {view.source} 里加入下面这一行（已有 JSON 里按格式放进去），保存后下次启动 Claude Code 生效。不要写 0：那会让它完全不保存会话。AgentDeck 不会修改这个文件。</p>
        <p><code>{view.snippet}</code></p></div>
      <div className="ad-setting-control"><Button size="small" onClick={() => void copy()}>{copied ? '已复制' : '复制这一行'}</Button></div>
    </div>
    {error ? <p className="ad-update-alert" role="alert">{error}</p> : null}
  </div>
}
