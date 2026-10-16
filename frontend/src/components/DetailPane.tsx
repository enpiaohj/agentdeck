// 右栏:会话详情。头部(身份/元数据/标签/备注)+ transcript 消息流
// (角色头像 + 分色气泡 + thinking 折叠 + 命中段定位)。
import {
  makeStyles,
  shorthands,
  tokens,
  Text,
  Button,
  Spinner,
  Input,
  Textarea,
  Tooltip,
} from '@fluentui/react-components'
import {
  DismissRegular,
  ArrowClockwiseRegular,
  ArchiveRegular,
  ArchiveArrowBackRegular,
  StarFilled,
  StarRegular,
  TagRegular,
  CopyRegular,
  CheckmarkRegular,
} from '@fluentui/react-icons'
import { useCallback, useEffect, useRef, useState } from 'react'
import { Dialogs } from '@wailsio/runtime'
import type { SessionView, TranscriptView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentMeta, fmtTime, relTime } from '../util'
import { AgentAvatar } from './AgentAvatar'

const useStyles = makeStyles({
  wrap: {
    height: '100%', width: '500px', display: 'flex', flexDirection: 'column',
    backgroundColor: tokens.colorNeutralBackground1, borderLeft: `1px solid ${tokens.colorNeutralStroke2}`,
  },
  head: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    padding: '14px 16px 12px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke2}`,
    backgroundColor: tokens.colorNeutralBackground2,
  },
  headTop: { display: 'flex', alignItems: 'flex-start', gap: '8px' },
  title: { flex: 1, minWidth: 0, overflowWrap: 'anywhere', lineHeight: 1.35 },
  metaGrid: {
    display: 'grid',
    gridTemplateColumns: 'auto 1fr',
    gap: '4px 12px',
    fontSize: '12px',
  },
  metaLabel: { color: tokens.colorNeutralForeground3, whiteSpace: 'nowrap' },
  metaValue: { color: tokens.colorNeutralForeground1, overflowWrap: 'anywhere' },
  actions: { display: 'flex', gap: '6px', marginTop: '2px', flexWrap: 'wrap' },
  tagRow: { display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap' },
  tagChip: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '2px',
    fontSize: '11px',
    padding: '1px 7px',
    borderRadius: '10px',
    backgroundColor: tokens.colorBrandBackground2,
    color: tokens.colorBrandForeground2,
  },
  tagRemove: { border: 'none', background: 'transparent', cursor: 'pointer', padding: '0 2px', color: tokens.colorNeutralForeground3 },
  noteBox: { display: 'flex', flexDirection: 'column', gap: '4px' },
  body: { flex: 1, overflowY: 'auto', padding: '12px 16px' },
  part: {
    marginBottom: '12px',
    ...shorthands.borderRadius('10px'),
    overflowWrap: 'anywhere',
    fontSize: '13px',
    lineHeight: 1.6,
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3),
  },
  partUser: { backgroundColor: tokens.colorBrandBackground2, ...shorthands.borderColor('transparent'), padding: '10px 12px' },
  partAgent: { backgroundColor: tokens.colorNeutralBackground1, padding: '10px 12px' },
  partTool: {
    backgroundColor: tokens.colorNeutralBackground3,
    fontFamily: 'Cascadia Code, Consolas, monospace',
    fontSize: '12px',
    padding: '10px 12px',
  },
  partHead: { display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px' },
  roleName: { fontSize: '12px', fontWeight: 600 },
  partTime: { fontSize: '11px', color: tokens.colorNeutralForeground3, marginLeft: 'auto' },
  code: {
    fontFamily: 'Cascadia Code, Consolas, monospace',
    fontSize: '12px',
    backgroundColor: tokens.colorNeutralBackground3,
    borderRadius: '6px',
    padding: '8px 10px',
    overflowX: 'auto',
    whiteSpace: 'pre',
    margin: '6px 0',
  },
  partHit: { ...shorthands.outline('2px', 'solid', tokens.colorBrandStroke1), backgroundColor: tokens.colorBrandBackground },
  roleTag: { fontSize: '11px', color: tokens.colorNeutralForeground3, display: 'block' },
  copyBtn: {
    border: 'none', background: 'transparent', cursor: 'pointer',
    display: 'flex', alignItems: 'center', padding: '2px', borderRadius: '4px',
    color: tokens.colorNeutralForeground3,
  },
  center: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' },
  grayed: { opacity: 0.55 },
})

