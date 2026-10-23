import { Button, ProgressBar, Spinner } from '@fluentui/react-components'
import { ArrowDownloadRegular, ArrowSyncRegular, DismissRegular, FolderOpenRegular, OpenRegular } from '@fluentui/react-icons'
import { Browser, Events } from '@wailsio/runtime'
import { useEffect, useState, useSyncExternalStore, type ReactNode } from 'react'
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
export const matchesUpdateSettings = (query: string) => matchesSetting(query, '更新 版本 检查更新 新版本 升级 安装 重启 取消下载 下载并更新 立即重启 自动检查', versionDescription, autoDescription)

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

// 版式照 RemoteFlow 的「软件更新」区：状态文字与按钮固定在同一行，进度条与更新说明
// 依次贴在下面。切换状态只换这一行的文字与按钮，其余位置不动——否则每换一个状态
// 页面高度就变一次，按钮跟着跑，用户得重新找。
export function UpdateControl({ id }: { id: string }) {
  const [settings] = useState(() => createUpdateSettings(updateService, { subscribe: subscribeStatus }))
  const { view, loading, error, notice } = useSyncExternalStore(settings.subscribe, settings.getSnapshot)
  const [confirmRestart, setConfirmRestart] = useState(false)
  useEffect(() => {
    void settings.load()
    return settings.watch()
  }, [settings])
  const stage = stageOf(view, loading)
  const busy = stage === 'loading' || stage === 'checking' || stage === 'downloading'
  const version = view?.dev ? '开发版本' : view?.current ? `v${view.current}` : '读取中…'
  const latest = view?.latest ? `v${view.latest}` : '新版本'
  const checkedAt = formatTime(view?.checked_at ?? '')
  const publishedAt = formatTime(view?.published_at ?? '')
  const notes = view?.notes?.trim() ?? ''
  const percent = view ? Math.round(view.progress * 100) : 0
  // 后端的错误（落地失败、自动检查失败）与本地错误都要显示：之前只渲染本地那条，
  // 「上次退出时未能完成更新」这类后端提示等于没写。
  const message = error || view?.error || ''
  const releasePage = view?.page_url || view?.release_page_url || ''
  const installable = view?.can_install ?? false

  let status: ReactNode
  if (stage === 'loading') status = '正在读取更新状态…'
  else if (stage === 'checking') status = <><Spinner size="extra-tiny" />正在检查更新…</>
  else if (stage === 'downloading') status = `正在下载并校验 ${latest}…${percent > 0 ? ` ${percent}%` : ''}`
  else if (stage === 'ready') status = `${latest} 已下载并校验，重启后生效；直接退出也会在退出时写入，下次启动即为新版本。`
  else if (stage === 'available') status = installable
    ? `发现新版本 ${latest}${publishedAt ? `（发布于 ${publishedAt}）` : ''}。`
    : `发现新版本 ${latest}。当前运行方式不支持应用内安装，请到发布页手动下载。`
  else if (stage === 'upToDate') status = `已是最新版本${checkedAt ? `（检查于 ${checkedAt}）` : ''}。`
  else if (stage === 'dev') status = `当前是开发构建，无法与发布版本比较${view?.latest ? `；GitHub 上最新的正式版是 ${latest}` : ''}。`
  else status = '尚未检查更新。'
  // 取消下载后后端只是回到「有新版本」，取消这件事没有别的痕迹，用一条中性提示说出来。
  const line = message || (stage === 'available' && notice ? notice : status)
  const showNotes = !!notes && (stage === 'available' || stage === 'downloading' || stage === 'ready' || !!message)

  return <div className="ad-update-settings" aria-busy={busy}>
    <div className="ad-setting-card">
      <SettingRow id={`${id}-version`} title="当前版本" description={versionDescription}>
        <span className="ad-update-current">{version}</span>
      </SettingRow>

      <div className="ad-update-live">
        <div className="ad-update-live-row">
          <p className={message ? 'ad-update-state ad-update-state-error' : 'ad-update-state'}
            role={message ? 'alert' : 'status'} aria-live="polite">{line}</p>
          <div className="ad-update-actions">
            {stage !== 'ready' ? <Button id={`${id}-check`} size="small" disabled={busy} onClick={() => void settings.check()}>
              {stage === 'checking' ? '正在检查…' : '检查更新'}
            </Button> : null}
            {stage === 'available' && installable ? <Button id={`${id}-download`} size="small" appearance="primary" icon={<ArrowDownloadRegular />}
              disabled={!view?.downloadable} onClick={() => void settings.download()}>下载并更新</Button> : null}
            {stage === 'available' && !installable && releasePage ? <Button size="small" appearance="primary" icon={<OpenRegular />}
              onClick={() => void Browser.OpenURL(releasePage)}>到发布页下载</Button> : null}
            {stage === 'downloading' ? <Button id={`${id}-cancel`} size="small" appearance="subtle" icon={<DismissRegular />}
              onClick={() => void settings.cancelDownload()}>取消下载</Button> : null}
            {stage === 'ready' ? <>
              <Button id={`${id}-restart`} size="small" appearance="primary" icon={<ArrowSyncRegular />}
                disabled={busy} onClick={() => setConfirmRestart(true)}>立即重启</Button>
              <Button size="small" icon={<FolderOpenRegular />} onClick={() => void settings.openFolder()}>打开所在文件夹</Button>
            </> : null}
            {releasePage && (stage === 'available' || stage === 'downloading' || stage === 'ready' || !!message)
              ? <Button size="small" appearance="subtle" icon={<OpenRegular />} onClick={() => void Browser.OpenURL(releasePage)}>打开下载页</Button>
              : null}
          </div>
        </div>
        {stage === 'downloading' ? <ProgressBar value={view?.progress ? view.progress : undefined} /> : null}
        {stage === 'ready' && view?.target_path ? <p className="ad-update-path">{view.target_path}</p> : null}
        {showNotes ? <div className="ad-update-notes" tabIndex={0}>{notes}</div> : null}
      </div>

      <SettingRow id={`${id}-auto`} title="启动后自动检查" description={autoDescription}>
        <input id={`${id}-auto`} type="checkbox" role="switch" aria-label="启动后自动检查" checked={view?.auto_check ?? true}
          aria-describedby={`${id}-auto-description`} disabled={busy || !view} onChange={event => void settings.setAutoCheck(event.target.checked)} />
      </SettingRow>
    </div>

    <FormDialog open={confirmRestart} title="重启以完成更新"
      intro={<p className="ad-update-confirm-intro">AgentDeck 会先关闭，再写入新版本并重新打开。</p>}
      submitLabel="立即重启" busyLabel="正在重启…" busy={false}
      onSubmit={() => { setConfirmRestart(false); void settings.restart() }}
      onClose={() => setConfirmRestart(false)} returnFocus={`#${id}-restart`}>
      <p className="ad-settings-note">正在终端里运行的会话不受影响；下次打开时是 {latest}。若这次写入失败，下次启动会提示，并保留原版本可用。</p>
    </FormDialog>

    <p className="ad-settings-note">更新包来自 GitHub Releases，下载后按发布提供的 SHA-256 校验完整性。项目未做代码签名，校验只说明文件与发布记录一致，不能证明发布者身份。</p>
  </div>
}