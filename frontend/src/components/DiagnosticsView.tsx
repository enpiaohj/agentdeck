// 诊断页(UI-08/UI-09 合并):Agent 能力表、索引状态与重建、
// 标注库备份与恢复、崩溃恢复提示。
import {
  makeStyles,
  mergeClasses,
  tokens,
  Text,
  Button,
  Spinner,
} from '@fluentui/react-components'
import { ArrowClockwiseRegular } from '@fluentui/react-icons'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'
import { DataBackup } from './DataBackup'
import { DataLocation } from './DataLocation'
import type { DiagRow, IndexStatus, LibraryStatus } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as searchService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import { storageProgressFromEvent, type StorageProgress } from '../eventPayload'
import { fmtBytes as formatBytes, fmtElapsed, fmtPercent } from '../format'
import { useIndexStatus } from '../hooks/useIndexStatus'
import { agentLabel, relTime } from '../util'

const useStyles = makeStyles({
  wrap: { height: '100%', overflowY: 'auto', padding: '28px', boxSizing: 'border-box', backgroundColor: tokens.colorNeutralBackground2 },
  content: { maxWidth: '1100px', margin: '0 auto' },
  section: { marginBottom: '16px', padding: '20px', border: `1px solid ${tokens.colorNeutralStroke2}`, borderRadius: '8px', backgroundColor: tokens.colorNeutralBackground1 },
  tableWrap: { overflowX: 'auto' },
  h: { display: 'block', fontSize: '14px', fontWeight: 600, marginBottom: '10px' },
  table: { width: '100%', minWidth: '680px', borderCollapse: 'collapse', fontSize: '13px' },
  th: {
    textAlign: 'left',
    padding: '6px 10px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke2}`,
    color: tokens.colorNeutralForeground3,
    fontWeight: 600,
  },
  td: { padding: '8px 10px', borderBottom: `1px solid ${tokens.colorNeutralStroke3}`, verticalAlign: 'top', fontVariantNumeric: 'tabular-nums' },
  ok: { color: tokens.colorPaletteGreenForeground1 },
  warn: { color: tokens.colorPaletteYellowForeground1 },
  row: { display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '8px', flexWrap: 'wrap' },
  msg: { fontSize: '12px', color: tokens.colorNeutralForeground3 },
  err: { fontSize: '12px', color: tokens.colorPaletteRedForeground1 },
})

