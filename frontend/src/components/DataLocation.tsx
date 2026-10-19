// 数据存储位置：查看占用、迁移到其他目录、清理旧位置。
//
// 迁移的实际动作全在 Go 侧（internal/app/migrate.go）：目标目录为空、空间
// 足够、逐个文件校验通过才会写指针；旧位置一律保留，清理是另行确认的动作。
// 这里只负责如实展示与确认，不自己拼路径。
import { Button, Dialog, DialogActions, DialogBody, DialogContent, DialogSurface, DialogTitle, Spinner, Text } from '@fluentui/react-components'
import { ArrowClockwiseRegular, ArrowResetRegular, ArrowUploadRegular, DeleteRegular } from '@fluentui/react-icons'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import type { CleanupPlan, StorageView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { storageProgressFromEvent, type StorageProgress } from '../eventPayload'
import { pickOpenPath } from '../nativeDialogs'
import { fmtBytes, fmtPercent } from '../format'

export function DataLocation() {
  const [view, setView] = useState<StorageView | null>(null)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  // pending 是已选好、等待确认的迁移目标目录。
  const [pending, setPending] = useState('')
  const [progress, setProgress] = useState<StorageProgress | null>(null)
  const [cleanup, setCleanup] = useState<CleanupPlan | null>(null)
  const [restartNeeded, setRestartNeeded] = useState(false)
  const load = useCallback(() => {
    settingsService.Storage().then(setView).catch(reason => setError(`读取数据位置失败：${String(reason)}`))
  }, [])
  useEffect(load, [load])
  useEffect(() => Events.On('storage:progress', (payload: unknown) => {
    const next = storageProgressFromEvent(payload)
    if (next) setProgress(next)
  }), [])
  const choose = async () => {
    setError('')
    setMessage('')
    try {
      const dir = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: '选择新的数据存储位置', ButtonText: '迁到此目录' })
      if (!dir) return
      setPending(dir)
    } catch (reason) {
      setError(`选择目录失败：${String(reason)}`)
    }
  }
  const migrate = async () => {
    if (!pending || busy) return
    setBusy('migrate')
    setError('')
    setMessage('')
    setProgress(null)
    try {
      const report = await settingsService.Migrate(pending)
      setPending('')
      setRestartNeeded(true)
      setMessage(report.message)
      load()
    } catch (reason) {
      setPending('')
      setError(`迁移未完成：${String(reason)}`)
      load()
    } finally {
      setBusy('')
      setProgress(null)
    }
  }
  const preview = async () => {
    if (busy) return
    setError('')
    setMessage('')
    try {
      setCleanup(await settingsService.CleanupPreview())
    } catch (reason) {
      setError(`读取旧位置失败：${String(reason)}`)
    }
  }
  const cleanOld = async () => {
    if (!cleanup || busy) return
    setBusy('cleanup')
    setError('')
    setMessage('')
    try {
      const report = await settingsService.CleanupOld(cleanup.token)
      setCleanup(null)
      setMessage(report.message)
      load()
    } catch (reason) {
      setCleanup(null)
      setError(`清理未完成：${String(reason)}`)
      load()
    } finally {
      setBusy('')
    }
  }
  const reset = async () => {
    if (busy) return
    setBusy('reset')
    setError('')
    setMessage('')
    try {
      await settingsService.ResetRoot()
      setRestartNeeded(true)
      setMessage('数据位置已改回默认位置，重启 AgentDeck 后生效。')
      load()
    } catch (reason) {
      setError(`未能改回默认位置：${String(reason)}`)
    } finally {
      setBusy('')
    }
  }
  const restart = async () => {
    setError('')
    try {
      await settingsService.Restart()
    } catch (reason) {
      setError(String(reason))
    }
  }
  const copy = (view?.items ?? []).filter(item => item.present && !item.rebuild)
  const skipped = (view?.items ?? []).filter(item => item.rebuild)
  return <section className="ad-storage-section" aria-label="数据存储位置">
    <h3>数据存储位置</h3>
    <p>AgentDeck 自己的数据库与导入副本。原生 Agent 的会话仍保存在它们各自的目录里，不随这里迁移。</p>
    {view?.note ? <Text block role="alert" className="ad-storage-warn">{view.note}</Text> : null}
    {restartNeeded ? <Text block role="status" className="ad-storage-restart">数据位置已改变，重启后生效。
      <Button size="small" appearance="primary" onClick={() => void restart()}>立即重启</Button></Text> : null}
    {view ? <>
      <dl className="ad-storage-summary">
        <dt>当前位置</dt><dd>{view.using_default ? '默认位置' : view.root}</dd>
        {view.using_default ? <><dt>数据目录</dt><dd>{view.config_dir}</dd><dt>本地目录</dt><dd>{view.local_dir}</dd></> : null}
        <dt>合计占用</dt><dd>{fmtBytes(view.total_bytes)}</dd>
        <dt>磁盘剩余</dt><dd>{view.free_bytes > 0 ? fmtBytes(view.free_bytes) : '未知'}</dd>
      </dl>
      <table className="ad-storage-table">
        <thead><tr><th>数据</th><th>位置</th><th>占用</th><th>说明</th></tr></thead>
        <tbody>
          {view.items.filter(item => item.present).map(item => <tr key={item.path}>
            <td>{item.name}</td>
            <td className="ad-storage-path">{item.path}</td>
            <td>{fmtBytes(item.bytes)}</td>
            <td>{item.note}</td>
          </tr>)}
          {view.items.every(item => !item.present) ? <tr><td colSpan={4}>当前还没有数据文件。</td></tr> : null}
        </tbody>
      </table>
      <div className="ad-storage-actions">
        <Button size="small" icon={busy === 'migrate' ? <Spinner size="extra-tiny" /> : <ArrowUploadRegular />} disabled={!!busy} onClick={() => void choose()}>迁移到其他位置…</Button>
        {!view.using_default ? <Button size="small" appearance="subtle" icon={busy === 'reset' ? <Spinner size="extra-tiny" /> : <ArrowResetRegular />} disabled={!!busy} onClick={() => void reset()}>改回默认位置</Button> : null}
        <Button size="small" appearance="subtle" icon={<ArrowClockwiseRegular />} disabled={!!busy} onClick={load}>更新统计</Button>
      </div>
      <p>迁移只复制 AgentDeck 自己的数据，不移动原生 Agent 的目录。全文索引{skipped.some(item => item.rebuild && item.present) ? `（当前 ${fmtBytes(skipped.reduce((sum, item) => sum + (item.present ? item.bytes : 0), 0))}）` : ''}可重建，不参与复制，重启后自动重建。</p>
      <p>迁移过程中请勿关闭应用。旧位置的数据一律保留，确认无误后再清理。</p>
    </> : <Spinner size="tiny" />}
    {progress ? <Text block role="status" className="ad-storage-progress">
      <Spinner size="extra-tiny" />{stageLabel(progress.stage)}{progress.total > 0 ? ` ${fmtPercent(progress.done, progress.total)}` : ''}{progress.message ? ` · ${progress.message}` : ''}
    </Text> : null}
    {message ? <Text block role="status" className="ad-storage-message">{message}</Text> : null}
    {error ? <Text block role="alert" className="ad-storage-error">{error}</Text> : null}
    {view && (view.prev_dirs?.length ?? 0) > 0 ? <section className="ad-storage-old">
      <h4>之前的数据位置</h4>
      <p>以下位置在迁移前使用，数据仍然保留。确认新位置运行正常后可以清理。</p>
      <ul>{view.prev_dirs.map(dir => <li key={dir}><span className="ad-storage-path">{dir}</span></li>)}</ul>
      {view.prev_items.length ? <p>可清理：{view.prev_items.map(item => item.name).join('、')}，合计 {fmtBytes(view.prev_bytes)}。</p> : <p>该位置已没有可清理的 AgentDeck 数据。</p>}
      {view.prev_in_use ? <p>旧位置仍是本次运行使用的目录，请先重启 AgentDeck，确认新位置运行正常后再清理。</p> : null}
      <Button size="small" appearance="subtle" icon={<DeleteRegular />} disabled={!!busy || !view.prev_items.length || view.prev_in_use} onClick={() => void preview()}>清理旧位置数据…</Button>
    </section> : null}
    <Dialog open={!!pending} onOpenChange={(_, data) => { if (!data.open && !busy) setPending('') }}>
      <DialogSurface><DialogBody>
        <DialogTitle>迁移数据存储位置</DialogTitle>
        <DialogContent>
          <p>将把以下数据复制到新位置：</p>
          <ul>{copy.map(item => <li key={item.path}>{item.name}（{fmtBytes(item.bytes)}）</li>)}</ul>
          <p>目标目录：<span className="ad-storage-path">{pending}</span></p>
          {copy.length === 0 ? <p>当前还没有可迁移的数据文件。</p> : null}
          <p>目标目录必须是空目录。完成后需要重启 AgentDeck 才会使用新位置；旧位置的数据不会被删除。</p>
        </DialogContent>
        <DialogActions>
          <Button disabled={!!busy} onClick={() => setPending('')}>取消</Button>
          <Button appearance="primary" disabled={!!busy} icon={busy === 'migrate' ? <Spinner size="extra-tiny" /> : undefined} onClick={() => void migrate()}>{busy === 'migrate' ? '正在迁移…' : '开始迁移'}</Button>
        </DialogActions>
      </DialogBody></DialogSurface>
    </Dialog>
    <Dialog open={!!cleanup} onOpenChange={(_, data) => { if (!data.open && !busy) setCleanup(null) }}>
      <DialogSurface><DialogBody>
        <DialogTitle>清理旧位置数据</DialogTitle>
        <DialogContent>
          <p>将删除以下位置中的 AgentDeck 数据文件，共 {cleanup?.files ?? 0} 个文件（{fmtBytes(cleanup?.bytes ?? 0)}）：</p>
          <ul>{(cleanup?.dirs ?? []).map(dir => <li key={dir}><span className="ad-storage-path">{dir}</span></li>)}</ul>
          <p>应用配置文件、原生 Agent 的目录与旧位置中的其他条目不会被删除，也不会删除非空目录。</p>
          {cleanup?.other?.length ? <p>不会被删除的条目：{cleanup.other.join('、')}</p> : null}
        </DialogContent>
        <DialogActions>
          <Button disabled={!!busy} onClick={() => setCleanup(null)}>取消</Button>
          <Button appearance="primary" disabled={!!busy} icon={busy === 'cleanup' ? <Spinner size="extra-tiny" /> : undefined} onClick={() => void cleanOld()}>{busy === 'cleanup' ? '正在清理…' : '清理旧位置数据'}</Button>
        </DialogActions>
      </DialogBody></DialogSurface>
    </Dialog>
  </section>
}

// stageLabel 把迁移阶段译成中文说明。
function stageLabel(stage: string): string {
  switch (stage) {
    case 'scan': return '正在统计待迁移数据…'
    case 'copy': return '正在复制数据…'
    case 'verify': return '正在校验副本…'
    case 'switch': return '正在切换数据位置…'
    case 'done': return '迁移完成'
    case 'failed': return '迁移失败'
    default: return '正在处理…'
  }
}