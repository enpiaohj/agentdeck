// 数据存储位置：查看占用、迁移到其他目录、清理旧位置。
//
// 迁移的实际动作全在 Go 侧（internal/app/migrate.go）：目标目录为空、空间
// 足够、逐个文件校验通过才会写指针；旧位置一律保留，清理是另行确认的动作。
// 这里只负责如实展示与确认，不自己拼路径——用户选的是「数据放在哪个目录下」，
// 实际数据根由 Go 侧补全（固定的 AgentDeck 子目录），是否已存在也由 Go 侧判断。
import { Button, Dialog, DialogActions, DialogBody, DialogContent, DialogSurface, DialogTitle, Spinner, Text } from '@fluentui/react-components'
import { ArrowClockwiseRegular, ArrowResetRegular, ArrowSyncRegular, ArrowUploadRegular, DeleteRegular } from '@fluentui/react-icons'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import type { CleanupPlan, DestinationView, StorageView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { storageProgressFromEvent, type StorageProgress } from '../eventPayload'
import { pickOpenPath } from '../nativeDialogs'
import { fmtBytes, fmtElapsed, fmtPercent } from '../format'

export function DataLocation() {
  const [view, setView] = useState<StorageView | null>(null)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  // pending 是已选好目录、等待确认的迁移目标：含补全后的数据位置与它当前的状态。
  const [pending, setPending] = useState<DestinationView | null>(null)
  const [progress, setProgress] = useState<StorageProgress | null>(null)
  const [cleanup, setCleanup] = useState<CleanupPlan | null>(null)
  const [restartNeeded, setRestartNeeded] = useState(false)
  // migrated 是刚完成的迁移报告文案，非空即弹窗的「完成」态。迁移成功后不关
  // 弹窗、就地换成重启提示：否则关闭动画与新的重启弹窗会同时挂在屏幕上。
  const [migrated, setMigrated] = useState('')
  const load = useCallback(() => {
    settingsService.Storage().then(setView).catch(reason => setError(`读取数据位置失败：${String(reason)}`))
  }, [])
  useEffect(load, [load])
  useEffect(() => Events.On('storage:progress', (payload: unknown) => {
    const next = storageProgressFromEvent(payload)
    // 同一个事件还带着索引升级与历史库压缩的进度（诊断页显示）；这里只
    // 认迁移自己的阶段，免得别的进度串进迁移弹窗。
    if (next && next.stage !== 'compact' && next.stage !== 'index-upgrade') setProgress(next)
  }), [])
  const choose = async () => {
    setError('')
    setMessage('')
    setMigrated('')
    setProgress(null)
    try {
      const dir = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: '选择数据存放的目录', ButtonText: '选择此目录' })
      if (!dir) return
      // 由 Go 侧补全数据根并判断那里是否已有目录：界面不自己拼路径。
      setPending(await settingsService.ResolveDestination(dir))
    } catch (reason) {
      setError(`选择目录失败：${String(reason)}`)
    }
  }
  const migrate = async () => {
    if (!pending || busy || !pending.ready) return
    setBusy('migrate')
    setError('')
    setMessage('')
    setMigrated('')
    setProgress(null)
    try {
      const report = await settingsService.Migrate(pending.picked)
      // 成功后就地切到「完成」态：弹窗保持打开，等用户选「立即重启」或「稍后」。
      setMigrated(report.message)
      setRestartNeeded(true)
      setMessage(report.message)
      load()
    } catch (reason) {
      setPending(null)
      setError(`迁移未完成：${String(reason)}`)
      load()
    } finally {
      setBusy('')
      setProgress(null)
    }
  }
  // closeMigration 关闭迁移弹窗（含「取消」与完成后的「稍后」）。
  const closeMigration = () => {
    if (busy) return
    setPending(null)
    setMigrated('')
    setProgress(null)
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
  // restart 走「正常退出再自启」（Go 侧延迟启动），所以失败是可能的：配置根
  // 不可写、单实例互斥未释放等。fromDialog 为真表示这次点击来自完成弹窗——
  // 此时必须先关弹窗，否则错误提示落在被模态遮住的设置区里，用户看不见。
  const restart = async (fromDialog = false) => {
    setError('')
    try {
      await settingsService.Restart()
    } catch (reason) {
      if (fromDialog) {
        setPending(null)
        setMigrated('')
        setProgress(null)
      }
      setError(`未能自动重启：${String(reason)}。请手动退出 AgentDeck 后重新打开，新位置才会生效。`)
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
      <p>迁移只复制 AgentDeck 自己的数据，不移动原生 Agent 的目录。数据位置固定是所选目录下的 {view.leaf} 子目录。全文索引{skipped.some(item => item.rebuild && item.present) ? `（当前 ${fmtBytes(skipped.reduce((sum, item) => sum + (item.present ? item.bytes : 0), 0))}）` : ''}可重建，不参与复制，重启后自动重建。</p>
      <p>迁移过程中请勿关闭应用。旧位置的数据一律保留，确认无误后再清理。</p>
    </> : <Spinner size="tiny" />}
    {/* 迁移进度只画在弹窗里：迁移期间弹窗是模态且不许关闭，设置区被遮在后面，
        画在这里等于没人看得见——这正是「没有进度」的由来之一。 */}
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
    <Dialog open={!!pending} onOpenChange={(_, data) => { if (!data.open) closeMigration() }}>
      <DialogSurface><DialogBody>
        <DialogTitle>{migrated ? '数据迁移完成' : busy === 'migrate' ? '正在迁移数据' : '迁移数据存储位置'}</DialogTitle>
        <DialogContent>
          {migrated ? <>
            <Text block role="status" className="ad-storage-message">{migrated}</Text>
            <p>新位置已经就绪，重启 AgentDeck 后生效。旧位置的数据一律保留，重启并确认新位置运行正常后再到设置里清理。</p>
          </> : busy === 'migrate' ? <>
            <dl className="ad-storage-summary">
              <dt>数据位置</dt><dd><span className="ad-storage-path">{pending?.dest}</span></dd>
            </dl>
            <MigrationProgress progress={progress} />
            <p>迁移期间请勿关闭应用。旧位置的数据不会被删除。</p>
          </> : <>
            <dl className="ad-storage-summary">
              <dt>所选目录</dt><dd><span className="ad-storage-path">{pending?.picked}</span></dd>
              <dt>数据位置</dt><dd><span className="ad-storage-path">{pending?.dest}</span></dd>
            </dl>
            <p>{pending?.rule}</p>
            {pending?.exists && !pending.empty
              ? <Text block role="alert" className="ad-storage-warn">该目录已存在，其中有 {pending.files} 个文件（{fmtBytes(pending.bytes)}）。迁移不会覆盖已有文件：请先清理该目录，或改选其他位置。</Text>
              : <p>{pending?.exists ? '该目录已存在且为空，可以直接使用。' : '该目录当前不存在，迁移时创建。'}</p>}
            <p>将把以下数据复制到新位置：</p>
            <ul>{copy.map(item => <li key={item.path}>{item.name}（{fmtBytes(item.bytes)}）</li>)}</ul>
            {copy.length === 0 ? <p>当前还没有可迁移的数据文件。</p> : null}
            {pending?.reason ? <Text block role="alert" className="ad-storage-error">{pending.reason}</Text> : null}
            <p>完成后需要重启 AgentDeck 才会使用新位置；旧位置的数据不会被删除。</p>
          </>}
        </DialogContent>
        <DialogActions>
          {migrated ? <>
            <Button disabled={!!busy} onClick={closeMigration}>稍后</Button>
            <Button appearance="primary" icon={<ArrowSyncRegular />} onClick={() => void restart(true)}>立即重启</Button>
          </> : <>
            <Button disabled={!!busy} onClick={closeMigration}>取消</Button>
            <Button appearance="primary" disabled={!!busy || !pending?.ready} icon={busy === 'migrate' ? <Spinner size="extra-tiny" /> : undefined} onClick={() => void migrate()}>{busy === 'migrate' ? '正在迁移…' : '开始迁移'}</Button>
          </>}
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

// MigrationProgress 是迁移进度的唯一渲染处：阶段 + 文件计数 + 百分比 + 当前
// 文件 + 已用时间。
//
// 这一行没有转圈图标，是刻意的：Fluent 的 Spinner 根节点是 display:flex，在
// Text（display:block）的普通流里就是块级盒，会独占一整行、并把自己的环居中到
// 那一行的中间——弹窗正中于是悬着一个和任何文字都不相邻的圈。「正在动」由弹窗
// 主按钮上的 Spinner 表示，这一行只负责说清在做什么。
//
// 百分比是按目标文件现有大小估出来的估算值，不是复制回调的精确值：三个库用
// VACUUM INTO 生成一致副本，那是一条 SQL 语句，2 GB 级的会话库要跑几分钟，
// 期间没有任何可挂的回调。所以除了百分比还一定显示已用时间——大小不动的长
// 步骤里，它是「还在推进」的唯一凭据。progress 为 null 是首次心跳到达之前的
// 那一秒，此时不能显示 0%，只能说明在准备。
function MigrationProgress({ progress }: { progress: StorageProgress | null }) {
  if (!progress) return <Text block role="status" className="ad-storage-progress">正在准备迁移…</Text>
  const elapsed = fmtElapsed(progress.elapsed)
  return <Text block role="status" className="ad-storage-progress">
    {stageLabel(progress.stage)}
    {progress.files > 0 ? ` · ${progress.done_files}/${progress.files} 个文件` : ''}
    {progress.total > 0 ? ` ${fmtPercent(progress.done, progress.total)}` : ''}
    {progress.message ? ` · ${progress.message}` : ''}
    {elapsed ? ` · 已用 ${elapsed}` : ''}
  </Text>
}

// stageLabel 把迁移阶段译成中文说明。这里是「阶段 + 细节」的第一段，所以
// 只给名词短语，不加「正在…」与省略号——后面紧跟着文件名、百分比与已用时间。
function stageLabel(stage: string): string {
  switch (stage) {
    case 'copy': return '复制数据'
    case 'verify': return '校验副本'
    case 'switch': return '切换数据位置'
    case 'done': return '迁移完成'
    case 'failed': return '迁移失败'
    case 'compact': return '压缩历史库'
    case 'index-upgrade': return '升级全文索引'
    default: return '正在处理…'
  }
}