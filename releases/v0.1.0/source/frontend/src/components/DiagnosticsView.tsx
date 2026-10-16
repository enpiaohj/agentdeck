// 诊断页(UI-08/UI-09 合并):Agent 能力表、索引状态与重建、
// 标注库备份与恢复、崩溃恢复提示。
import {
  makeStyles,
  tokens,
  Text,
  Button,
  Spinner,
} from '@fluentui/react-components'
import { ArrowClockwiseRegular, DatabaseRegular, ArrowUploadRegular } from '@fluentui/react-icons'
import { useCallback, useEffect, useState } from 'react'
import { Dialogs } from '@wailsio/runtime'
import type { DiagRow, IndexStatus } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import * as searchService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import { useIndexStatus } from '../hooks/useIndexStatus'
import { agentLabel, relTime } from '../util'

const useStyles = makeStyles({
  wrap: { height: '100%', overflowY: 'auto', padding: '16px 20px', boxSizing: 'border-box' },
  section: { marginBottom: '24px' },
  h: { fontSize: '15px', fontWeight: 600, marginBottom: '10px' },
  table: { width: '100%', borderCollapse: 'collapse', fontSize: '13px' },
  th: {
    textAlign: 'left',
    padding: '6px 10px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke2}`,
    color: tokens.colorNeutralForeground3,
    fontWeight: 600,
  },
  td: { padding: '8px 10px', borderBottom: `1px solid ${tokens.colorNeutralStroke3}`, verticalAlign: 'top' },
  ok: { color: tokens.colorPaletteGreenForeground1 },
  warn: { color: tokens.colorPaletteYellowForeground1 },
  row: { display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '8px', flexWrap: 'wrap' },
  msg: { fontSize: '12px', color: tokens.colorNeutralForeground3 },
  err: { fontSize: '12px', color: tokens.colorPaletteRedForeground1 },
})

export function DiagnosticsView() {
  const styles = useStyles()
  const [rows, setRows] = useState<DiagRow[] | null>(null)
  const status = useIndexStatus()
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    sessionService.Diagnostics().then(setRows).catch((e) => setErr(String(e)))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const rebuild = () => {
    searchService.Rebuild()
    setMsg('已开始后台重建全文索引,进度见左栏状态')
  }

  const backup = () => {
    Dialogs.OpenFile({
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: true,
      Title: '选择备份保存目录',
      ButtonText: '备份到这里',
    })
      .then((dir) => {
        if (!dir || typeof dir !== 'string') return
        setMsg(''); setErr('')
        markService.BackupMarks(dir).then(
          (p) => { setMsg(`已备份到:${p}`); load() },
          (e) => setErr(`备份失败:${e}`),
        )
      })
      .catch(() => {})
  }

  const restore = () => {
    Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      Title: '选择标注库备份文件',
      Filters: [{ DisplayName: 'AgentDeck 备份', Pattern: '*.db' }],
    })
      .then((f) => {
        if (!f || typeof f !== 'string') return
        setMsg(''); setErr('')
        markService.RestoreMarks(f).then(
          () => { setMsg('恢复完成,标注已切换到备份时点'); load() },
          (e) => setErr(`恢复失败:${e}`),
        )
      })
      .catch(() => {})
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.section}>
        <Text className={styles.h}>Agent 能力诊断</Text>
        <Text className={styles.msg} style={{ display: 'block', marginBottom: '8px' }}>
          每次打开本页即时验证;能力状态来自本机真实扫描,非宣传表。
        </Text>
        {err ? <Text className={styles.err}>{err}</Text> : null}
        {rows === null ? (
          <Spinner size="tiny" />
        ) : rows.length === 0 ? (
          <Text className={styles.msg}>尚未发现任何 Agent 会话。确认已安装并使用过 Claude Code / Codex / ZCode 等工具后点顶部刷新。</Text>
        ) : (
          <table className={styles.table}>
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
                  <td className={`${styles.td} ${styles.ok}`}>{r.found ? '已发现' : '未发现'}</td>
                  <td className={styles.td}>{r.sessions}</td>
                  <td className={styles.td}>{relTime(r.last_at)}</td>
                  <td className={`${styles.td} ${r.resume ? styles.ok : styles.warn}`}>
                    {r.resume ? '可用' : '仅可查看'}
                  </td>
                  <td className={styles.td}>{r.resume_note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className={styles.section}>
        <Text className={styles.h}>全文索引</Text>
        <div className={styles.row}>
          <IndexLine status={status} />
          <Button size="small" icon={<ArrowClockwiseRegular />} onClick={rebuild}>重建索引</Button>
        </div>
        <Text className={styles.msg}>重建期间搜索自动降级为快速扫描;收藏/标签/备注不受影响。</Text>
      </div>

      <div className={styles.section}>
        <Text className={styles.h}>标注库备份与恢复</Text>
        <div className={styles.row}>
          <Button size="small" icon={<DatabaseRegular />} onClick={backup}>备份标注库…</Button>
          <Button size="small" icon={<ArrowUploadRegular />} onClick={restore}>从备份恢复…</Button>
        </div>
        <Text className={styles.msg}>
          备份含收藏/标签/备注/归档(在线一致性快照,不覆盖已有备份);恢复前会自动留底当前库。
          会话正文为派生索引,可随时重建,不在备份范围。
        </Text>
      </div>

      {msg ? <Text className={styles.msg}>{msg}</Text> : null}
      {err ? <Text className={styles.err}>{err}</Text> : null}
    </div>
  )
}

function IndexLine({ status }: { status: IndexStatus | null }) {
  if (!status) return <Spinner size="extra-tiny" />
  switch (status.state) {
    case 'ready': return <Text size={300}>索引就绪:{status.sessions} 会话 / {status.docs} 段</Text>
    case 'building': return <Text size={300}><Spinner size="extra-tiny" /> 构建中:{status.sessions} 会话…</Text>
    case 'failed': return <Text size={300} style={{ color: tokens.colorPaletteRedForeground1 }}>索引失败:{status.error}(搜索降级为快速扫描)</Text>
    default: return <Text size={300}>索引未开始</Text>
  }
}
