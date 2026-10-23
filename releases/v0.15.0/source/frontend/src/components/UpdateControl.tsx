import { Button, ProgressBar, Spinner } from '@fluentui/react-components'
import { ArrowDownloadRegular, ArrowSyncRegular, FolderOpenRegular, OpenRegular } from '@fluentui/react-icons'
import { Browser, Events } from '@wailsio/runtime'
import { useEffect, useState, useSyncExternalStore } from 'react'
import * as updateService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/updateservice'
import type { UpdateView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { matchesSetting } from '../settingsSearch'
import { createUpdateSettings } from '../updateSettings'
import { SettingRow } from './SettingRow'

// 后端在检查与下载过程中推送完整视图（含进度）。
const subscribeStatus = (handler: (view: UpdateView) => void) =>
  Events.On('update:status', payload => {
    const view = payload.data as UpdateView | null
    if (view) handler(view)
  })

const versionDescription = '当前安装的版本。检查更新会联网读取 GitHub 上的最新正式发布。'
const autoDescription = '启动后在后台检查一次新版本，同一个版本每天最多打扰一次。关闭后仍可手动检查。'
export const matchesUpdateSettings = (query: string) => matchesSetting(query, '更新 版本 检查更新 新版本 升级 安装 重启 自动检查', versionDescription, autoDescription)

// 时间按本地时区显示到分钟：发布与检查时间只需要能对上，不需要秒。
function formatTime(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export function UpdateControl({ id }: { id: string }) {
  const [settings] = useState(() => createUpdateSettings(updateService, { subscribe: subscribeStatus }))
  const { view, loading, error } = useSyncExternalStore(settings.subscribe, settings.getSnapshot)
  useEffect(() => {
    void settings.load()
    return settings.watch()
  }, [settings])
  const busy = loading || !!view?.checking || !!view?.downloading
  const checking = loading || !!view?.checking
  const version = view?.dev ? '开发版本' : view?.current ? `v${view.current}` : '读取中…'
  const checkedAt = formatTime(view?.checked_at ?? '')
  const publishedAt = formatTime(view?.published_at ?? '')
  return <div className="ad-update-settings" aria-busy={busy}>
    <div className="ad-setting-card">
      <SettingRow id={`${id}-version`} title="当前版本" description={versionDescription}>
        <div className="ad-update-version">
          <span className="ad-update-current">{version}</span>
          <Button id={`${id}-check`} size="small" disabled={busy} onClick={() => void settings.check()}>{checking ? '正在检查…' : '检查更新'}</Button>
        </div>
      </SettingRow>
      <SettingRow id={`${id}-auto`} title="启动后自动检查" description={autoDescription}>
        <input id={`${id}-auto`} type="checkbox" role="switch" aria-label="启动后自动检查" checked={view?.auto_check ?? true}
          aria-describedby={`${id}-auto-description`} disabled={busy || !view} onChange={event => void settings.setAutoCheck(event.target.checked)} />
      </SettingRow>
    </div>

    {checking ? <div className="ad-update-status" role="status"><Spinner size="extra-tiny" />正在检查新版本…</div> : null}

    {view?.downloading ? <div className="ad-update-download" role="status">
      <ProgressBar value={view.progress || undefined} />
      <span>{view.progress > 0 ? `正在下载并校验：${Math.round(view.progress * 100)}%` : '正在下载并校验新版本…'}</span>
    </div> : null}

    {view?.available && !view.downloading && !view.ready_to_restart ? <div className="ad-update-found">
      <div className="ad-update-found-head">
        <strong>发现新版本 v{view.latest}</strong>
        <span>{publishedAt ? `发布于 ${publishedAt}` : ''}</span>
      </div>
      {view.notes ? <pre className="ad-update-notes">{view.notes}</pre> : null}
      <div className="ad-update-actions">
        <Button size="small" appearance="primary" icon={<ArrowDownloadRegular />} disabled={busy || !view.downloadable} onClick={() => void settings.download()}>{view.can_install ? '下载并准备安装' : '下载到本地'}</Button>
        <Button size="small" appearance="subtle" icon={<OpenRegular />} onClick={() => void Browser.OpenURL(view.page_url || view.release_page_url)}>打开发布页面</Button>
      </div>
    </div> : null}

    {view?.ready_to_restart ? <div className="ad-update-downloaded">
      <strong>新版本 v{view.latest} 已下载并校验</strong>
      {view.target_path ? <p className="ad-update-path">{view.target_path}</p> : null}
      <div className="ad-update-actions">
        <Button size="small" appearance="primary" icon={<ArrowSyncRegular />} disabled={busy} onClick={() => void settings.restart()}>重启并安装</Button>
        <Button size="small" icon={<FolderOpenRegular />} onClick={() => void settings.openFolder()}>打开所在文件夹</Button>
      </div>
      <p className="ad-settings-note">点「重启并安装」会关闭 AgentDeck、写入新版本并重新打开。也可以直接退出：新版本会在退出时写入，下次启动即为新版本。正在运行的会话请先在终端里结束。</p>
    </div> : null}

    {view?.checked && !view.dev && !view.available && !view.downloading ? <div className="ad-update-status" role="status">
      已是最新版本{checkedAt ? `（检查于 ${checkedAt}）` : ''}
    </div> : null}

    {view?.dev && view.checked ? <div className="ad-update-status" role="status">
      当前是开发构建，无法与发布版本比较{view.latest ? `；GitHub 上最新的正式版是 v${view.latest}` : ''}。
    </div> : null}

    {error ? <div className="ad-update-error" role="alert">{error}{view ? null : <Button size="small" onClick={() => void settings.load()}>重试读取</Button>}</div> : null}

    <p className="ad-settings-note">更新包来自 GitHub Releases，下载后按 SHA256SUMS 校验完整性。项目未做代码签名，校验只说明文件与发布清单一致，不能证明发布者身份。</p>
  </div>
}