type Props = {
  session: SessionView | null
  onClose: () => void
  onChanged: () => void // 标注变化后让列表重拉
  jumpPart?: number // 搜索命中定位到 transcript 第 n 段(-1/undefined = 不定位)
}

export function DetailPane({ session, onClose, onChanged, jumpPart }: Props) {
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
  return <Loaded key={session.key} session={session} onClose={onClose} onChanged={onChanged} jumpPart={jumpPart ?? -1} />
}

function Loaded({ session, onClose, onChanged, jumpPart }: { session: SessionView; onClose: () => void; onChanged: () => void; jumpPart: number }) {
  const styles = useStyles()
  const m = agentMeta(session.agent)
  const [tr, setTr] = useState<TranscriptView | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [tagDraft, setTagDraft] = useState('')
  const [noteDraft, setNoteDraft] = useState(session.note)
  const [exportMsg, setExportMsg] = useState('')
  const [copied, setCopied] = useState('')
  const hitRef = useRef<HTMLElement | null>(null)

  // 搜索定位:transcript 就绪后滚到命中段并高亮(TC-03)
  useEffect(() => {
    if (jumpPart >= 0 && tr && !loading) {
      hitRef.current?.scrollIntoView({ block: 'center' })
    }
  }, [jumpPart, tr, loading])

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
  const saveNote = () => {
    markService.SetNote(session.key, noteDraft).then(onChanged).catch(() => {})
  }

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

  const copyPart = (text: string, i: number) => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(String(i))
      setTimeout(() => setCopied(''), 1500)
    }).catch(() => {})
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.head}>
        <div className={styles.headTop}>
          <AgentAvatar agent={session.agent} size={28} />
          <Text className={styles.title} weight="semibold" size={400}>{session.title || '(无标题)'}</Text>
          <button
            className={styles.copyBtn}
            onClick={toggleStar}
            title={session.favorite ? '取消收藏' : '收藏'}
            style={{ color: session.favorite ? tokens.colorStatusWarningForeground3 : undefined }}
          >
            {session.favorite ? <StarFilled fontSize={16} /> : <StarRegular fontSize={16} />}
          </button>
          <Button appearance="subtle" icon={<DismissRegular />} onClick={onClose} title="收起" />
        </div>
        <div className={styles.metaGrid}>
          <span className={styles.metaLabel}>Agent</span>
          <span className={styles.metaValue}>{m.label}{session.source === 'wsl' ? '(WSL)' : ''}</span>
          <span className={styles.metaLabel}>最后活动</span>
          <span className={styles.metaValue}>{relTime(session.last)} · {fmtTime(session.last)}</span>
          <span className={styles.metaLabel}>创建时间</span>
          <span className={styles.metaValue}>{fmtTime(session.created)}</span>
          <span className={styles.metaLabel}>消息数</span>
          <span className={styles.metaValue}>{session.messages}</span>
          <span className={styles.metaLabel}>项目</span>
          <span className={styles.metaValue}>{session.project || '项目未知'}</span>
          {session.cwd ? (<>
            <span className={styles.metaLabel}>工作目录</span>
            <span className={styles.metaValue}>{session.cwd}</span>
          </>) : null}
          {session.source_path ? (<>
            <span className={styles.metaLabel}>源路径</span>
            <span className={styles.metaValue}>{session.source_path}</span>
          </>) : null}
          {session.tokens > 0 ? (<>
            <span className={styles.metaLabel}>Token</span>
            <span className={styles.metaValue}>{session.tokens.toLocaleString()}</span>
          </>) : null}
        </div>
        <div className={styles.tagRow}>
          <TagRegular fontSize={13} style={{ color: tokens.colorNeutralForeground3 }} />
          {session.tags.map((t) => (
            <span key={t} className={styles.tagChip}>
              {t}
              <button className={styles.tagRemove} onClick={() => removeTag(t)} title={`移除标签 ${t}`}>×</button>
            </span>
          ))}
          <Input
            size="small"
            placeholder="添加标签"
            value={tagDraft}
            onChange={(_, d) => setTagDraft(d.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') addTag() }}
            style={{ width: 110 }}
          />
        </div>
        <div className={styles.noteBox}>
          <Textarea
            size="small"
            placeholder="备注:为这个会话记点什么(只存本机标注库)…"
            value={noteDraft}
            onChange={(_, d) => setNoteDraft(d.value)}
            rows={noteDraft ? 3 : 1}
            style={{ minHeight: 0 }}
          />
          {noteDraft !== session.note ? (
            <Button size="small" appearance="primary" onClick={saveNote}>保存备注</Button>
          ) : null}
        </div>
        <div className={styles.actions}>
          {session.resume ? (
            <>
              <Button size="small" onClick={() => void resumeService.CopyToClipboard(session.key)}>复制恢复命令</Button>
              <Button size="small" appearance="primary" onClick={() => void resumeService.OpenInTerminal(session.key)}>在终端打开</Button>
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
          <Button size="small" onClick={exportMd}>导出 Markdown</Button>
          <span className={styles.grayed} style={{ display: 'inline-flex', gap: '6px' }}>
            <Tooltip content="v1.1 提供,当前版本未实现" relationship="description">
              <Button size="small" disabled>交接(v1.1)</Button>
            </Tooltip>
            <Tooltip content="v1.1 提供,当前版本未实现" relationship="description">
              <Button size="small" disabled>摘要(v1.1)</Button>
            </Tooltip>
          </span>
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
          tr.parts.map((p, i) => {
            const hit = i === jumpPart
            const setHitRef = (el: HTMLElement | null) => { if (hit) hitRef.current = el }
            const isUser = p.role === 'user'
            const isToolish = p.kind === 'tool_use' || p.kind === 'tool_result' || p.kind === 'context'
            const cls = styles.part + ' ' + (isUser ? styles.partUser : isToolish ? styles.partTool : styles.partAgent) + (hit ? ' ' + styles.partHit : '')
            if (p.kind === 'thinking' || p.role === 'thinking') {
              return (
                <details key={i} ref={setHitRef} className={cls} open={hit ? true : undefined}>
                  <summary className={styles.roleTag} style={{ cursor: 'pointer' }}>
                    思考过程(点击展开){p.model ? ` · ${p.model}` : ''}
                  </summary>
                  <div style={{ marginTop: 6 }}>{p.text}</div>
                </details>
              )
            }
            return (
              <div key={i} ref={setHitRef} className={cls}>
                <div className={styles.partHead}>
                  <AgentAvatar agent={isUser ? 'user' : session.agent} size={18} />
                  <span className={styles.roleName}>{isUser ? '用户' : m.label}</span>
                  {p.model ? <span className={styles.roleTag} style={{ display: 'inline' }}>{p.model}</span> : null}
                  <button className={styles.copyBtn} onClick={() => copyPart(p.text, i)} title="复制此段">
                    {copied === String(i) ? <CheckmarkRegular fontSize={12} /> : <CopyRegular fontSize={12} />}
                  </button>
                </div>
                {p.text}
              </div>
            )
          })
        ) : null}
      </div>
    </div>
  )
}

function shortErr(e: unknown): string {
  const s = String(e)
  return s.length > 200 ? s.slice(0, 200) + '…' : s
}
