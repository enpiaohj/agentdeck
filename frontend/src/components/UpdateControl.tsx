import { Button, ProgressBar, Spinner } from '@fluentui/react-components'
import { ArrowDownloadRegular, ArrowSyncRegular, CheckmarkCircleFilled, DismissRegular, ErrorCircleFilled, FolderOpenRegular, InfoFilled, OpenRegular, ArrowCircleDownFilled } from '@fluentui/react-icons'
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

const autoDescription = '启动后在后台检查一次新版本，同一个版本每天最多打扰一次。关闭后仍可手动检查。'
export const matchesUpdateSettings = (query: string) => matchesSetting(query, '更新 版本 检查更新 新版本 升级 安装 重启 取消下载 下载并更新 立即重启 自动检查 更新内容 发布页', autoDescription)

// 时间按本地时区显示到分钟：发布与检查时间只需要能对上，不需要秒。
function formatTime(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// UpdateStage 是界面的显式状态：只有一处判断，标题、按钮与提示都跟着它走，
// 避免「按钮可点但其实必然失败」这类靠字段各自为政推导出来的组合。
type UpdateStage = 'loading' | 'checking' | 'downloading' | 'ready' | 'available' | 'upToDate' | 'dev' | 'idle'

export function stageOf(view: UpdateView | null, loading: boolean): UpdateStage {
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

type Tone = 'ok' | 'accent' | 'neutral' | 'error'

// 版式：顶部是一块「状态头」——图标、一句话结论、一句补充，按钮在右侧；其下是
// 键值信息列表（当前版本、最新版本、检查时间、更新方式），再往下是更新内容与
// 自动检查开关。切换状态只换头部的文字、图标与按钮，下面的信息行位置不动。
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
  const latestVersion = view?.latest ? `v${view.latest}` : ''
  const latest = latestVersion || '新版本'
  const checkedAt = formatTime(view?.checked_at ?? '')
  const publishedAt = formatTime(view?.published_at ?? '')
  const notes = view?.notes?.trim() ?? ''
  const percent = view ? Math.round(view.progress * 100) : 0
  // 后端的错误（落地失败、自动检查失败）与本地错误都要显示：之前只渲染本地那条，
  // 「上次退出时未能完成更新」这类后端提示等于没写。
  const message = error || view?.error || ''
  const releasePage = view?.page_url || view?.release_page_url || ''
  const installable = view?.can_install ?? false

  let tone: Tone = 'neutral'
  let icon: ReactNode = <InfoFilled />
  let title: ReactNode = '尚未检查更新'
  let detail = '点击「检查更新」，联网读取 GitHub 上的最新正式发布。'
  if (stage === 'loading') { title = '正在读取更新状态…'; detail = '' }
  else if (stage === 'checking') { icon = <Spinner size="tiny" />; title = '正在检查更新…'; detail = '正在联网读取 GitHub 上的最新正式发布。' }
  else if (stage === 'downloading') {
    tone = 'accent'; icon = <ArrowCircleDownFilled />
    title = `正在下载 ${latest}${percent > 0 ? `　${percent}%` : ''}`
    detail = '下载完成后按发布提供的 SHA-256 校验，可随时取消。'
  } else if (stage === 'ready') {
    tone = 'ok'; icon = <CheckmarkCircleFilled />
    title = `${latest} 已下载并校验`
    detail = '重启后生效；直接退出也会在退出时写入，下次启动即为新版本。'
  } else if (stage === 'available') {
    tone = 'accent'; icon = <ArrowCircleDownFilled />
    title = `发现新版本 ${latest}`
    detail = installable ? (publishedAt ? `发布于 ${publishedAt}` : '可下载并校验后更新。') : '当前运行方式不支持应用内安装，请到发布页手动下载。'
  } else if (stage === 'upToDate') {
    tone = 'ok'; icon = <CheckmarkCircleFilled />
    title = '已是最新版本'
    detail = checkedAt ? `上次检查于 ${checkedAt}` : ''
  } else if (stage === 'dev') {
    title = '当前是开发构建'
    detail = `无法与发布版本比较${latestVersion ? `；GitHub 上最新的正式版是 ${latestVersion}` : ''}。`
  }
  // 取消下载后后端只是回到「有新版本」，取消这件事没有别的痕迹，用一条中性提示说出来。
  const aside = message ? '' : stage === 'available' ? notice : ''
  if (message) { tone = 'error'; icon = <ErrorCircleFilled /> }
  const showNotes = !!notes && (stage === 'available' || stage === 'downloading' || stage === 'ready' || !!message)
  const method = !installable ? '手动下载（到发布页）' : '应用内下载，SHA-256 校验，退出时写入'

  return <div className="ad-update-settings" aria-busy={busy}>
    <div className="ad-setting-card">
      <div className="ad-update-head" data-tone={tone}>
        <span className="ad-update-icon" aria-hidden="true">{icon}</span>
        <div className="ad-update-headline">
          <h3 role="status" aria-live="polite">{title}</h3>
          {detail ? <p>{detail}</p> : null}
        </div>
        <div className="ad-update-actions">
          {stage !== 'ready' ? <Button id={`${id}-check`} size="small" appearance={stage === 'available' || stage === 'downloading' ? 'secondary' : 'primary'} icon={<ArrowSyncRegular />}
            disabled={busy} onClick={() => void settings.check()}>{stage === 'checking' ? '正在检查…' : '检查更新'}</Button> : null}
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
        </div>
      </div>
      {stage === 'downloading' ? <div className="ad-update-progress"><ProgressBar value={view?.progress ? view.progress : undefined} /></div> : null}
      {message ? <p className="ad-update-alert" role="alert">{message}</p> : null}
      {aside ? <p className="ad-update-aside" role="status">{aside}</p> : null}

      <dl className="ad-info-list">
        <div><dt>当前版本</dt><dd>{version}</dd></div>
        <div><dt>最新版本</dt><dd>{latestVersion || '—'}{publishedAt && latestVersion ? <span className="ad-info-sub">发布于 {publishedAt}</span> : null}</dd></div>
        <div><dt>上次检查</dt><dd>{checkedAt || '尚未检查'}</dd></div>
        <div><dt>更新方式</dt><dd>{method}</dd></div>
        {releasePage ? <div><dt>发布页</dt><dd><button className="ad-link" onClick={() => void Browser.OpenURL(releasePage)}>{releasePage}</button></dd></div> : null}
        {stage === 'ready' && view?.target_path ? <div><dt>写入位置</dt><dd className="ad-info-path">{view.target_path}</dd></div> : null}
      </dl>

      {showNotes ? <div className="ad-update-notes-wrap">
        <h4>{latestVersion ? `${latestVersion} 更新内容` : '更新内容'}</h4>
        <div className="ad-update-notes" tabIndex={0}>{notes}</div>
      </div> : null}

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
