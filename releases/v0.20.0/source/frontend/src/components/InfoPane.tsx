// 会话详情：元数据、标签和备注；路径按需展开，恢复操作统一放在阅读区。
import {
  makeStyles,
  mergeClasses,
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
  AddRegular,
  DocumentTextRegular,
  StarFilled,
  StarRegular,
  ArchiveRegular,
  DeleteRegular,
  EditRegular,
  ArrowResetRegular,
  DocumentDismissRegular,
  ChevronRightRegular,
  PinRegular,
  PinOffRegular,
} from '@fluentui/react-icons'
import { useEffect, useRef, useState } from 'react'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentMeta, fmtTime, relTime } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { deletionUnavailableReason } from '../sessionDeletion'
import { UPSTREAM_DELETED_HINT } from '../sessionPresentation'
import { PaneResizer } from './PaneResizer'
import type { usePaneResizer } from '../hooks/usePaneResizer'
import type { Notify } from '../hooks/useToasts'
import { markDoneMessage } from '../markFeedback'
import { usePreferences } from '../hooks/usePreferences'
import { isPinned, togglePins } from '../pins'

const useStyles = makeStyles({
  wrap: {
    height: '100%',
    width: 'var(--ad-info-width)',
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    minHeight: 0,
    position: 'relative',
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
  titleInput: { flex: 1, minWidth: 0 },
  // 智能体与来源只占一行，不再用灰底卡片框住两个字
  agentRow: { display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', minHeight: '24px' },
  agentSource: { fontSize: '12px', color: tokens.colorNeutralForeground3 },
  agentSep: { color: tokens.colorNeutralForeground4 },
  noteState: { display: 'inline-flex', alignItems: 'center', gap: '3px', marginLeft: 'auto', fontSize: '11px', color: tokens.colorNeutralForeground3 },
  noteDirty: { color: tokens.colorPaletteMarigoldForeground1 },
  danger: {
    color: tokens.colorPaletteRedForeground1,
    ':hover': { color: tokens.colorPaletteRedForeground1, ...shorthands.borderColor(tokens.colorPaletteRedBorder2) },
  },
  advanced: { borderTop: `1px solid ${tokens.colorNeutralStroke3}`, paddingTop: '12px', fontSize: '12px', color: tokens.colorNeutralForeground2, lineHeight: 1.6 },
  advancedSummary: { display: 'flex', alignItems: 'center', gap: '6px', cursor: 'pointer', color: tokens.colorNeutralForeground3, marginBottom: '8px', listStyle: 'none', ':hover': { color: tokens.colorNeutralForeground1 } },
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
    backgroundColor: 'transparent',
    cursor: 'pointer',
    ':hover': { color: tokens.colorBrandForeground1, ...shorthands.borderColor(tokens.colorBrandStroke1) },
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
  /** 全局提示条；归档会让会话从列表消失，用它给一句带撤销的确认。 */
  notify?: Notify
  onClose: () => void
  onDeleteSessions: (items: SessionView[]) => void
  /** 详情栏宽度分隔条（由 App 持有偏好）；未传时不显示。 */
  resizer?: ReturnType<typeof usePaneResizer>
}

const noteDrafts = new Map<string, string>()

export function InfoPane({ session, onChanged, notify, onClose, onDeleteSessions, resizer }: Props) {
  const { preferences, update: updatePreferences } = usePreferences()
  const styles = useStyles()
  const [tagDraft, setTagDraft] = useState('')
  const [adding, setAdding] = useState(false)
  const [noteDraft, setNoteDraft] = useState(session?.note ?? '')
  const [copied, setCopied] = useState('')
  const [msg, setMsg] = useState('')
  const [editingTitle, setEditingTitle] = useState(false)
  const [titleDraft, setTitleDraft] = useState('')
  const [savingTitle, setSavingTitle] = useState(false)
  const currentKey = useRef(session?.key)
  currentKey.current = session?.key

  useEffect(() => { setNoteDraft(session ? noteDrafts.get(session.key) ?? session.note : '') }, [session?.key, session?.note])
  useEffect(() => { setMsg(''); setAdding(false); setTagDraft(''); setEditingTitle(false); setTitleDraft(''); setSavingTitle(false) }, [session?.key])

  if (!session) {
    return (
      <aside id="session-details" className={`ad-info-pane ${styles.wrap}`} aria-label="会话信息">
        {resizer ? <PaneResizer label="会话详情" edge="left" width={resizer} /> : null}
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
  const deleteReason = deletionUnavailableReason([session])

  const saveTitle = (value: string) => {
    const key = session.key
    setSavingTitle(true)
    setMsg('')
    markService.SetTitle(key, value).then(() => {
      if (currentKey.current === key) { setEditingTitle(false); setMsg(value ? '标题已保存' : '已恢复原名') }
      onChanged()
    }).catch((e: unknown) => { if (currentKey.current === key) setMsg(`修改标题失败：${String(e).slice(0, 120)}`) })
      .finally(() => { if (currentKey.current === key) setSavingTitle(false) })
  }

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
    const key = session.key
    const value = noteDraft
    markService.SetNote(key, value).then(() => {
      if (noteDrafts.get(key) === value) noteDrafts.delete(key)
      if (currentKey.current === key) setMsg('备注已保存')
      onChanged()
    }).catch((e) => { if (currentKey.current === key) setMsg(String(e)) })
  }
  const toggleStar = () => {
    markService.SetFavorite(session.key, !session.favorite).then(onChanged).catch(e => setMsg(`收藏操作失败：${String(e).slice(0, 120)}`))
  }
  const pinned = isPinned(preferences.pinnedSessions, session.key)
  const toggleArchive = () => {
    const key = session.key
    const archive = !session.archived
    markService.SetArchived(key, archive).then(() => {
      onChanged()
      notify?.(markDoneMessage(archive ? 'archive' : 'unarchive', 1), {
        tone: 'success',
        action: { label: '撤销', run: () => { markService.SetArchived(key, !archive).then(onChanged).catch(e => notify(`撤销失败：${String(e).slice(0, 120)}`, { tone: 'error' })) } },
      })
    }).catch(e => setMsg(`归档操作失败：${String(e).slice(0, 120)}`))
  }

  return (
    <aside id="session-details" className={`ad-info-pane ${styles.wrap}`} aria-label="会话信息">
      {resizer ? <PaneResizer label="会话详情" edge="left" width={resizer} /> : null}
      <div className={styles.tabs}>
        <Text className={styles.paneHeading}>会话详情</Text>
        <Button appearance="subtle" size="small" icon={<DismissRegular />} onClick={onClose} aria-label="收起信息面板" style={{ marginLeft: 'auto' }} />
      </div>

      <div className={styles.body}>
        <div className={styles.titleRow}>
          {editingTitle ? <>
            <Input
              className={styles.titleInput} size="small" autoFocus aria-label="会话标题"
              value={titleDraft} disabled={savingTitle}
              onChange={(_, data) => setTitleDraft(data.value)}
              onKeyDown={event => {
                if (event.nativeEvent.isComposing || event.keyCode === 229) return
                if (event.key === 'Enter') { event.preventDefault(); if (!savingTitle) { const title = titleDraft.trim(); if (title && title !== session.title) saveTitle(title); else if (title) setEditingTitle(false) } }
                if (event.key === 'Escape') { event.stopPropagation(); setEditingTitle(false) }
              }}
            />
            <Button size="small" appearance="subtle" icon={<CheckmarkRegular />} aria-label="保存标题" disabled={savingTitle || !titleDraft.trim()} onClick={() => { const title = titleDraft.trim(); if (title === session.title) setEditingTitle(false); else saveTitle(title) }} />
            <Button size="small" appearance="subtle" icon={<DismissRegular />} aria-label="取消编辑标题" disabled={savingTitle} onClick={() => setEditingTitle(false)} />
          </> : <>
            <Text className={styles.paneTitle}>{session.title || '未命名会话'}</Text>
            <Button size="small" appearance="subtle" icon={<EditRegular />} aria-label="编辑标题" disabled={savingTitle} onClick={() => { setTitleDraft(session.title); setEditingTitle(true); setMsg('') }} />
            {session.title !== session.original_title ? <Button size="small" appearance="subtle" icon={<ArrowResetRegular />} aria-label="恢复原名" title={`恢复原名：${session.original_title || '未命名会话'}`} disabled={savingTitle} onClick={() => saveTitle('')} /> : null}
            <Button size="small" appearance="subtle" icon={session.favorite ? <StarFilled /> : <StarRegular />} onClick={toggleStar} aria-label={session.favorite ? '取消收藏' : '收藏'} />
            <Button appearance="subtle" size="small" icon={copied === 'title' ? <CheckmarkRegular /> : <CopyRegular />} onClick={() => copy(session.title, 'title')} aria-label="复制标题" />
          </>}
        </div>

        <div className={styles.agentRow}>
          <AgentMark agent={session.agent} size={22} />
          <Text size={300}>{m.label}{session.source === 'wsl' ? '(WSL)' : ''}</Text>
          {session.source === 'import' || session.source === 'cache' || session.source === 'retained' ? <><span aria-hidden="true" className={styles.agentSep}>·</span><span className={styles.agentSource}>{session.source === 'import' ? '官方导入' : session.source === 'cache' ? 'Desktop 聊天缓存' : '永久保留副本'}</span></> : null}
          {session.upstream_deleted ? <span role="img" aria-label="已在原 Agent 中删除" title={UPSTREAM_DELETED_HINT} style={{ display: 'inline-flex', color: tokens.colorNeutralForeground4, cursor: 'default' }}><DocumentDismissRegular fontSize={16} /></span> : null}
        </div>

        <div className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.sectionTitle}>标签</span>
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
            <span role="status" className={mergeClasses(styles.noteState, noteDraft !== session.note && styles.noteDirty)}>
              {noteDraft !== session.note ? '未保存 · 本次运行保留草稿' : <><CheckmarkRegular fontSize={12} />已保存</>}
            </span>
            {noteDraft !== session.note ? (
              <button className={styles.linkBtn} title="Ctrl+Enter 保存备注" onClick={saveNote}>保存</button>
            ) : null}
          </div>
          <Textarea
            size="small"
            placeholder="输入会话备注…" title="Ctrl+Enter 保存备注" aria-label="会话备注"
            value={noteDraft}
            onChange={(_, d) => { noteDrafts.set(session.key, d.value); setNoteDraft(d.value) }}
            onKeyDown={event => { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); saveNote() } }}
            rows={2}
            resize="vertical"
          />
        </div>

        <details className={mergeClasses('ad-disclosure', styles.advanced)} key={`metadata-${session.key}`}>
          <summary className={styles.advancedSummary}><ChevronRightRegular className="ad-disclosure-icon" fontSize={12} />完整会话信息</summary>
          <div className={styles.kv}>
            <span className={styles.k}>会话类型</span><span className={styles.v}>{session.role === 'subagent' ? '子 Agent 会话' : '主会话'}</span>
            {session.account ? <><span className={styles.k}>账号</span><span className={styles.v}>{session.account}</span></> : null}
            <span className={styles.k}>保留策略</span><span className={styles.v}>读取后持久保存，不自动清理</span>
            <span className={styles.k}>创建时间</span><span className={styles.v}>{fmtTime(session.created)}</span>
            <span className={styles.k}>最后更新</span><span className={styles.v} title={fmtTime(session.last)}>{relTime(session.last)}</span>
            <span className={styles.k}>消息数量</span><span className={styles.v}>{session.messages}</span>
            {session.tokens > 0 ? <><span className={styles.k}>Token 消耗</span><span className={styles.v}>约 {fmtTokens(session.tokens)}</span></> : null}
            <span className={styles.k}>项目</span><span className={styles.v}>{session.project || '未识别项目'}</span>
          </div>
        </details>
        {session.cwd || session.source_path ? <details className={mergeClasses('ad-disclosure', styles.advanced)} key={session.key}>
          <summary className={styles.advancedSummary}><ChevronRightRegular className="ad-disclosure-icon" fontSize={12} />工作目录与存储位置</summary>
          <div className={styles.kv}>
            {session.cwd ? <><span className={styles.k}>工作目录</span><span className={styles.v}>{session.cwd}</span></> : null}
            {session.source_path ? <><span className={styles.k}>源路径</span><span className={styles.v}>{session.source_path}</span></> : null}
          </div>
        </details> : null}
        <div className={styles.section}>
          <span className={styles.sectionTitle}>更多操作</span>
          <div className={styles.actions}>
            <Button size="small" icon={pinned ? <PinOffRegular /> : <PinRegular />} style={{ width: '100%' }} onClick={() => updatePreferences({ pinnedSessions: togglePins(preferences.pinnedSessions, [session.key]) })}>
              {pinned ? '取消置顶' : '置顶'}
            </Button>
            <Button size="small" icon={<ArchiveRegular />} style={{ width: '100%' }} onClick={toggleArchive}>
              {session.archived ? '取消归档' : '归档'}
            </Button>
            <Button size="small" icon={<DeleteRegular />} className={deleteReason ? undefined : styles.danger} style={{ width: '100%' }} disabled={!!deleteReason} title={deleteReason || '删除原始记录及全部保留版本'} onClick={() => onDeleteSessions([session])}>删除会话…</Button>
          </div>
          {deleteReason ? <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>{deleteReason}</Text> : null}
        </div>
      </div>

      {msg ? <div className={styles.foot} role="status"><Text size={200} style={{ color: tokens.colorNeutralForeground3, overflowWrap: 'anywhere' }}>{msg}</Text></div> : null}
    </aside>
  )
}
