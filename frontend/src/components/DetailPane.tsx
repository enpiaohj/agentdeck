// 右栏:会话详情。头部字段 + transcript 全文(按需加载)+ 收藏/归档/标签。
import {
  makeStyles,
  tokens,
  Text,
  Button,
  Spinner,
  Input,
} from '@fluentui/react-components'
import {
  DismissRegular,
  ArrowClockwiseRegular,
  ArchiveRegular,
  ArchiveArrowBackRegular,
  StarFilled,
  StarRegular,
  TagRegular,
} from '@fluentui/react-icons'
import { useCallback, useEffect, useState } from 'react'
import { Dialogs } from '@wailsio/runtime'
import type { SessionView, TranscriptView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentLabel, relTime } from '../util'

const useStyles = makeStyles({
  wrap: { height: '100%', width: '480px', display: 'flex', flexDirection: 'column', backgroundColor: tokens.colorNeutralBackground1 },
  head: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
    padding: '12px 14px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke2}`,
  },
  headTop: { display: 'flex', alignItems: 'flex-start', gap: '8px' },
  title: { flex: 1, minWidth: 0, overflowWrap: 'anywhere' },
  metaGrid: { display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '2px 10px', fontSize: '12px', color: tokens.colorNeutralForeground3 },
  actions: { display: 'flex', gap: '8px', marginTop: '4px' },
  tagRow: { display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap', marginTop: '2px' },
  tagChip: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '2px',
    fontSize: '11px',
    padding: '1px 6px',
    borderRadius: '4px',
    backgroundColor: tokens.colorNeutralBackground4,
  },
  tagRemove: { border: 'none', background: 'transparent', cursor: 'pointer', padding: '0 2px', color: tokens.colorNeutralForeground3 },
  iconBtn: {
    border: 'none', background: 'transparent', cursor: 'pointer',
    display: 'flex', alignItems: 'center', padding: '4px', color: tokens.colorNeutralForeground3,
  },
  body: { flex: 1, overflowY: 'auto', padding: '10px 14px' },
  part: {
    marginBottom: '10px',
    padding: '8px 10px',
    borderRadius: '6px',
    backgroundColor: tokens.colorNeutralBackground2,
    whiteSpace: 'pre-wrap',
    overflowWrap: 'anywhere',
    fontSize: '13px',
    lineHeight: 1.55,
  },
  roleTag: { fontSize: '11px', color: tokens.colorNeutralForeground3, marginBottom: '4px', display: 'block' },
  center: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' },
})

type Props = {
  session: SessionView | null
  onClose: () => void
  onChanged: () => void // 标注变化后让列表重拉
}

export function DetailPane({ session, onClose, onChanged }: Props) {
  const styles = useStyles()
  if (!session) {
    return (
      <div className={styles.wrap}>
        <div className={styles.center}>
          <Text size={300} style={{ color: tokens.colorNeutralForeground3 }}>选择左侧会话查看详情</Text>
        </div>
      </div>
    )
  }
  return <Loaded key={session.key} session={session} onClose={onClose} onChanged={onChanged} />
}

