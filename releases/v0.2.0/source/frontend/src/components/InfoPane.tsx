// 会话详情：元数据、标签和备注；路径按需展开，恢复操作统一放在阅读区。
import {
  makeStyles,
  shorthands,
  tokens,
  Text,
  Button,
  Input,
  Textarea,
} from '@fluentui/react-components'
import {
  DismissRegular,
  CopyRegular,
  CheckmarkRegular,
  ArrowDownloadRegular,
  CheckmarkCircleRegular,
  AddRegular,
  DocumentTextRegular,
  StarFilled,
  StarRegular,
  ArchiveRegular,
} from '@fluentui/react-icons'
import { useEffect, useState } from 'react'
import { Dialogs } from '@wailsio/runtime'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentMeta, fmtTime, relTime } from '../util'
import { AgentMark } from '../icons/AgentMark'

const useStyles = makeStyles({
  wrap: {
    height: '100%',
    width: 'var(--ad-info-width)',
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    minHeight: 0,
    backgroundColor: tokens.colorNeutralBackground1,
    borderLeft: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  tabs: { display: 'flex', alignItems: 'center', gap: '8px', padding: '0 12px 0 16px', minHeight: '48px', flexShrink: 0, borderBottom: `1px solid ${tokens.colorNeutralStroke3}` },
  paneHeading: { fontSize: '12px', fontWeight: 600, color: tokens.colorNeutralForeground2 },
  body: { flex: 1, overflowY: 'auto', padding: '16px', display: 'flex', flexDirection: 'column', gap: '16px' },
  section: { display: 'flex', flexDirection: 'column', gap: '8px', borderTop: `1px solid ${tokens.colorNeutralStroke3}`, paddingTop: '12px' },
  sectionHead: { display: 'flex', alignItems: 'center', gap: '6px' },
  sectionTitle: { fontSize: '13px', fontWeight: 600 },
  linkBtn: {
    marginLeft: 'auto', border: 'none', background: 'transparent', cursor: 'pointer',
    fontSize: '12px', color: tokens.colorBrandForeground1, padding: 0,
  },
  titleRow: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '8px',
    paddingBottom: '8px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  paneTitle: { flex: 1, minWidth: 0, fontSize: '13px', lineHeight: 1.6, overflowWrap: 'anywhere', fontWeight: 600 },
  agentRow: { display: 'flex', alignItems: 'center', gap: '8px', padding: '10px', backgroundColor: tokens.colorNeutralBackground2, ...shorthands.borderRadius('6px') },
  advanced: { borderTop: `1px solid ${tokens.colorNeutralStroke3}`, paddingTop: '12px', fontSize: '12px', color: tokens.colorNeutralForeground2, lineHeight: 1.6 },
  advancedSummary: { cursor: 'pointer', color: tokens.colorNeutralForeground3, marginBottom: '8px' },
  capability: { display: 'inline-flex', gap: '3px', alignItems: 'center', marginLeft: 'auto', fontSize: '11px', color: tokens.colorBrandForeground1, whiteSpace: 'nowrap' },
  kv: { display: 'grid', gridTemplateColumns: '70px minmax(0, 1fr)', gap: '6px 8px', fontSize: '12px', lineHeight: 1.6 },
  k: { color: tokens.colorNeutralForeground3 },
  v: { color: tokens.colorNeutralForeground1, overflowWrap: 'anywhere' },
  chips: { display: 'flex', flexWrap: 'wrap', gap: '6px', alignItems: 'center' },
  chip: {
    display: 'inline-flex', alignItems: 'center', gap: '4px',
    fontSize: '12px', padding: '2px 4px 2px 8px',
    ...shorthands.borderRadius('6px'),
    backgroundColor: tokens.colorNeutralBackground3,
    color: tokens.colorNeutralForeground1,
  },
  chipX: { border: 'none', background: 'transparent', cursor: 'pointer', padding: '0 2px', color: tokens.colorNeutralForeground3, display: 'flex' },
  chipAdd: {
    display: 'inline-flex', alignItems: 'center', gap: '2px',
    fontSize: '12px', padding: '2px 8px',
    ...shorthands.borderRadius('6px'),
    ...shorthands.border('1px', 'dashed', tokens.colorNeutralStroke1),
    color: tokens.colorNeutralForeground3,
  },
  foot: {
    flexShrink: 0,
    padding: '12px 16px 14px',
    borderTop: `1px solid ${tokens.colorNeutralStroke3}`,
    backgroundColor: tokens.colorNeutralBackground1,
  },
  actions: { display: 'flex', flexWrap: 'wrap', gap: '8px' },
  empty: {
    padding: '72px 24px 24px',
    textAlign: 'center',
    color: tokens.colorNeutralForeground3,
  },
})

// fmtTokens 紧凑显示 token 数(百万级会话很常见)。
function fmtTokens(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}K`
  return String(n)
}

type Props = {
  session: SessionView | null
  onChanged: () => void
  onClose: () => void
}

export function InfoPane({ session, onChanged, onClose }: Props) {
  const styles = useStyles()
  const [tagDraft, setTagDraft] = useState('')
  const [adding, setAdding] = useState(false)
  const [noteDraft, setNoteDraft] = useState(session?.note ?? '')
  const [copied, setCopied] = useState('')
  const [msg, setMsg] = useState('')

  useEffect(() => { setNoteDraft(session?.note ?? '') }, [session?.key, session?.note])
  useEffect(() => { setMsg(''); setAdding(false); setTagDraft('') }, [session?.key])

  if (!session) {
    return (
      <aside id="session-details" className={`ad-info-pane ${styles.wrap}`} aria-label="会话信息">
        <div className={styles.tabs}>
          <Text className={styles.paneHeading}>会话详情</Text>
          <Button appearance="subtle" size="small" icon={<DismissRegular />} onClick={onClose} aria-label="收起信息面板" style={{ marginLeft: 'auto' }} />
        </div>
        <div className={styles.empty}>
          <DocumentTextRegular fontSize={24} />
          <Text size={200} style={{ display: 'block', marginTop: 8 }}>选择会话后显示详细信息、标签和可用操作。</Text>
        </div>
      </aside>
    )
  }

  const m = agentMeta(session.agent)

  const copy = (text: string, tag: string) => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(tag)
      setTimeout(() => setCopied(''), 1500)
    }).catch(() => {})
  }

  const addTag = () => {
    const t = tagDraft.trim()
    if (!t || session.tags.includes(t)) { setTagDraft(''); setAdding(false); return }
    markService.SetTags(session.key, [...session.tags, t]).then(() => {
      setTagDraft(''); setAdding(false); onChanged()
    }).catch(e => setMsg(`添加标签失败：${String(e).slice(0, 120)}`))
  }
  const removeTag = (t: string) => {
    markService.SetTags(session.key, session.tags.filter((x) => x !== t)).then(onChanged).catch(e => setMsg(`移除标签失败：${String(e).slice(0, 120)}`))
  }
  const saveNote = () => {
    markService.SetNote(session.key, noteDraft).then(() => { setMsg('备注已保存'); onChanged() }).catch((e) => setMsg(String(e)))
  }
  const toggleStar = () => {
    markService.SetFavorite(session.key, !session.favorite).then(onChanged).catch(e => setMsg(`收藏操作失败：${String(e).slice(0, 120)}`))
  }
  const toggleArchive = () => {
    markService.SetArchived(session.key, !session.archived).then(onChanged).catch(e => setMsg(`归档操作失败：${String(e).slice(0, 120)}`))
  }
  const exportMd = () => {
    Dialogs.OpenFile({
      CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true,
      Title: '选择 Markdown 导出目录', ButtonText: '导出到此目录',
    }).then((dir) => {
      if (!dir || typeof dir !== 'string') return
      sessionService.ExportMarkdown(session.key, dir).then(
        (p) => setMsg(`导出完成：${p}`),
        (e) => setMsg(`导出失败：${String(e).slice(0, 120)}`),
      )
    }).catch(() => {})
  }

  return (
    <aside id="session-details" className={`ad-info-pane ${styles.wrap}`} aria-label="会话信息">
      <div className={styles.tabs}>
        <Text className={styles.paneHeading}>会话详情</Text>
        <Button appearance="subtle" size="small" icon={<DismissRegular />} onClick={onClose} aria-label="收起信息面板" style={{ marginLeft: 'auto' }} />
      </div>

      <div className={styles.body}>
        <div className={styles.titleRow}>
          <Text className={styles.paneTitle}>{session.title || '未命名会话'}</Text>
          <Button
            appearance="subtle" size="small"
            icon={copied === 'title' ? <CheckmarkRegular /> : <CopyRegular />}
            onClick={() => copy(session.title, 'title')}
            title="复制标题"
          />
        </div>

        <div className={styles.agentRow}>
          <AgentMark agent={session.agent} size={22} />
          <Text size={300}>{m.label}{session.source === 'wsl' ? '(WSL)' : ''}</Text>
          <span className={styles.capability} style={{ color: session.resume ? undefined : tokens.colorNeutralForeground3 }}>{session.resume ? <><CheckmarkCircleRegular />支持继续会话</> : '仅支持查看'}</span>
        </div>

        <div className={styles.kv}>
          {session.account ? <><span className={styles.k}>账号</span><span className={styles.v}>{session.account}</span></> : null}
          {session.source === 'import' ? <><span className={styles.k}>记录来源</span><span className={styles.v}>官方导出 · 本地副本</span></> : null}
          {session.source === 'retained' ? <><span className={styles.k}>记录来源</span><span className={styles.v}>AgentDeck 永久保留</span></> : null}
          <span className={styles.k}>保留策略</span><span className={styles.v}>读取后持久保存，不自动清理</span>
          <span className={styles.k}>创建时间</span><span className={styles.v}>{fmtTime(session.created)}</span>
          <span className={styles.k}>最后更新</span><span className={styles.v} title={fmtTime(session.last)}>{relTime(session.last)}</span>
          <span className={styles.k}>消息数量</span><span className={styles.v}>{session.messages}</span>
          {session.tokens > 0 ? (<>
            <span className={styles.k}>Token 消耗</span>
            <span className={styles.v}>约 {fmtTokens(session.tokens)}</span>
          </>) : null}
          <span className={styles.k}>项目</span><span className={styles.v}>{session.project || '未识别项目'}</span>
          <span className={styles.k}>恢复能力</span>
          <span className={styles.v} >
            {session.resume ? '可恢复' : '仅支持查看历史'}
          </span>
        </div>

        {session.cwd || session.source_path ? <details className={styles.advanced} key={session.key}>
          <summary className={styles.advancedSummary}>工作目录与存储位置</summary>
          <div className={styles.kv}>
            {session.cwd ? <><span className={styles.k}>工作目录</span><span className={styles.v}>{session.cwd}</span></> : null}
            {session.source_path ? <><span className={styles.k}>源路径</span><span className={styles.v}>{session.source_path}</span></> : null}
          </div>
        </details> : null}

        <div className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.sectionTitle}>标签</span>
            <button
              className={styles.linkBtn}
              onClick={toggleStar}
            >
              {session.favorite ? <StarFilled style={{ color: tokens.colorStatusWarningForeground3 }} /> : <StarRegular />} {session.favorite ? '已收藏' : '收藏'}
            </button>
          </div>
          <div className={styles.chips}>
            {session.tags.map((t) => (
              <span key={t} className={styles.chip}>
                {t}
                <button className={styles.chipX} onClick={() => removeTag(t)} title={`移除 ${t}`}>
                  <DismissRegular fontSize={11} />
                </button>
              </span>
            ))}
            {adding ? (
              <Input
                size="small" autoFocus placeholder="标签名称" value={tagDraft} style={{ width: 100 }}
                onChange={(_, d) => setTagDraft(d.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') addTag(); if (e.key === 'Escape') setAdding(false) }}
                onBlur={addTag}
              />
            ) : (
              <button className={styles.chipAdd} onClick={() => setAdding(true)}>
                <AddRegular fontSize={12} />标签
              </button>
            )}
          </div>
        </div>

        <div className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.sectionTitle}>备注</span>
            {noteDraft !== session.note ? (
              <button className={styles.linkBtn} title="Ctrl+Enter 保存备注" onClick={saveNote}>保存</button>
            ) : null}
          </div>
          <Textarea
            size="small"
            placeholder="输入会话备注…" title="Ctrl+Enter 保存备注" aria-label="会话备注"
            value={noteDraft}
            onChange={(_, d) => setNoteDraft(d.value)}
            onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); saveNote() } }}
            rows={2}
            resize="vertical"
          />
        </div>

        <div className={styles.section}>
          <span className={styles.sectionTitle}>更多操作</span>
          <div className={styles.actions}>
            <Button size="small" icon={<ArrowDownloadRegular />} style={{ width: '100%' }} onClick={exportMd}>
              导出为 Markdown
            </Button>
            <Button size="small" icon={<ArchiveRegular />} style={{ width: '100%' }} onClick={toggleArchive}>
              {session.archived ? '取消归档' : '归档'}
            </Button>
          </div>
        </div>
      </div>

      {msg ? <div className={styles.foot} role="status"><Text size={200} style={{ color: tokens.colorNeutralForeground3, overflowWrap: 'anywhere' }}>{msg}</Text></div> : null}
    </aside>
  )
}
