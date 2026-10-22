// 中栏:会话列表。支持本地筛选、双向时间排序与按日分组，宽度随窗口变化。
// 桌面选择：单击 / Ctrl 切换 / Shift 连选 / Ctrl+A 全选，批量条独立占位。
import {
  makeStyles,
  mergeClasses,
  shorthands,
  tokens,
  Text,
  Spinner,
  Button,
  Input,
  Tooltip,
} from '@fluentui/react-components'
import {
  StarFilled,
  StarRegular,
  ArchiveRegular,
  ArchiveArrowBackRegular,
  ArrowClockwiseRegular,
  DocumentDismissRegular,
  ArrowSortRegular,
  SearchRegular,
  DismissRegular,
  TagRegular,
  SelectAllOnRegular,
  SelectAllOffRegular,
  ChevronRightRegular,
  ChevronDownRegular,
  DeleteRegular,
} from '@fluentui/react-icons'
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import { agentMeta, fmtTime, relTime } from '../util'
import { pickOpenPath } from '../nativeDialogs'
import { AgentMark } from '../icons/AgentMark'
import type { SidebarFilter } from './Sidebar'
import { usePreferences } from '../hooks/usePreferences'
import { presentSessions, sessionDateGroup, sessionMonth, UPSTREAM_DELETED_HINT, type SessionOrder } from '../sessionPresentation'
import { applyMarkPatch } from '../markPatch'
import { inverseAction, markDoneMessage, markPartialMessage, undoFailureMessage, type MarkAction } from '../markFeedback'
import type { Notify } from '../hooks/useToasts'
import { selectSession, selectContextSession, pruneSelection, type SessionSelection, type SelectionModifiers } from '../sessionSelection'
import { SessionContextMenu, type SessionAction } from './SessionContextMenu'
import { groupSessionRows } from '../sessionHierarchy'
import { deletionUnavailableReason, explicitDeletionTargets } from '../sessionDeletion'
import { PaneResizer } from './PaneResizer'
import type { usePaneResizer } from '../hooks/usePaneResizer'

const useStyles = makeStyles({
  wrap: {
    width: 'var(--ad-list-width)',
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    minWidth: 0,
    minHeight: 0,
    backgroundColor: tokens.colorNeutralBackground1,
    borderRight: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  head: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '16px 16px 10px',
  },
  // 标题优先完整显示（最多占一半宽），计数在剩余空间里省略；按钮靠右
  headTitle: { fontSize: '14px', fontWeight: 650, flexShrink: 0, maxWidth: '50%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  tools: { padding: '0 16px 12px', display: 'flex', flexDirection: 'column', gap: '8px', borderBottom: `1px solid ${tokens.colorNeutralStroke3}` },
  count: { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', fontSize: '11px', color: tokens.colorNeutralForeground3, fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' },
  headBtn: {
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    userSelect: 'none',
    color: tokens.colorNeutralForeground3,
    display: 'flex',
    alignItems: 'center',
    padding: '4px',
    ...shorthands.borderRadius('4px'),
    ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover },
  },
  rows: { flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', paddingBottom: '8px' },
  groupTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '14px 16px 7px',
    fontSize: '11px',
    fontWeight: 600,
    color: tokens.colorNeutralForeground3,
  },
  groupCount: { color: tokens.colorNeutralForeground4, fontWeight: 400, fontVariantNumeric: 'tabular-nums' },
  // 两行会话：标题与时间、来源与项目；保留完整信息的悬停说明。
  row: {
    position: 'relative',
    display: 'flex',
    gap: '8px',
    margin: '0 8px 3px',
    padding: '10px 8px',
    // 屏幕外的行不参与布局与绘制：几百上千行的列表滚动与首屏更轻，焦点、
    // 查找与 scrollIntoView 照常工作（行仍在 DOM 里，aria 结构不变）。
    contentVisibility: 'auto',
    containIntrinsicSize: 'auto 64px',
    ...shorthands.border('1px', 'solid', 'transparent'),
    ...shorthands.borderRadius('7px'),
    backgroundColor: tokens.colorNeutralBackground1,
    cursor: 'pointer',
    userSelect: 'none',
    ':hover': {
      backgroundColor: tokens.colorNeutralBackground1Hover,
    },
  },
  rowActive: {
    backgroundColor: tokens.colorBrandBackground2,
    ...shorthands.borderColor(tokens.colorBrandBackground2),
    boxShadow: `inset 3px 0 0 ${tokens.colorBrandBackground}`,
    ':hover': { backgroundColor: tokens.colorBrandBackground2Hover },
  },
  rowSelected: { backgroundColor: tokens.colorBrandBackground2, ...shorthands.borderColor(tokens.colorBrandBackground2), ':hover': { backgroundColor: tokens.colorBrandBackground2Hover } },
  childRow: { marginLeft: '24px', borderLeft: `1px solid ${tokens.colorNeutralStroke2}` },
  familyToggle: { display: 'inline-flex', alignItems: 'center', gap: '3px', padding: '1px 4px', border: 'none', borderRadius: '4px', backgroundColor: 'transparent', color: tokens.colorNeutralForeground3, fontSize: '11px', cursor: 'pointer', flexShrink: 0, ':hover': { backgroundColor: tokens.colorNeutralBackground3, color: tokens.colorNeutralForeground1 } },
  childLabel: { color: tokens.colorNeutralForeground3, fontSize: '10px', flexShrink: 0 },
  rowBody: { display: 'flex', flexDirection: 'column', gap: '3px', flex: 1, minWidth: 0 },
  titleLine: { display: 'flex', alignItems: 'center', gap: '6px', minWidth: 0 },
  title: { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '13px', fontWeight: 600 },
  starBtn: {
    border: 'none', background: 'transparent', cursor: 'pointer',
    display: 'flex', alignItems: 'center', padding: '2px', flexShrink: 0,
    ...shorthands.borderRadius('4px'),
    color: tokens.colorNeutralForeground4,
    ':hover': { color: tokens.colorStatusWarningForeground3 },
  },
  starOn: { color: tokens.colorStatusWarningForeground3 },
  titleOn: { color: tokens.colorNeutralForeground1 },
  meta: { display: 'flex', alignItems: 'center', gap: '5px', minWidth: 0, height: '18px', color: tokens.colorNeutralForeground3 },
  agentName: { fontSize: '11px', flexShrink: 0 },
  context: { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '11px' },
  tagCount: { display: 'inline-flex', alignItems: 'center', gap: '2px', fontSize: '10px', flexShrink: 0 },
  // 「上游已删」是状态而不是操作：低调的灰色图标，悬停给出说明，不抢标题与时间
  upstreamGone: { display: 'inline-flex', alignItems: 'center', flexShrink: 0, color: tokens.colorNeutralForeground4, cursor: 'default' },
  time: { fontSize: '11px', color: tokens.colorNeutralForeground3, flexShrink: 0, fontWeight: 400 },
  center: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: '8px', padding: '0 16px', textAlign: 'center' },
  bulkBar: {
    flexShrink: 0,
    minWidth: 0,
    margin: '0 8px 8px',
    flexWrap: 'wrap',
    justifyContent: 'flex-end',
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    padding: '6px',
    ...shorthands.borderRadius('6px'),
    backgroundColor: tokens.colorNeutralBackground3,
    boxShadow: tokens.shadow16,
    zIndex: 10,
  },
})

