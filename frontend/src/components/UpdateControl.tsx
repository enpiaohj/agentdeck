import { Button, ProgressBar, Spinner } from '@fluentui/react-components'
import { ArrowDownloadRegular, ArrowSyncRegular, DismissRegular, FolderOpenRegular, OpenRegular } from '@fluentui/react-icons'
import { Browser, Events } from '@wailsio/runtime'
import { useEffect, useState, useSyncExternalStore } from 'react'
import * as updateService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/updateservice'
import type { UpdateView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { matchesSetting } from '../settingsSearch'
import { createUpdateSettings } from '../updateSettings'
import { FormDialog } from './FormDialog'
import { SettingRow } from './SettingRow'

// 后端在检查与下载过程中推送完整视图（含进度）。
const subscribeStatus = (handler: (view: UpdateView) => void) =>
  Events.On('update:status', payload => {
    const view = payload.data as UpdateView | null
    if (view) handler(view)
  })

const versionDescription = '当前安装的版本。检查更新会联网读取 GitHub 上的最新正式发布。'
const autoDescription = '启动后在后台检查一次新版本，同一个版本每天最多打扰一次。关闭后仍可手动检查。'
export const matchesUpdateSettings = (query: string) => matchesSetting(query, '更新 版本 检查更新 新版本 升级 安装 重启 取消下载 自动检查', versionDescription, autoDescription)

// 时间按本地时区显示到分钟：发布与检查时间只需要能对上，不需要秒。
function formatTime(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// UpdateStage 是界面的显式状态：只有一处判断，按钮与提示都跟着它走，
// 避免「按钮可点但其实必然失败」这类靠字段各自为政推导出来的组合。
type UpdateStage = 'loading' | 'checking' | 'downloading' | 'ready' | 'available' | 'upToDate' | 'dev' | 'idle'

function stageOf(view: UpdateView | null, loading: boolean): UpdateStage {
  if (loading && !view) return 'loading'
  if (!view) return 'idle'
  if (view.checking) return 'checking'
  if (view.downloading) return 'downloading'
  if (view.ready_to_restart) return 'ready'
  if (view.available) return 'available'
  if (view.dev && view.checked) return 'dev'
  if (view.checked) return 'upToDate'
  return 'idle'
}

export function UpdateControl({ id }: { id: string }) {
  const [settings] = useState(() => createUpdateSettings(updateService, { subscribe: subscribeStatus }))
  const { view, loading, error } = useSyncExternalStore(settings.subscribe, settings.getSnapshot)
  const [confirmRestart, setConfirmRestart] = useState(false)
  useEffect(() => {
    void settings.load()
    return settings.watch()
  }, [settings])
  const stage = stageOf(view, loading)
  const busy = stage === 'loading' || stage === 'checking' || stage === 'downloading'
  const version = view?.dev ? '开发版本' : view?.current ? `v${view.current}` : '读取中…'
  const checkedAt = formatTime(view?.checked_at ?? '')
  const publishedAt = formatTime(view?.published_at ?? '')
  const notes = view?.notes?.trim() ?? ''
  const percent = view ? Math.round(view.progress * 100) : 0
  // 后端的错误（落地失败、自动检查失败）与本地错误都要显示：之前只渲染本地那条，
  // 「上次退出时未能完成更新」这类后端提示等于没写。
  const message = error || view?.error || ''
  const releasePage = view?.page_url || view?.release_page_url || ''
  const checkButton = <Button id={`${id}-check`} size="small" disabled={busy} onClick={() => void settings.check()}>
    {stage === 'checking' ? '正在检查…' : '检查更新'}
  </Button>
  return <div className="ad-update-settings" aria-busy={busy}>
    <div className="ad-setting-card">
      <SettingRow id={`${id}-version`} title="当前版本" description={versionDescription}>
        <div className="ad-update-version">
          <span className="ad-update-current">{version}</span>
          {checkButton}
        </div>
      </SettingRow>
      <SettingRow id={`${id}-auto`} title="启动后自动检查" description={autoDescription}>
        <input id={`${id}-auto`} type="checkbox" role="switch" aria-label="启动后自动检查" checked={view?.auto_check ?? true}
          aria-describedby={`${id}-auto-description`} disabled={busy || !view} onChange={event => void settings.setAutoCheck(event.target.checked)} />
      </SettingRow>
    </div>

    {stage === 'checking' ? <div className="ad-update-status" role="status"><Spinner size="extra-tiny" />正在检查新版本…</div> : null}

    {stage === 'downloading' ? <div className="ad-update-download" role="status">
      <ProgressBar value={view?.progress ? view.progress : undefined} />
      <div className="ad-update-progress-line">
        <span>{percent > 0 ? `正在下载并校验新版本 v${view?.latest}：${percent}%` : `正在连接，准备下载 v${view?.latest}…`}</span>
        <Button id={`${id}-cancel`} size="small" appearance="subtle" icon={<DismissRegular />} onClick={() => void settings.cancelDownload()}>取消下载</Button>
      </div>
      <p className="ad-settings-note">下载期间可以继续使用，取消不会留下半成品。</p>
    </div> : null}

    {stage === 'available' ? <div className="ad-update-found">
      <div className="ad-update-found-head">
        <strong>发现新版本 v{view?.latest}</strong>
        <span>{publishedAt ? `发布于 ${publishedAt}` : ''}</span>
      </div>
      {notes ? <div className="ad-update-notes" tabIndex={0}>{notes}</div> : null}
      <div className="ad-update-actions">
        {view?.can_install
          ? <Button id={`${id}-download`} size="small" appearance="primary" icon={<ArrowDownloadRegular />} disabled={busy || !view.downloadable} onClick={() => void settings.download()}>下载并安装</Button>
          : <Button id={`${id}-release`} size="small" appearance="primary" icon={<OpenRegular />} onClick={() => void Browser.OpenURL(releasePage)}>到发布页下载</Button>}
        {view?.can_install && releasePage ? <Button size="small" appearance="subtle" icon={<OpenRegular />} onClick={() => void Browser.OpenURL(releasePage)}>打开发布页</Button> : null}
      </div>
      {view?.can_install
        ? <p className="ad-settings-note">下载完成后会校验文件完整性，再提示重启安装。</p>
        : <p className="ad-settings-note">当前运行方式无法在应用内安装（例如通过 go run 或测试二进制启动），请到发布页手动下载。</p>}
    </div> : null}

    {stage === 'ready' ? <div className="ad-update-downloaded">
      <strong>新版本 v{view?.latest} 已下载并校验</strong>
      {view?.target_path ? <p className="ad-update-path">{view.target_path}</p> : null}
      <div className="ad-update-actions">
        <Button id={`${id}-restart`} size="small" appearance="primary" icon={<ArrowSyncRegular />} disabled={busy} onClick={() => setConfirmRestart(true)}>重启并安装</Button>
        <Button size="small" icon={<FolderOpenRegular />} onClick={() => void settings.openFolder()}>打开所在文件夹</Button>
      </div>
      <p className="ad-settings-note">点「重启并安装」会关闭 AgentDeck、写入新版本并重新打开。也可以直接退出：新版本会在退出时写入并把快捷方式与开机启动改指向它，下次启动即为新版本。正在运行的会话请先在终端里结束。</p>
    </div> : null}

    {stage === 'upToDate' ? <div className="ad-update-status" role="status">
      已是最新版本{checkedAt ? `（检查于 ${checkedAt}）` : ''}
    </div> : null}

    {stage === 'dev' ? <div className="ad-update-status" role="status">
      当前是开发构建，无法与发布版本比较{view?.latest ? `；GitHub 上最新的正式版是 v${view.latest}` : ''}。
    </div> : null}

    {message ? <div className="ad-update-error" role="alert">
      <span>{message}</span>
      {!view ? <Button size="small" onClick={() => void settings.load()}>重试读取</Button>
        : stage === 'downloading' ? null
        : <Button size="small" onClick={() => void settings.check()}>重新检查</Button>}
      {releasePage ? <Button size="small" appearance="subtle" icon={<OpenRegular />} onClick={() => void Browser.OpenURL(releasePage)}>打开下载页</Button> : null}
    </div> : null}

    <FormDialog open={confirmRestart} title="重启以完成更新"
      intro={<p className="ad-update-confirm-intro">AgentDeck 会先关闭，再写入新版本并重新打开。</p>}
      submitLabel="立即重启" busyLabel="正在重启…" busy={false}
      onSubmit={() => { setConfirmRestart(false); void settings.restart() }}
      onClose={() => setConfirmRestart(false)} returnFocus={`#${id}-restart`}>
      <p className="ad-settings-note">正在终端里运行的会话不受影响；下次打开时是 v{view?.latest}。若这次写入失败，下次启动会提示，并保留原版本可用。</p>
    </FormDialog>

    <p className="ad-settings-note">更新包来自 GitHub Releases，下载后按发布提供的 SHA-256 校验完整性。项目未做代码签名，校验只说明文件与发布记录一致，不能证明发布者身份。</p>
  </div>
}