export function DiagnosticsView({ revision = 0, embedded = false }: { revision?: number; embedded?: boolean }) {
  const styles = useStyles()
  const Heading = embedded ? 'h2' : 'h1'
  const [rows, setRows] = useState<DiagRow[] | null>(null)
  const [library, setLibrary] = useState<LibraryStatus | null>(null)
  const [libraryError, setLibraryError] = useState('')
  const status = useIndexStatus()
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  // 压缩历史库是几十秒到几分钟的原地重写：进度经 storage:progress 推来
  // （stage=compact），期间禁掉按钮，避免并发压同一批行。
  const [compactBusy, setCompactBusy] = useState('')
  const [compactDone, setCompactDone] = useState('')
  const [compactProgress, setCompactProgress] = useState<StorageProgress | null>(null)

  const load = useCallback(() => {
    sessionService.Diagnostics().then(setRows).catch((e) => setErr(String(e)))
    sessionService.LibraryStatus().then(value => { setLibrary(value); setLibraryError('') }).catch(reason => setLibraryError(String(reason)))
  }, [])

  useEffect(() => {
    load()
  }, [load, revision])

  // 索引升级（旧库换成 contentless FTS）也走同一个事件，进度显示在全文
  // 索引那一节；这里只认压缩。
  const [upgradeProgress, setUpgradeProgress] = useState<StorageProgress | null>(null)
  useEffect(() => Events.On('storage:progress', (payload: unknown) => {
    const next = storageProgressFromEvent(payload)
    if (!next) return
    if (next.stage === 'compact') setCompactProgress(next)
    else if (next.stage === 'index-upgrade') setUpgradeProgress(next)
  }), [])

  // 「重建索引」是用户显式要求的整库重建（RebuildFull）；启动与刷新走的
  // 是增量路径（Rebuild），不会清库。
  const rebuild = () => {
    setErr('')
    void searchService.RebuildFull().then(() => setMsg('正在整库重建全文索引，进度显示在底部状态栏。')).catch(reason => setErr(`索引重建未能启动：${String(reason)}`))
  }

  // 压缩只改存储形态：正文逐行解压比对后才写回，失败会在 Go 侧中止并
  // 报出原因，界面照实显示。
  const compact = () => {
    setLibraryError('')
    setCompactDone('')
    setCompactProgress(null)
    setCompactBusy('正在压缩历史库…')
    sessionService.CompactLibrary()
      .then(report => {
        setCompactDone(`压缩完成：${formatBytes(report.before)} → ${formatBytes(report.after)}（共 ${report.versions} 个版本，用时 ${fmtElapsed(report.elapsed)}）`)
        load()
      })
      .catch(reason => setLibraryError(`压缩失败：${String(reason)}`))
      .finally(() => setCompactBusy(''))
  }

  return (
    <div className={styles.wrap} style={embedded ? { height:'auto',overflow:'visible',padding:0,background:'transparent' } : undefined}><div className={styles.content}>
      <Heading style={{ fontSize: embedded ? 17 : 20, margin: '0 0 8px' }}>{embedded ? '数据与诊断' : '设置'}</Heading>
      <p className={styles.msg} style={{ margin: '0 0 24px' }}>查看数据来源、维护全文索引和管理本机标注。</p>
      <DataBackup onRestored={load} />
      <DataLocation />
      <div className={styles.section}>
        <Text className={styles.h}>永久会话库</Text>
        <div className={styles.row}>
          <Text size={300}>默认开启 · 不自动清理</Text>
          <Button size="small" appearance="subtle" icon={<ArrowClockwiseRegular />} onClick={load}>更新统计</Button>
          <Button size="small" appearance="subtle" onClick={compact} disabled={!!compactBusy}>压缩历史库</Button>
        </div>
        {library ? <>
          <Text block size={300}>{library.sessions} 个会话 · {library.versions} 个内容版本 · 正文约 {formatBytes(library.bytes)}</Text>
          <Text block className={styles.msg} style={{ marginTop: 8, overflowWrap: 'anywhere', userSelect: 'text' }}>{library.path}</Text>
          {library.error ? <Text block role="alert" className={styles.err}>{library.error}</Text> : null}
        </> : libraryError ? <Text block role="alert" className={styles.err}>{libraryError}</Text> : <Spinner size="extra-tiny" />}
        {compactBusy ? <Text block role="status" className={styles.msg} style={{ marginTop: 8 }}>
          {compactBusy}
          {compactProgress && compactProgress.total > 0 ? ` ${fmtPercent(compactProgress.done, compactProgress.total)}` : ''}
          {compactProgress ? ` · 已用 ${fmtElapsed(compactProgress.elapsed)}` : ''}
        </Text> : null}
        {compactDone ? <Text block role="status" className={styles.ok} style={{ marginTop: 8 }}>{compactDone}</Text> : null}
        <Text block className={styles.msg} style={{ marginTop: 8 }}>启动、刷新后的索引构建及打开正文时自动保存成功读取的内容。相同正文去重，变化保留版本；上游清理后仍可阅读、搜索和导出。重建索引不删除永久库。</Text>
        <Text block className={styles.msg} style={{ marginTop: 8 }}>压缩历史库只重新编码正文（原始内容不变，逐行校验过），并回收文件空间；可以随时重复执行。</Text>
      </div>
      <div className={styles.section}>
        <Text className={styles.h}>Agent 能力诊断</Text>
        <Text className={styles.msg} style={{ display: 'block', marginBottom: '8px' }}>
          诊断结果基于当前扫描数据。新增记录需返回工作台后单击“刷新”。
        </Text>
        {err ? <Text className={styles.err}>{err}</Text> : null}
        {rows === null ? (
          <Spinner size="tiny" />
        ) : rows.length === 0 ? (
          <Text className={styles.msg}>未发现 Agent 会话。使用受支持的 Agent 创建会话后，请返回工作台重新扫描。</Text>
        ) : (
          <div className={styles.tableWrap}><table className={styles.table}>
            <thead>
              <tr>
                <th className={styles.th}>Agent</th>
                <th className={styles.th}>发现</th>
                <th className={styles.th}>会话数</th>
                <th className={styles.th}>最近活动</th>
                <th className={styles.th}>原生恢复</th>
                <th className={styles.th}>说明</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.agent}>
                  <td className={styles.td}>{agentLabel(r.agent)}{r.source === 'wsl' ? '(WSL)' : ''}</td>
                  <td className={mergeClasses(styles.td, r.found && styles.ok)}>{r.found ? '已发现' : '未发现'}</td>
                  <td className={styles.td}>{r.sessions}</td>
                  <td className={styles.td}>{relTime(r.last_at)}</td>
                  <td className={mergeClasses(styles.td, r.resume ? styles.ok : styles.warn)}>
                    {r.resume ? '可用' : '仅可查看'}
                  </td>
                  <td className={styles.td}>{r.resume_note}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
      </div>

      <div className={styles.section}>
        <Text className={styles.h}>全文索引</Text>
        <div className={styles.row}>
          <IndexLine status={status} />
          <Button size="small" icon={<ArrowClockwiseRegular />} onClick={rebuild}>重建索引</Button>
        </div>
        {upgradeProgress && upgradeProgress.done < upgradeProgress.total ? <Text block className={styles.msg} style={{ marginBottom: 8 }}>
          正在把旧索引改成不重复存正文的形态：{upgradeProgress.done} / {upgradeProgress.total} 段（只需一次）
        </Text> : null}
        <Text className={styles.msg}>重建期间搜索自动降级为快速扫描；永久会话库及收藏 / 标签 / 备注不受影响。</Text>
      </div>

      {msg ? <Text className={styles.msg}>{msg}</Text> : null}
      {err ? <Text className={styles.err}>{err}</Text> : null}
    </div></div>
  )
}

function IndexLine({ status }: { status: IndexStatus | null }) {
  if (!status) return <Spinner size="extra-tiny" />
  switch (status.state) {
    case 'ready': return <Text size={300}>索引就绪：{status.sessions} 会话 / {status.docs} 段</Text>
    case 'building': return <Text size={300}><Spinner size="extra-tiny" /> 正在构建：{status.sessions}{status.total ? ` / ${status.total}` : ''} 会话…</Text>
    case 'failed': return <Text size={300} style={{ color: tokens.colorPaletteRedForeground1 }}>索引构建失败：{status.error}（当前使用逐会话扫描）</Text>
    default: return <Text size={300}>索引未开始</Text>
  }
}