type Props = {
  filter: SidebarFilter
  selectedKey: string | null
  onSelect: (s: SessionView) => void
  onOpenDetails: (s: SessionView) => void
  onDeleteSessions: (items: SessionView[]) => void
  listVersion?: number
  /** 全局提示条：收藏 / 归档的结果与撤销都走它。未传时只做操作、不提示。 */
  notify?: Notify
  /** 空列表时的「新建会话」入口。 */
  onNewSession?: () => void
  /** 标注写操作后的局部更新（见 markPatch.ts）；seq 递增表示有新的一批。 */
  markPatch?: { seq: number; views: SessionView[] } | null
  onFilter: (filter: SidebarFilter) => void
  /** 递增即清空列表自身的筛选（关键词、月份、账号、排序）；侧栏「全部会话」用它回到全集。 */
  resetToken?: number
  /** 重新扫描本机会话（与 F5 同一入口）；放在列表头部，就在结果旁边。 */
  onRefresh?: () => void
  refreshing?: boolean
  /** 列表宽度分隔条（由 App 持有偏好）；未传时不显示。 */
  resizer?: ReturnType<typeof usePaneResizer>
}

export function SessionList({ filter, selectedKey, onSelect, onOpenDetails, onDeleteSessions, listVersion, markPatch, notify, onNewSession, onFilter, resetToken, onRefresh, refreshing = false, resizer }: Props) {
  const styles = useStyles()
  const { rows, loading, error, reload } = useRows(filter, listVersion, markPatch)
  const [selection, setSelection] = useState<SessionSelection>(() => ({ keys: new Set(selectedKey ? [selectedKey] : []), anchor: selectedKey, focus: selectedKey }))
  const rowRefs = useRef(new Map<string, HTMLDivElement>())
  const gridRef = useRef<HTMLDivElement>(null)
  const helpId = useId()
  const [busy, setBusy] = useState(false)
  const [bulkError, setBulkError] = useState('')
  const [actionMessage, setActionMessage] = useState('')
  const [context, setContext] = useState<{ key: string; keys: string[]; x: number; y: number; scope: string } | null>(null)
  const [renamingKey, setRenamingKey] = useState<string | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [renameSaving, setRenameSaving] = useState(false)
  const renamePending = useRef(false)
  const renameCancelled = useRef(false)
  const [query, setQuery] = useState('')
  const [month, setMonth] = useState('')
  const [account, setAccount] = useState('')
  const accounts = useMemo(() => [...new Set(rows.map(row => row.account).filter(Boolean))].sort(), [rows])
  const months = useMemo(() => [...new Set(rows.map(row => sessionMonth(row.last)).filter(Boolean))].sort().reverse(), [rows])
  const { preferences } = usePreferences()
  const [order, setOrder] = useState<SessionOrder>(preferences.listOrder)
  useEffect(() => { setOrder(preferences.listOrder) }, [preferences.listOrder])
  const [expandedFamilies, setExpandedFamilies] = useState<Set<string>>(() => new Set())
  const allFamilies = useMemo(() => groupSessionRows(rows, new Set()), [rows])
  const filteredRows = useMemo(() => presentSessions(rows, query, order, account).filter(row => !month || sessionMonth(row.last) === month), [rows, query, order, month, account])
  const hierarchy = useMemo(() => groupSessionRows(filteredRows, expandedFamilies), [filteredRows, expandedFamilies])
  const visibleRows = hierarchy.visible
  const selectedChild = filteredRows.find(row => row.key === selectedKey && hierarchy.childKeys.has(row.key))
  const pendingReveal = !!selectedChild?.group_key && !expandedFamilies.has(selectedChild.group_key)
  const visibleKeys = useMemo(() => visibleRows.map(row => row.key), [visibleRows])
  const visibleIndexes = useMemo(() => new Map(visibleKeys.map((key, index) => [key, index])), [visibleKeys])
  const visibleSelection = useMemo(() => pruneSelection(selection, visibleKeys), [selection, visibleKeys])
  const checked = visibleSelection.keys
  const deletionTargets = explicitDeletionTargets(visibleRows, checked)
  const deleteReason = deletionUnavailableReason(deletionTargets)
  const scope = JSON.stringify([filter.agents ?? [], filter.project, filter.favorites, filter.archived, query, month, account])
  const previousScope = useRef(scope)
  const contextItems = context?.scope === scope ? context.keys.map(key => visibleRows[visibleIndexes.get(key) ?? -1]).filter((row): row is SessionView => !!row) : []
  const contextOpen = !!context && contextItems.length === context.keys.length && contextItems.length > 0 && !busy

  useEffect(() => {
    if (previousScope.current !== scope) {
      previousScope.current = scope
      setSelection({ keys: new Set(), anchor: null, focus: null })
      setBulkError('')
      setContext(null)
      setRenamingKey(null)
      setActionMessage('')
    }
  }, [scope])
  // A search jump can mount the list with a child already selected. Wait for
  // its family's expansion before pruning, or the selection is lost on load.
  useEffect(() => { if (!loading && !pendingReveal) setSelection(previous => pruneSelection(previous, visibleKeys)) }, [visibleKeys, loading, pendingReveal])
  useEffect(() => { setMonth(''); setAccount('') }, [filter])
  // 「全部会话」的复位信号：把列表自持的筛选也退回默认。用计数而非布尔，
  // 这样连续两次点「全部会话」都能生效，也不会被其它筛选变更误触发。
  useEffect(() => {
    if (resetToken === undefined) return
    setQuery(''); setMonth(''); setAccount(''); setOrder(preferences.listOrder)
  }, [resetToken])
  useEffect(() => {
    if (selectedKey) setSelection(previous => previous.focus === selectedKey ? previous : { keys: new Set([selectedKey]), anchor: selectedKey, focus: selectedKey })
  }, [selectedKey])
  useEffect(() => {
    const selected = rows.find(row => row.key === selectedKey)
    if (!selected?.group_key || selected.role !== 'subagent') return
    setExpandedFamilies(previous => previous.has(selected.group_key) ? previous : new Set([...previous, selected.group_key]))
  }, [selectedKey, rows])

  const toggleFamily = (s: SessionView) => {
    if (busy) return
    const collapsing = expandedFamilies.has(s.key)
    if (collapsing && hierarchy.children.get(s.key)?.some(child => child.key === selectedKey)) {
      onSelect(s)
      setSelection({ keys: new Set([s.key]), anchor: s.key, focus: s.key })
    }
    setExpandedFamilies(previous => {
      const next = new Set(previous)
      if (collapsing) next.delete(s.key)
      else next.add(s.key)
      return next
    })
    setContext(null)
  }

  const choose = (s: SessionView, modifiers: SelectionModifiers = {}) => {
    if (busy) return
    const next = selectSession(visibleSelection, visibleKeys, s.key, modifiers)
    setSelection(next)
    setBulkError('')
    if (next.keys.has(s.key)) onSelect(s)
  }

  const selectAll = () => {
    if (busy || !visibleKeys.length) return
    setSelection({ keys: new Set(visibleKeys), anchor: visibleSelection.anchor ?? visibleKeys[0], focus: visibleSelection.focus ?? visibleKeys[0] })
    setBulkError('')
  }

  const clearSelection = () => {
    setSelection({ keys: new Set(), anchor: null, focus: visibleSelection.focus })
    setBulkError('')
    gridRef.current?.focus({ preventScroll: true })
  }

  // 一条标注操作的写入与反向写入（撤销用）。
  const applyMark = (action: MarkAction, key: string) => {
    if (action === 'favorite' || action === 'unfavorite') return markService.SetFavorite(key, action === 'favorite')
    return markService.SetArchived(key, action === 'archive')
  }
  // 成功后的一句话提示，带撤销：归档会让行直接从列表消失，没有这句话用户会
  // 怀疑是不是误删了。
  const announceMark = (action: MarkAction, keys: string[], failed = 0) => {
    if (!notify || keys.length === 0) return
    notify(failed ? markPartialMessage(action, keys.length, failed) : markDoneMessage(action, keys.length), {
      tone: failed ? 'info' : 'success',
      action: {
        label: '撤销',
        run: () => {
          void Promise.allSettled(keys.map(key => applyMark(inverseAction(action), key))).then(results => {
            const text = undoFailureMessage(results.filter(r => r.status === 'rejected').length)
            if (text) notify(text, { tone: 'error' })
          })
        },
      },
    })
  }

  const toggleStar = (s: SessionView) => (e: React.MouseEvent) => {
    e.stopPropagation()
    markService.SetFavorite(s.key, !s.favorite).catch(reason => notify?.(`收藏操作失败：${String(reason).slice(0, 120)}`, { tone: 'error' }))
  }

  const unarchive = (s: SessionView) => (e: React.MouseEvent) => {
    e.stopPropagation()
    markService.SetArchived(s.key, false).then(() => announceMark('unarchive', [s.key]))
      .catch(reason => notify?.(`取消归档失败：${String(reason).slice(0, 120)}`, { tone: 'error' }))
  }

  const bulk = (op: (key: string) => Promise<unknown>, targets: ReadonlySet<string> = checked, done?: (succeeded: string[], failed: number) => void) => async () => {
    if (targets.size === 0 || busy) return
    setBusy(true)
    setBulkError('')
    const succeeded = new Set<string>()
    const failed: string[] = []
    try {
      for (const key of targets) {
        try { await op(key); succeeded.add(key) }
        catch (reason) { failed.push(String(reason).slice(0, 120)) }
      }
      if (failed.length) setBulkError(`${failed.length} 项操作失败，未完成项保留选中：${failed[0]}`)
    } finally {
      setBusy(false)
      setSelection(previous => ({ ...previous, keys: new Set([...previous.keys].filter(key => !succeeded.has(key))) }))
      if (done) done([...succeeded], failed.length)
    }
  }

  const openContext = (s: SessionView, x: number, y: number) => {
    if (busy) return
    const next = selectContextSession(visibleSelection, visibleKeys, s.key)
    setSelection(next)
    setBulkError(''); setActionMessage('')
    setContext({ key: s.key, keys: visibleKeys.filter(key => next.keys.has(key)), x, y, scope })
    rowRefs.current.get(s.key)?.focus({ preventScroll: true })
  }
  const closeContext = () => {
    if (context) rowRefs.current.get(context.key)?.focus({ preventScroll: true })
    setContext(null)
  }
  const saveRename = async (key: string, previousTitle: string, draft: string) => {
    if (renameCancelled.current || renamePending.current) return
    const title = draft.trim()
    if (!title || title === previousTitle) { setRenamingKey(null); return }
    renamePending.current = true
    setRenameSaving(true); setBulkError('')
    try {
      await markService.SetTitle(key, title)
      setRenamingKey(null)
    } catch (reason) { setBulkError(`重命名失败：${String(reason).slice(0, 240)}`) }
    finally { renamePending.current = false; setRenameSaving(false) }
  }

  const contextAction = async (action: SessionAction) => {
    if (!contextOpen || !context || busy) return
    const items = contextItems
    const item = items[0]
    closeContext()
    if (action === 'open') { onSelect(item); return }
    if (action === 'details') { onOpenDetails(item); return }
    if (action === 'rename') { onSelect(item); renameCancelled.current = false; setRenamingKey(item.key); setRenameDraft(item.title); return }
    if (action === 'delete') { onDeleteSessions(items); return }
    if (action === 'favorite' || action === 'archive') {
      const value = !items.every(row => action === 'favorite' ? row.favorite : row.archived)
      const mark: MarkAction = action === 'favorite' ? (value ? 'favorite' : 'unfavorite') : (value ? 'archive' : 'unarchive')
      await bulk(key => applyMark(mark, key), new Set(items.map(row => row.key)), (keys, failed) => announceMark(mark, keys, failed))()
      return
    }
    setBusy(true); setBulkError(''); setActionMessage('')
    try {
      if (action === 'copyTitle' || action === 'copyCwd') {
        if (!navigator.clipboard) throw new Error('剪贴板当前不可用')
        await navigator.clipboard.writeText(action === 'copyCwd' ? item.cwd : items.map(row => row.title || '未命名会话').join('\n'))
        setActionMessage(action === 'copyCwd' ? '工作目录已复制。' : '会话标题已复制。')
      } else if (action === 'resume') {
        // 这条路径的错误消息后端保证自解释（其中一条是「终端已打开、只是
        // 附带动作失败」），不再套「操作失败：」前缀，否则与事实相反。
        try { await resumeService.OpenInTerminal(item.key); setActionMessage('已在终端中打开会话。') }
        catch (reason) { setBulkError(String(reason).slice(0, 240)) }
      } else if (action === 'copyResume') {
        await resumeService.CopyToClipboard(item.key); setActionMessage('恢复命令已复制。')
      } else if (action === 'export') {
        const dir = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: '选择 Markdown 导出目录', ButtonText: '导出到此目录' })
        if (!dir || typeof dir !== 'string') return
        let succeeded = 0
        const failures: string[] = []
        for (const row of items) {
          try { await sessionService.ExportMarkdown(row.key, dir); succeeded++ }
          catch (reason) { failures.push(`${row.title || '未命名会话'}：${String(reason).slice(0, 120)}`) }
        }
        setActionMessage(`已导出 ${succeeded} 个会话。`)
        if (failures.length) setBulkError(`${failures.length} 个会话导出失败。${failures[0]}`)
      }
    } catch (reason) { setBulkError(`操作失败：${String(reason).slice(0, 240)}`) }
    finally { setBusy(false) }
  }

  const groups = useMemo(() => {
    const g = new Map<string, SessionView[]>()
    for (const s of hierarchy.roots) {
      const d = sessionDateGroup(s.last)
      if (!g.has(d)) g.set(d, [])
      g.get(d)!.push(s)
      if (expandedFamilies.has(s.key)) g.get(d)!.push(...(hierarchy.children.get(s.key) ?? []))
    }
    const keys = [...g.keys()].filter(key => key !== '时间未知')
    if (g.has('时间未知')) keys.push('时间未知')
    return keys.map(key => [key, g.get(key)!] as const)
  }, [hierarchy, expandedFamilies])

  const projectName = filter.project?.split(/[\\/]/).filter(Boolean).pop()
  // 侧栏允许 Ctrl+单击叠加维度，标题必须如实反映全部条件：单项沿用原措辞，
  // 组合时用「·」连起来，否则「某项目的收藏」会只显示成项目名。
  const titleParts = [
    filter.archived ? '归档' : '',
    filter.favorites ? '收藏' : '',
    projectName ?? '',
    (filter.agents ?? []).map(agent => agentMeta(agent).label).join('、'),
  ].filter(Boolean)
  const listTitle = titleParts.length === 0 ? '全部会话'
    : titleParts.length === 1 ? (filter.archived ? '归档会话' : filter.favorites ? '收藏会话' : titleParts[0])
    : titleParts.join(' · ')

  return (
    <div className={`ad-list-pane ${styles.wrap}`} onKeyDown={event => {
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
      if (event.nativeEvent.isComposing || event.keyCode === 229 || (event.target as HTMLElement).closest('input, textarea, select, [contenteditable="true"], [role="menu"]')) return
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') { event.preventDefault(); selectAll() }
      if (event.key === 'Escape' && !busy && checked.size) { event.preventDefault(); event.stopPropagation(); clearSelection() }
    }}>
      <div className={styles.head}>
        <span className={styles.headTitle} title={filter.project || listTitle}>{listTitle}</span>
        <span className={styles.count} aria-live="polite" title="子会话收在主会话下，不重复计入会话数量">{query || month || account ? `${hierarchy.roots.length} / ${allFamilies.roots.length}` : allFamilies.roots.length} 个会话{hierarchy.childKeys.size ? ` · ${hierarchy.childKeys.size} 个子会话` : ''}</span>
        {/* 全选与清空是同一个位置的一个图标：没有选中项时是全选，选中后是清空。
            此前两个动作分在两端——头部是全选，底部批量条里才有「清空」，而且
            只在多选时才出现，单选一项时反而没有清空的入口。 */}
        <Tooltip content={checked.size ? '清空选择 (Esc)' : '全选筛选后当前展开的会话（含已展开子会话）(Ctrl+A)'} relationship="label">
          <Button size="small" appearance="subtle"
            icon={checked.size ? <SelectAllOffRegular /> : <SelectAllOnRegular />}
            aria-label={checked.size ? '清空选择' : '全选会话'}
            disabled={busy || (!checked.size && !visibleRows.length)}
            onClick={checked.size ? clearSelection : selectAll} />
        </Tooltip>
        <Tooltip content={order === 'latest' ? '切换为最早活动优先' : '切换为最近活动优先'} relationship="label">
        <button className={styles.headBtn} aria-label={order === 'latest' ? '最早活动优先' : '最近活动优先'} onClick={() => setOrder(order === 'latest' ? 'oldest' : 'latest')}>
          <ArrowSortRegular fontSize={15} />
        </button>
        </Tooltip>
        {onRefresh ? <Tooltip content={refreshing ? '正在扫描本机会话…' : '重新扫描本机会话 (F5)'} relationship="label">
          <Button size="small" appearance="subtle" aria-label="刷新会话列表" disabled={refreshing}
            icon={refreshing ? <Spinner size="extra-tiny" /> : <ArrowClockwiseRegular />}
            onClick={onRefresh} />
        </Tooltip> : null}
      </div>
      <span id={helpId} className="ad-sr-only">单击选中；Ctrl 点击切换；Shift 连选；Ctrl+Shift 追加连选；Ctrl+A 全选筛选后当前展开项（包含屏幕外行）；Esc 清空选择。上下方向键移动，Home / End 到首尾；左右方向键展开或收起子会话。</span>
      <div className={styles.tools}>
        <div className="ad-filter-chips">
          {(filter.agents ?? []).map(agent => <Button key={agent} size="small" onClick={() => {
            const agents = (filter.agents ?? []).filter(value => value !== agent)
            const next: SidebarFilter = { ...filter, agents }
            if (!agents.length) delete next.agents
            onFilter(next)
          }}>{agentMeta(agent).label} ×</Button>)}
          {filter.project ? <Button size="small" title={filter.project} onClick={() => onFilter({ ...filter, project: undefined })}>{projectName} ×</Button> : null}
          {filter.favorites || filter.archived ? <Button size="small" onClick={() => onFilter({ ...filter, favorites: false, archived: false })}>{filter.archived ? '归档' : '收藏'} ×</Button> : null}
        </div>
        <Input size="small" appearance="filled-lighter" aria-label="筛选当前会话列表" placeholder="筛选标题、项目、标签、账号…" value={query} onChange={(_, data) => setQuery(data.value)} contentBefore={<SearchRegular />} contentAfter={query || month || account ? <button className={styles.headBtn} aria-label="清除列表筛选" title="清除关键词、月份和账号筛选" onClick={() => { setQuery(''); setMonth(''); setAccount('') }}><DismissRegular /></button> : undefined} />
        <div className="ad-list-filters"><div className="ad-date-filter">
          <select aria-label="按活动月份筛选会话" value={month} onChange={event => setMonth(event.target.value)}>
            <option value="">所有月份</option>
            {months.map(value => <option key={value} value={value}>{value.replace('-', '年')}月</option>)}
          </select>
        </div>
        {accounts.length ? <div className="ad-date-filter">
          <select aria-label="按聊天账号筛选会话" value={account} onChange={event => setAccount(event.target.value)}>
            <option value="">全部账号</option>
            {accounts.map(value => <option key={value} value={value}>{value}</option>)}
          </select>
        </div> : null}</div>
        {filter.agents?.length === 1 && filter.agents[0] === 'claude-desktop' ? <div className="ad-source-note">本地记录与导入的聊天历史</div> : null}
      </div>

      {error ? (
        <div className={styles.center} role="alert">
          <Text size={300}>会话加载失败：{error}</Text>
          <Button size="small" appearance="primary" onClick={reload}>重试</Button>
        </div>
      ) : loading && rows.length === 0 ? (
        <div className={styles.center}>
          <Spinner size="tiny" />
          <ScanStatus />
        </div>
      ) : visibleRows.length === 0 ? (
        <div className={styles.center}>
          <Text size={400} weight="semibold">
            {query || month || account ? '未找到匹配会话' : filter.archived ? '暂无归档会话' : filter.favorites ? '暂无收藏会话' : '未发现会话'}
          </Text>
          <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>
            {query || month || account ? '请调整关键词，或清除筛选条件。' : filter.archived
              ? '会话归档后显示在此列表，可随时取消归档。'
              : '使用受支持的 Agent 创建会话后，请单击“刷新”重新扫描。'}
          </Text>
          {query || month || account ? <Button size="small" onClick={() => { setQuery(''); setMonth(''); setAccount('') }}>清除筛选</Button> : null}
          {!(query || month || account) && !filter.archived && !filter.favorites && onNewSession ? <>
            <Button size="small" appearance="primary" onClick={onNewSession}>新建会话</Button>
            <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>已有会话却没出现？到「设置 → 数据来源」检查智能体是否被检测到。</Text>
          </> : null}
        </div>
      ) : (
        <div ref={gridRef} className={styles.rows} role="grid" aria-label="会话列表" aria-multiselectable="true" aria-rowcount={visibleRows.length} aria-colcount={1} aria-describedby={helpId} aria-busy={busy} tabIndex={-1} onKeyDown={event => {
          if (busy || event.target !== event.currentTarget || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
          event.preventDefault()
          const s = event.key === 'End' ? visibleRows.at(-1) : event.key === 'Home' ? visibleRows[0] : visibleRows.find(row => row.key === visibleSelection.focus) ?? visibleRows[0]
          if (s) {
            if ((event.ctrlKey || event.metaKey) && !event.shiftKey) setSelection(previous => ({ ...previous, focus: s.key }))
            else choose(s, { toggle: event.ctrlKey || event.metaKey, range: event.shiftKey })
            rowRefs.current.get(s.key)?.focus()
          }
        }}>
          {groups.map(([day, list]) => (
            <div key={day} role="rowgroup" aria-label={day}>
              <div className={styles.groupTitle} aria-hidden="true">
                {day}<span className={styles.groupCount}>{list.filter(row => !hierarchy.childKeys.has(row.key)).length}</span>
              </div>
              {list.map((s) => {
                const selected = checked.has(s.key)
                const active = selected && selectedKey === s.key
                const m = agentMeta(s.agent)
                // 行内信息与「全部会话」一致：筛选只决定列哪些会话，不省略行里的智能体与项目
                const contextLabel = [s.account, s.project].filter(Boolean).join(' · ')
                const detail = [m.label, s.role === 'subagent' ? '子会话' : '', contextLabel, s.tags.length ? `标签：${s.tags.join('、')}` : '', s.upstream_deleted ? '上游已删除' : ''].filter(Boolean).join(' · ')
                const childCount = hierarchy.children.get(s.key)?.length ?? 0
                const nested = hierarchy.childKeys.has(s.key)
                return (
                  <div
                    key={s.key}
                    ref={element => { if (element) rowRefs.current.set(s.key, element); else rowRefs.current.delete(s.key) }}
                    className={mergeClasses('sessionRow', styles.row, nested && styles.childRow, selected && styles.rowSelected, active && styles.rowActive)}
                    data-session-key={s.key}
                    data-child-session={nested || undefined}
                    onMouseDown={event => { if (!renamingKey && (event.shiftKey || event.ctrlKey || event.metaKey)) event.preventDefault() }}
                    onClick={event => { if ((event.target as HTMLElement).closest('button, input')) return; choose(s, { toggle: event.ctrlKey || event.metaKey, range: event.shiftKey }); event.currentTarget.focus() }}
                    onContextMenu={event => { event.preventDefault(); event.stopPropagation(); openContext(s, event.clientX, event.clientY) }}
                    tabIndex={(visibleSelection.focus ?? visibleKeys[0]) === s.key ? 0 : -1}
                    role="row"
                    aria-label={`查看会话 ${s.title || '未命名会话'}`}
                    aria-selected={selected}
                    aria-haspopup="menu"
                    aria-expanded={contextOpen && context?.key === s.key}
                    aria-rowindex={(visibleIndexes.get(s.key) ?? 0) + 1}
                    onKeyDown={(e) => {
                      if (e.target !== e.currentTarget || e.nativeEvent.isComposing || busy) return
                      const modifiers = { toggle: e.ctrlKey || e.metaKey, range: e.shiftKey }
                      if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) { e.preventDefault(); e.stopPropagation(); const box=e.currentTarget.getBoundingClientRect(); openContext(s, box.left+20, box.top+20); return }
                      if (e.key === 'F2' && !modifiers.toggle && !modifiers.range) { e.preventDefault(); onSelect(s); renameCancelled.current = false; setRenamingKey(s.key); setRenameDraft(s.title); return }
                      if (e.key === 'Delete') {
                        // 与右键菜单同一入口：先出预览与确认，不会直接删。选中行属于多选时作用于整组。
                        e.preventDefault()
                        const targets = checked.has(s.key) ? deletionTargets : [s]
                        if (!deletionUnavailableReason(targets)) onDeleteSessions(targets)
                        return
                      }
                      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); choose(s, modifiers); return }
                      if (e.key === 'ArrowRight' && childCount && !expandedFamilies.has(s.key)) { e.preventDefault(); toggleFamily(s); return }
                      if (e.key === 'ArrowLeft') {
                        if (childCount && expandedFamilies.has(s.key)) { e.preventDefault(); toggleFamily(s); return }
                        const parent = nested ? filteredRows.find(row => row.key === s.group_key) : undefined
                        if (parent) { e.preventDefault(); choose(parent); rowRefs.current.get(parent.key)?.focus(); return }
                      }
                      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) return
                      e.preventDefault()
                      const index = visibleIndexes.get(s.key) ?? 0
                      const nextIndex = e.key === 'Home' ? 0 : e.key === 'End' ? visibleRows.length - 1 : Math.max(0, Math.min(visibleRows.length - 1, index + (e.key === 'ArrowDown' ? 1 : -1)))
                      const next = visibleRows[nextIndex]
                      if (modifiers.toggle && !modifiers.range) setSelection(previous => ({ ...previous, focus: next.key }))
                      else choose(next, modifiers)
                      rowRefs.current.get(next.key)?.focus()
                      rowRefs.current.get(next.key)?.scrollIntoView({ block: 'nearest' })
                    }}
                  >
                    <div className={styles.rowBody} role="gridcell">
                      <div className={styles.titleLine}>
                        {renamingKey === s.key ? <Input
                          className={styles.title} size="small" autoFocus aria-label={`重命名会话 ${s.title || '未命名会话'}`}
                          value={renameDraft} disabled={renameSaving}
                          onClick={event => event.stopPropagation()}
                          onChange={(_, data) => setRenameDraft(data.value)}
                          onBlur={event => { if (!renameCancelled.current) void saveRename(s.key, s.title, event.currentTarget.value) }}
                          onKeyDown={event => {
                            event.stopPropagation()
                            if (event.nativeEvent.isComposing || event.keyCode === 229) return
                            if (event.key === 'Enter') { event.preventDefault(); void saveRename(s.key, s.title, event.currentTarget.value) }
                            if (event.key === 'Escape' && !renameSaving) { event.preventDefault(); renameCancelled.current = true; setRenamingKey(null) }
                          }}
                        /> : <Text title={s.title || '未命名会话'} className={mergeClasses(styles.title, selected && styles.titleOn)}>{s.title || '未命名会话'}</Text>}
                        <span className={styles.time} title={fmtTime(s.last)}>{relTime(s.last)}</span>
                        <button
                          data-favorite={s.favorite}
                          className={mergeClasses('ad-row-star', styles.starBtn, s.favorite && styles.starOn)}
                          onClick={toggleStar(s)}
                          title={s.favorite ? '取消收藏' : '收藏'}
                          aria-label={`${s.favorite ? '取消收藏' : '收藏'}会话 ${s.title || '未命名会话'}`}
                          tabIndex={(visibleSelection.focus ?? visibleKeys[0]) === s.key ? 0 : -1}
                        >
                          {s.favorite ? <StarFilled fontSize={13} /> : <StarRegular fontSize={13} />}
                        </button>
                      </div>
                      <div className={styles.meta} title={detail}>
                        <AgentMark agent={s.agent} size={14} />
                        {s.role === 'subagent' ? <span className={styles.childLabel}>子会话</span> : null}
                        <span className={styles.agentName} title={m.label}>{m.label}</span><span aria-hidden="true">·</span>
                        <span className={styles.context}>{contextLabel || '无项目'}</span>
                        {childCount ? <button type="button" className={styles.familyToggle} aria-label={`${expandedFamilies.has(s.key) ? '收起' : '展开'} ${childCount} 个子会话`} aria-expanded={expandedFamilies.has(s.key)} disabled={busy} onClick={event => { event.stopPropagation(); toggleFamily(s) }}>
                          {expandedFamilies.has(s.key) ? <ChevronDownRegular fontSize={12} /> : <ChevronRightRegular fontSize={12} />}{childCount} 个子会话
                        </button> : null}
                        {s.tags.length ? <span className={styles.tagCount} aria-label={`${s.tags.length} 个标签`}><TagRegular fontSize={11} />{s.tags.length}</span> : null}
                        {s.upstream_deleted ? <span className={styles.upstreamGone} role="img" aria-label="已在原 Agent 中删除" title={UPSTREAM_DELETED_HINT}><DocumentDismissRegular fontSize={13} /></span> : null}
                        {filter.archived ? <Button size="small" appearance="subtle" icon={<ArchiveArrowBackRegular />} aria-label="取消归档" title="取消归档" tabIndex={(visibleSelection.focus ?? visibleKeys[0]) === s.key ? 0 : -1} onClick={unarchive(s)} /> : null}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      )}

      {contextOpen && context ? <SessionContextMenu key={`${context.key}:${context.x}:${context.y}`} items={contextItems} point={context} onClose={closeContext} onAction={action => void contextAction(action)} /> : null}
      {actionMessage ? <Text size={200} role="status" style={{ padding: '0 12px 8px', color: tokens.colorNeutralForeground3 }}>{actionMessage}</Text> : null}
      {bulkError ? <Text size={200} role="alert" style={{ padding: '0 12px 8px', color: tokens.colorPaletteRedForeground1, overflowWrap: 'anywhere' }}>{bulkError}</Text> : null}
      {checked.size === 1 ? <Text size={200} style={{ padding: '6px 16px', color: tokens.colorNeutralForeground3 }}>正在阅读 · Ctrl / Shift 多选，右键查看单项操作</Text> : null}
      {checked.size > 1 ? (
        <div className={styles.bulkBar} role="group" aria-label="批量会话操作">
          <Text size={200} weight="semibold" aria-live="polite" style={{ marginRight: 'auto' }}>已选 {checked.size} 项</Text>
          <Button size="small" icon={<StarRegular />} disabled={busy}
            onClick={bulk((k) => applyMark('favorite', k), checked, (keys, failed) => announceMark('favorite', keys, failed))}>收藏</Button>
          <Button size="small" icon={filter.archived ? <ArchiveArrowBackRegular /> : <ArchiveRegular />} disabled={busy}
            onClick={bulk((k) => applyMark(filter.archived ? 'unarchive' : 'archive', k), checked, (keys, failed) => announceMark(filter.archived ? 'unarchive' : 'archive', keys, failed))}>{filter.archived ? '取消归档' : '归档'}</Button>
          <Button size="small" icon={<DeleteRegular />} disabled={busy || !!deleteReason} title={deleteReason || '删除原始记录及全部保留版本'} onClick={() => onDeleteSessions(deletionTargets)}>删除会话…</Button>
        </div>
      ) : null}
      {checked.size > 1 && deleteReason ? <Text size={200} className="ad-delete-disabled-reason">{deleteReason}</Text> : null}
      {resizer ? <PaneResizer label="会话列表" width={resizer} /> : null}
    </div>
  )
}

type RowsState = { rows: SessionView[]; loading: boolean; error: string; reload: () => void }

function useRows(filter: SidebarFilter, listVersion?: number, markPatch?: { seq: number; views: SessionView[] } | null): RowsState {
  const scope = JSON.stringify([filter.agents ?? [], filter.project ?? '', !!filter.favorites, !!filter.archived])
  const [state, setState] = useState({ rows: [] as SessionView[], loading: true, error: '', scope })

  const request = useRef(0)
  const load = useCallback(() => {
    const version = ++request.current
    setState((s) => ({ rows: s.scope === scope ? s.rows : [], loading: true, error: '', scope }))
    sessionService
      .List({
        agent: '',
        agents: filter.agents ?? [],
        favorites: filter.favorites ?? false,
        archived: filter.archived ?? false,
        project: filter.project ?? '',
        tag: '',
        search: '',
      })
      .then((rows) => { if (version === request.current) setState({ rows, loading: false, error: '', scope }) })
      .catch((e) => { if (version === request.current) setState({ rows: [], loading: false, error: String(e), scope }) })
  }, [filter, scope])

  useEffect(() => { load(); return () => { request.current++ } }, [load, listVersion])

  useEffect(() => {
    const off = Events.On('sessions:messages', () => load())
    return () => { off() }
  }, [load])

  // 标注变化：只把收藏 / 归档 / 标签 / 备注 / 标题并进已有的行；判断不了
  // （有行新进入当前范围）才退回整表重载。
  const rowsRef = useRef(state.rows)
  rowsRef.current = state.rows
  const appliedPatch = useRef(0)
  useEffect(() => {
    if (!markPatch || markPatch.seq === appliedPatch.current) return
    appliedPatch.current = markPatch.seq
    const next = applyMarkPatch(rowsRef.current, markPatch.views, { favorites: filter.favorites, archived: filter.archived })
    if (next === null) load()
    else setState((s) => ({ ...s, rows: next }))
  }, [markPatch, filter, load])

  // A new sidebar scope must never expose the previous scope's actionable
  // rows, including the render before the request-start effect runs.
  return state.scope === scope ? { ...state, reload: load } : { rows: [], loading: true, error: '', reload: load }
}

// 首次扫描（或缓存失效后重建）可能要十几秒：后端早就有进度，这里轮询出来，
// 让用户知道是在推进而不是卡住。没有进度数据时退回原来的一句话。
function ScanStatus() {
  const [progress, setProgress] = useState<{ files: number; done: number } | null>(null)
  useEffect(() => {
    let active = true
    const tick = () => sessionService.ScanProgress()
      .then(p => { if (active) setProgress(p.indexing && p.files > 0 ? { files: p.files, done: p.done } : null) })
      .catch(() => {})
    tick()
    const timer = window.setInterval(tick, 500)
    return () => { active = false; window.clearInterval(timer) }
  }, [])
  return <Text size={300} role="status">正在扫描本机会话…{progress ? `（已处理 ${progress.done} / ${progress.files} 个文件）` : ''}</Text>
}