function Loaded({ session, onClose, onChanged }: { session: SessionView; onClose: () => void; onChanged: () => void }) {
  const styles = useStyles()
  const [tr, setTr] = useState<TranscriptView | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [tagDraft, setTagDraft] = useState('')
  const [exportMsg, setExportMsg] = useState('')

  const exportMd = () => {
    Dialogs.OpenFile({
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: true,
      Title: '选择 Markdown 导出目录',
      ButtonText: '在此导出',
    })
      .then((dir) => {
        if (!dir || typeof dir !== 'string') return
        sessionService.ExportMarkdown(session.key, dir).then(
          (p) => setExportMsg(`已导出:${p}`),
          (e) => setExportMsg(`导出失败:${shortErr(e)}`),
        )
      })
      .catch(() => {})
  }

  const load = useCallback(() => {
    setLoading(true)
    setErr('')
    sessionService
      .Transcript(session.key)
      .then(setTr)
      .catch((e) => setErr(shortErr(e)))
      .finally(() => setLoading(false))
  }, [session.key])

  useEffect(() => {
    // 会话不可恢复也可能有 transcript;加载失败仅详情降级
    load()
  }, [load])

  const toggleStar = () => {
    markService.SetFavorite(session.key, !session.favorite).then(onChanged).catch(() => {})
  }
  const toggleArchive = () => {
    markService.SetArchived(session.key, !session.archived).then(onChanged).catch(() => {})
  }
  const addTag = () => {
    const t = tagDraft.trim()
    if (!t || session.tags.includes(t)) { setTagDraft(''); return }
    markService.SetTags(session.key, [...session.tags, t]).then(() => { setTagDraft(''); onChanged() }).catch(() => {})
  }
  const removeTag = (t: string) => {
    markService.SetTags(session.key, session.tags.filter((x) => x !== t)).then(onChanged).catch(() => {})
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.head}>
        <div className={styles.headTop}>
          <Text className={styles.title} weight="semibold" size={400}>{session.title || '(无标题)'}</Text>
          <button
            className={styles.iconBtn}
            onClick={toggleStar}
            title={session.favorite ? '取消收藏' : '收藏'}
            style={{ color: session.favorite ? tokens.colorStatusWarningForeground3 : undefined }}
          >
            {session.favorite ? <StarFilled fontSize={16} /> : <StarRegular fontSize={16} />}
          </button>
          <Button appearance="subtle" icon={<DismissRegular />} onClick={onClose} title="收起" />
        </div>
        <div className={styles.metaGrid}>
          <span>Agent</span><span>{agentLabel(session.agent)}</span>
          <span>最后活动</span><span>{relTime(session.last)}</span>
          <span>消息数</span><span>{session.messages}</span>
          <span>项目</span><span>{session.project || '项目未知'}</span>
          {session.cwd ? (<><span>工作目录</span><span style={{ overflowWrap: 'anywhere' }}>{session.cwd}</span></>) : null}
          {session.tokens > 0 ? (<><span>Token</span><span>{session.tokens.toLocaleString()}</span></>) : null}
        </div>
        <div className={styles.tagRow}>
          <TagRegular fontSize={14} />
          {session.tags.map((t) => (
            <span key={t} className={styles.tagChip}>
              {t}
              <button className={styles.tagRemove} onClick={() => removeTag(t)} title={`移除标签 ${t}`}>×</button>
            </span>
          ))}
          <Input
            size="small"
            placeholder="添加标签,回车确认"
            value={tagDraft}
            onChange={(_, d) => setTagDraft(d.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') addTag() }}
            style={{ width: 140 }}
          />
        </div>
        <div className={styles.actions}>
          {session.resume ? (
            <>
              <Button size="small" onClick={() => void resumeService.CopyToClipboard(session.key)} title="复制恢复命令">复制恢复命令</Button>
              <Button size="small" appearance="primary" onClick={() => void resumeService.OpenInTerminal(session.key)} title="打开终端执行恢复">在终端打开</Button>
            </>
          ) : (
            <Text size={200} style={{ color: tokens.colorPaletteYellowForeground1 }}>该会话不可恢复,仅可查看历史</Text>
          )}
          <Button
            size="small"
            icon={session.archived ? <ArchiveArrowBackRegular /> : <ArchiveRegular />}
            onClick={toggleArchive}
            title={session.archived ? '从归档恢复' : '归档(仅列表隐藏,不动源文件)'}
          >
            {session.archived ? '取消归档' : '归档'}
          </Button>
          <Button size="small" onClick={exportMd} title="导出会话为 Markdown 文件">导出 Markdown</Button>
        </div>
        {exportMsg ? <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>{exportMsg}</Text> : null}
      </div>
      <div className={styles.body}>
        {loading ? (
          <div className={styles.center}><Spinner size="tiny" /></div>
        ) : err ? (
          <div>
            <Text size={300} style={{ color: tokens.colorPaletteRedForeground1 }}>{err}</Text>
            <Button size="small" icon={<ArrowClockwiseRegular />} onClick={load}>重试</Button>
          </div>
        ) : tr ? (
          tr.parts.map((p, i) =>
            p.kind === 'thinking' || p.role === 'thinking' ? (
              <details key={i} className={styles.part}>
                <summary className={styles.roleTag} style={{ cursor: 'pointer' }}>
                  {roleLabel(p.role, p.kind)}(思考过程,点击展开){p.model ? ` · ${p.model}` : ''}
                </summary>
                {p.text}
              </details>
            ) : (
              <div key={i} className={styles.part}>
                <span className={styles.roleTag}>{roleLabel(p.role, p.kind)}{p.model ? ` · ${p.model}` : ''}</span>
                {p.text}
              </div>
            ),
          )
        ) : null}
      </div>
    </div>
  )
}

function roleLabel(role: string, kind: string): string {
  if (kind && kind !== 'text') return `${roleLabel(role, '')} · ${kind}`
  switch (role) {
    case 'user': return '用户'
    case 'assistant': return 'Agent'
    case 'system': return '系统'
    default: return role
  }
}

function shortErr(e: unknown): string {
  const s = String(e)
  return s.length > 200 ? s.slice(0, 200) + '…' : s
}
