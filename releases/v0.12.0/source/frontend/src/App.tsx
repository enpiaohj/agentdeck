import {
  FluentProvider,
  makeStyles,
  shorthands,
  tokens,
  Button,
  Tooltip,
} from '@fluentui/react-components'
import {
  SearchRegular,
  ArrowUploadRegular,
  SettingsRegular,
  DismissRegular,
  PanelRightRegular,
  SubtractRegular,
  SquareRegular,
  SquareMultipleRegular,
} from '@fluentui/react-icons'
import { Window } from '@wailsio/runtime'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTheme } from './hooks/useTheme'
import { AppMark } from './icons/AppMark'
import applicationInfo from '../../build/windows/info.json'
import { darkTheme, lightTheme } from './theme'
import { Sidebar, type SidebarFilter } from './components/Sidebar'
import { SessionList } from './components/SessionList'
import { MessagePane } from './components/MessagePane'
import { InfoPane } from './components/InfoPane'
import { SearchResults } from './components/SearchPalette'
import { SettingsView } from './components/SettingsView'
import { NewProjectDialog } from './components/NewProjectDialog'
import { NewSessionDialog } from './components/NewSessionDialog'
import { ToastHost } from './components/Toasts'
import { usePreferences } from './hooks/usePreferences'
import { useToasts } from './hooks/useToasts'
import { useAgentAvailability } from './hooks/useAgentAvailability'
import { WorkspaceStatus } from './components/WorkspaceStatus'
import { ClaudeHistoryImport } from './components/ClaudeHistoryImport'
import { DeleteSessionsDialog, DeletedSessionsNotice } from './components/DeleteSessionsDialog'
import { useSessionDeletion } from './hooks/useSessionDeletion'
import { deletedSessionKeys } from './sessionDeletion'
import { navWidthRange, listWidthRange, infoWidthRange, paneWidthCss, readerWidthCss, rememberProject } from './preferences'
import { usePaneResizer, readerMinWidth } from './hooks/usePaneResizer'
import { PaneResizer } from './components/PaneResizer'
import { platformLabel } from './agentOptions'
import type { SessionView, ClaudeImportReport } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as searchService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import * as sessionService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { eventKey } from './markPatch'
import * as resumeService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'

const useStyles = makeStyles({
  outer: {
    display: 'flex',
    flexDirection: 'column',
    height: '100vh',
    width: '100vw',
    overflow: 'hidden',
    backgroundColor: tokens.colorNeutralBackground1,
  },
  // 四栏:导航 | 会话列表 | 消息流(自适应)| 会话信息
  // gridTemplateRows 必须显式 minmax(0,1fr):隐式行按内容定高,子列一旦
  // 撑高就会把消息流底部操作条与信息面板底部操作区挤出视口。
  main: {
    position: 'relative',
    display: 'grid',
    gridTemplateColumns: 'auto auto minmax(0, 1fr) auto',
    gridTemplateRows: 'minmax(0, 1fr)',
    flex: 1,
    minHeight: 0,
    minWidth: 0,
    overflow: 'hidden',
  },
  topbar: {
    display: 'flex',
    alignItems: 'center',
    gap: '16px',
    padding: '0 12px 0 16px',
    minHeight: '48px',
    height: '48px',
    flexShrink: 0,
    backgroundColor: tokens.colorNeutralBackground1,
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    minWidth: 0,
  },
  brand: { display: 'flex', alignItems: 'center', gap: '9px', flexShrink: 0, width: 'calc(var(--ad-nav-width) - 16px)' },
  brandText: { display: 'flex', alignItems: 'baseline', gap: '7px', lineHeight: 1.3 },
  brandTitle: { fontSize: '15px', fontWeight: 650, letterSpacing: '-0.025em' },
  brandSub: { fontSize: '11px', color: tokens.colorNeutralForeground3 },
  searchSlot: {
    flex: 1,
    minWidth: 0,
    maxWidth: '560px',
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    margin: '0 auto',
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke2),
    ...shorthands.borderRadius('6px'),
    backgroundColor: tokens.colorNeutralBackground2,
    padding: '0 12px',
    height: '34px',
    cursor: 'text',
    color: tokens.colorNeutralForeground3,
    fontSize: '14px',
    transitionProperty: 'border-color, box-shadow',
    transitionDuration: '120ms',
    ':hover': { ...shorthands.borderColor(tokens.colorNeutralStroke1Hover) },
    ':focus-within': { ...shorthands.borderColor(tokens.colorBrandStroke1), boxShadow: `0 0 0 3px ${tokens.colorBrandBackground2}` },
  },
  searchText: { flex: 1, minWidth: 0, width: '100%', border: 'none', outline: 'none', backgroundColor: 'transparent', color: tokens.colorNeutralForeground1, fontSize: '13px', height: '100%' },
  kbd: {
    fontSize: '11px',
    color: tokens.colorNeutralForeground3,
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3),
    ...shorthands.borderRadius('5px'),
    padding: '1px 6px',
    backgroundColor: tokens.colorNeutralBackground3,
    flexShrink: 0,
  },
  right: { display: 'flex', alignItems: 'center', gap: '4px', flexShrink: 0 },
  // 自绘窗口控制(无边框窗口;拖拽用 Wails v3 的 --wails-draggable 属性)
  winControls: {
    display: 'flex',
    alignItems: 'center',
    gap: '2px',
    marginLeft: '6px',
    alignSelf: 'stretch',
  },
  winBtn: {
    width: '38px',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    ...shorthands.borderRadius('4px'),
    color: tokens.colorNeutralForeground2,
    ':hover': { backgroundColor: tokens.colorNeutralBackground3Hover },
  },
  winBtnClose: {
    width: '38px',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    ...shorthands.borderRadius('4px'),
    color: tokens.colorNeutralForeground2,
    ':hover': { backgroundColor: '#C42B1C', color: '#ffffff' },
  },
  crashed: {
    display: 'flex',
    alignItems: 'center',
    padding: '6px 20px',
    fontSize: '12px',
    backgroundColor: tokens.colorStatusWarningBackground2,
    color: tokens.colorStatusWarningForeground1,
  },
  crashedClose: {
    marginLeft: 'auto',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    color: 'inherit',
    display: 'flex',
    alignItems: 'center',
    padding: '2px',
  },
  infoStrip: {
    display: 'flex',
    alignItems: 'center',
    border: 'none',
    borderLeft: `1px solid ${tokens.colorNeutralStroke3}`,
    width: '24px',
    justifyContent: 'center',
    backgroundColor: tokens.colorNeutralBackground2,
    cursor: 'pointer',
    padding: '0 6px',
    color: tokens.colorNeutralForeground3,
  },
})

// 四栏布局:顶栏(品牌/搜索/动作)+ 左导航 + 会话列表 + 消息流 + 会话信息。
export function App() {
  const { preferences, update } = usePreferences()
  const themeName = useTheme()
  const styles = useStyles()
  const { toasts, notify, dismiss } = useToasts()
  const availability = useAgentAvailability(0)
  const [collapsed, setCollapsed] = useState(() => preferences.navCollapsed || window.innerWidth < 900)
  const [infoOpen, setInfoOpen] = useState(preferences.infoPanel === 'open')
  const [filter, setFilter] = useState<SidebarFilter>({})
  const [selected, setSelected] = useState<SessionView | null>(null)
  const [listVersion, setListVersion] = useState(0)
  // listReset 只由侧栏「全部会话」自增，作为会话列表清空自身筛选的信号。
  const [listReset, setListReset] = useState(0)
  const [sourceVersion, setSourceVersion] = useState(0)
  // 标注写操作（收藏 / 归档 / 标签 / 备注 / 改名）不走整表重载：只重取变化的
  // 那几个 key，把标注字段并进已有的行（见 markPatch.ts）。markVersion 只驱动
  // 侧栏计数与项目这几处小请求。
  const [markVersion, setMarkVersion] = useState(0)
  const [markPatch, setMarkPatch] = useState<{ seq: number; views: SessionView[] } | null>(null)
  const pendingMarkKeys = useRef(new Set<string>())
  const markTimer = useRef<number | null>(null)
  const markSeq = useRef(0)
  const selectionRequest = useRef(0)
  const [searchQuery, setSearchQuery] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshError, setRefreshError] = useState('')
  const [view, setView] = useState<'sessions' | 'diagnostics' | 'imports'>('sessions')
  const [crashed, setCrashed] = useState(false)
  const [jumpPart, setJumpPart] = useState(-1)
  const [jumpToken, setJumpToken] = useState(0)
  const [maximised, setMaximised] = useState(false)
  // 新建弹窗由 App 持有：右键菜单、快捷键、侧栏按钮都走同一个入口，
  // 打开状态也只有一处。
  const [sessionDialog, setSessionDialog] = useState<{ project?: string; agent?: string } | null>(null)
  const [projectDialog, setProjectDialog] = useState<{ agent?: string } | null>(null)

  // 设置里改了「侧栏 / 信息栏默认状态」立即生效；之后用户手动开合不受影响。
  useEffect(() => { setCollapsed(preferences.navCollapsed || window.innerWidth < 900) }, [preferences.navCollapsed])
  useEffect(() => { setInfoOpen(preferences.infoPanel === 'open') }, [preferences.infoPanel])

  useEffect(() => {
    const compact = window.matchMedia('(max-width: 899px)')
    const fit = (event: MediaQueryListEvent) => {
      if (event.matches) setCollapsed(true)
    }
    compact.addEventListener('change', fit)
    return () => compact.removeEventListener('change', fit)
  }, [])

  // 当前选中会话 key 的引用:事件回调里需要最新值,又不希望因 selected
  // 每次更新而重订阅事件。
  const selectedRef = useRef<string | null>(null)
  selectedRef.current = selected?.key ?? null

  const onSessionsDeleted = useCallback((keys: string[]) => {
    if (!keys.length) return
    // Invalidate in-flight search jumps so a removed detail cannot reappear.
    selectionRequest.current++
    if (selectedRef.current && keys.includes(selectedRef.current)) {
      selectedRef.current = null
      setSelected(null); setJumpPart(-1)
    }
    setListVersion(value => value + 1)
    setSourceVersion(value => value + 1)
  }, [])
  const onDeletionRestored = useCallback(() => {
    setListVersion(value => value + 1)
    setSourceVersion(value => value + 1)
  }, [])
  const deletion = useSessionDeletion(onSessionsDeleted, onDeletionRestored)

  // 选中会话的快照刷新。消息数由后台补齐(事件 sessions:messages)、收藏/
  // 标签/备注由信息面板改动,两者都不走列表重载路径;不显式重取的话,
  // 信息面板与消息流头部的数字会一直停在打开那一刻的旧值(消息数恒为 0)。
  const refreshSelected = useCallback(() => {
    const k = selectedRef.current
    if (!k) return
    sessionService.Get(k).then((v) => { if (v && selectedRef.current === k) setSelected(v) }).catch(() => {})
  }, [])

  useEffect(() => { refreshSelected() }, [refreshSelected, listVersion])

  // 把一个时间窗内的标注变化合并成一次取数：批量收藏 N 个会话只产生一轮请求。
  const queueMarkRefresh = useCallback((key: string) => {
    pendingMarkKeys.current.add(key)
    if (markTimer.current !== null) return
    markTimer.current = window.setTimeout(() => {
      markTimer.current = null
      const keys = [...pendingMarkKeys.current]
      pendingMarkKeys.current.clear()
      void Promise.all(keys.map(k => sessionService.Get(k).catch(() => null))).then(results => {
        const views = results.filter((v): v is SessionView => !!v)
        // 取不到的 key（已被删除等）说明列表结构变了，退回整表重载。
        if (views.length !== keys.length) { setListVersion(value => value + 1); return }
        const current = selectedRef.current
        const fresh = views.find(v => v.key === current)
        if (fresh) setSelected(fresh)
        setMarkPatch({ seq: ++markSeq.current, views })
        setMarkVersion(value => value + 1)
      })
    }, 60)
  }, [])
  useEffect(() => () => { if (markTimer.current !== null) window.clearTimeout(markTimer.current) }, [])

  useEffect(() => {
    const off = Events.On('sessions:messages', () => refreshSelected())
    const offChanged = Events.On('sessions:changed', event => {
      const key = eventKey(event.data)
      if (key) { queueMarkRefresh(key); return }
      setListVersion(value => value + 1)
      setSourceVersion(value => value + 1)
    })
    // 只改变项目清单（新建 / 声明），会话行本身没变：只刷新侧栏。
    const offProjects = Events.On('projects:changed', () => setMarkVersion(value => value + 1))
    const offDeleted = Events.On('sessions:deleted', event => onSessionsDeleted(deletedSessionKeys(event.data)))
    return () => { off(); offChanged(); offProjects(); offDeleted() }
  }, [refreshSelected, onSessionsDeleted, queueMarkRefresh])

  // 无边框窗口:跟踪最大化状态以切换按钮图标
  useEffect(() => {
    Window.IsMaximised().then(setMaximised).catch(() => {})
  }, [])

  const toggleMaximise = async () => {
    const isMax = await Window.IsMaximised().catch(() => false)
    if (isMax) await Window.Restore().catch(() => {})
    else await Window.Maximise().catch(() => {})
    setMaximised(!isMax)
  }

  useEffect(() => {
    sessionService.LastCrashed().then(setCrashed).catch(() => {})
  }, [])

  // 启动后自动选中最近活动的会话(与邮件客户端一致:打开即见内容)。
  useEffect(() => {
    sessionService
      .List({ agent: '', favorites: false, archived: false, project: '', tag: '', search: '' })
      .then((rows) => { if (rows.length > 0) setSelected((cur) => cur ?? rows[0]) })
      .catch(() => {})
  }, [])

  const onSelect = (s: SessionView) => {
    selectionRequest.current++
    setSelected(s)
    setView('sessions')
    setJumpPart(-1)
  }

  // selectAllSessions 是侧栏「全部会话」的唯一入口：回到真正的全集，
  // 所以把智能体 / 项目 / 收藏 / 归档、顶栏搜索词和列表内的筛选一起清掉。
  // 其它侧栏项默认单选（单击替换整个筛选），只有按住 Ctrl 单击才叠加维度，
  // 见 Sidebar 的 pick。
  // listReset 是给 SessionList 的复位信号（关键词、月份、账号、排序都在它内部自持）。
  const selectAllSessions = () => {
    setFilter({})
    setSearchQuery('')
    setView('sessions')
    setListReset(value => value + 1)
  }

  const onJump = useCallback((key: string, partIndex: number) => {
    setView('sessions')
    const request = ++selectionRequest.current
    sessionService.Get(key).then((v) => { if (v && request === selectionRequest.current) { setSelected(v); setJumpPart(partIndex); setJumpToken(value => value + 1) } }).catch(reason => setRefreshError(`打开搜索结果失败：${String(reason)}`))
  }, [])

  const openNewSession = useCallback((project?: string, agent?: string) => {
    setView('sessions')
    setSessionDialog({ project, agent })
  }, [])
  const openNewProject = useCallback((agent?: string) => { setProjectDialog({ agent }) }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
      if ((e.ctrlKey || e.metaKey) && e.key === ',') { e.preventDefault(); setView('diagnostics'); return }
      if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault()
        setView('sessions')
        requestAnimationFrame(() => { searchRef.current?.focus(); searchRef.current?.select() })
        return
      }
      // Ctrl+Shift+N 先判：Shift 没判的话会先被 Ctrl+N 吃掉。
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'n' || e.key === 'N')) { e.preventDefault(); openNewProject(); return }
      if ((e.ctrlKey || e.metaKey) && !e.shiftKey && (e.key === 'n' || e.key === 'N')) { e.preventDefault(); openNewSession(); return }
      // F5 重新扫描本机会话；同时拦下 WebView 默认的整页重新加载
      if (e.key === 'F5' && !e.ctrlKey && !e.metaKey && !e.altKey) { e.preventDefault(); void refreshRef.current(); return }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [openNewProject, openNewSession])

  const refresh = async () => {
    if (refreshing) return
    setRefreshing(true); setRefreshError('')
    try {
      await sessionService.Refresh()
      setListVersion(value => value + 1)
      setSourceVersion(value => value + 1)
      await searchService.Rebuild()
    } catch (reason) { setRefreshError(String(reason)) }
    finally { setRefreshing(false) }
  }
  // 键盘处理在挂载时注册一次，经 ref 调到最新的 refresh（它读 refreshing 状态）
  const refreshRef = useRef(refresh)
  refreshRef.current = refresh

  // 启动会话后的自动刷新：CLI 写会话记录要时间，扫早了看不到；两次之后若
  // 仍没有，就把「点刷新」这条下一步摆出来，不让用户对着空列表等。
  const launchTimers = useRef<number[]>([])
  useEffect(() => () => { launchTimers.current.forEach(clearTimeout) }, [])
  const schedulePostLaunchRefresh = () => {
    launchTimers.current.forEach(clearTimeout)
    launchTimers.current = [
      window.setTimeout(() => setListVersion(value => value + 1), 12_000),
      window.setTimeout(() => {
        setListVersion(value => value + 1)
        notify('若仍未看到新会话，请按 F5 或点会话列表顶部的刷新按钮重新扫描本机会话。')
      }, 45_000),
    ]
  }
  const onLaunched = async (project: string, agent: string) => {
    await resumeService.NewSession(project, agent)
    update({ recentProjects: rememberProject(preferences.recentProjects, project) })
    notify(`已在终端中打开 ${platformLabel(agent)}：${project}。生成会话记录后列表会自动刷新。`, { tone: 'success' })
    schedulePostLaunchRefresh()
  }

  const onImported = async (report: ClaudeImportReport) => {
    setListVersion(value => value + 1)
    setSourceVersion(value => value + 1)
    await searchService.Rebuild()
    const rows = await sessionService.List({ agent: 'claude-desktop', favorites: false, archived: false, project: '', tag: '', search: '' })
    const first = rows.find(row => row.source === 'import' && row.account === report.account)
    notify(`${report.already_imported ? '文件已导入，未重复导入' : '聊天记录已导入'} · ${report.account} · ${report.conversations} 个会话${report.skipped_blocks ? `；${report.skipped_blocks} 个非文本内容块未纳入` : ''}`, { tone: 'success' })
    setFilter({ agents: ['claude-desktop'] }); setSearchQuery('')
    if (first) onSelect(first)
    else setView('sessions')
  }

  // 三栏宽度：导航、会话列表、会话详情都可用分隔条拖动，双击恢复自动宽度。
  // 没调过（0）时跟随布局默认，调过就固定，但始终给阅读区留下 readerMinWidth。
  // 量宽度直接查 DOM：列表的包装节点在搜索时是 display:contents，量不到宽度。
  const paneWidth = (kind: 'nav' | 'list' | 'info') => {
    const selector = kind === 'nav' ? '.ad-nav-slot' : kind === 'list' ? '.ad-list-pane' : '#session-details'
    return document.querySelector(selector)?.getBoundingClientRect().width ?? 0
  }
  // 拖动某一栏时，另两栏的实际宽度加上阅读区下限就是它的可用上限。
  const reserveExcept = (kind: 'nav' | 'list' | 'info') => () =>
    readerMinWidth + (['nav', 'list', 'info'] as const).filter(item => item !== kind).reduce((sum, item) => sum + paneWidth(item), 0)
  const nav = usePaneResizer(preferences.navWidth, navWidthRange, width => update({ navWidth: width }), 'right', reserveExcept('nav'))
  const list = usePaneResizer(preferences.listWidth, listWidthRange, width => update({ listWidth: width }), 'right', reserveExcept('list'))
  const info = usePaneResizer(preferences.infoWidth, infoWidthRange, width => update({ infoWidth: width }), 'left', reserveExcept('info'))
  const resizing = nav.resizing || list.resizing || info.resizing
  // 导航收起时宽度由折叠样式决定，拖动过的值只在展开态生效。
  const navWidth = collapsed ? 0 : nav.width
  const paneVars = {
    ...(navWidth ? { '--ad-nav-width': paneWidthCss(navWidth, navWidthRange) } : {}),
    ...(list.width ? { '--ad-list-width': paneWidthCss(list.width, listWidthRange) } : {}),
    ...(info.width ? { '--ad-info-width': paneWidthCss(info.width, infoWidthRange) } : {}),
  } as React.CSSProperties

  return (
    <FluentProvider theme={themeName === 'dark' ? darkTheme : lightTheme}>
      <div className={`ad-app ${styles.outer}${resizing ? ' is-resizing' : ''}`} data-theme={themeName} style={{ '--ad-reader-font-size':`${preferences.fontSize}px`, '--ad-reader-line-height':preferences.spacing==='comfortable'?'1.85':'1.65', '--ad-reader-width':readerWidthCss(preferences.readingWidth), ...paneVars } as React.CSSProperties}>
        {/* 无边框窗口:顶栏整体为拖拽区,交互控件标 no-drag */}
        <div className={`ad-titlebar ${styles.topbar}`} style={{ '--wails-draggable': 'drag' } as React.CSSProperties}
          onDoubleClick={event => {
            if (event.button !== 0 || event.defaultPrevented || !(event.target instanceof Element)) return
            if (getComputedStyle(event.target).getPropertyValue('--wails-draggable').trim() !== 'drag') return
            event.preventDefault()
            void toggleMaximise()
          }}>
          <div className={`ad-brand ${styles.brand}`}>
            <AppMark size={26} />
            <span className={styles.brandText}>
              <span className={`ad-brand-name ${styles.brandTitle}`}>AgentDeck</span>
              <span className="ad-brand-version" title={`AgentDeck 版本 ${applicationInfo.fixed.product_version}`}>v{applicationInfo.fixed.product_version}</span>
            </span>
          </div>
          {view === 'diagnostics' ? <div className="ad-titlebar-context">设置</div> : <div
            className={styles.searchSlot}
            style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}
          >
            <SearchRegular fontSize={15} />
            <input ref={searchRef} className={styles.searchText} aria-label="搜索全部会话正文" placeholder="搜索全部会话正文、代码、命令…" value={searchQuery}
              onChange={event => { setSearchQuery(event.target.value); setView('sessions') }}
              onKeyDown={event => {
                if (event.nativeEvent.isComposing || event.keyCode === 229) return
                if (event.key === 'Escape') { setSearchQuery(''); event.currentTarget.blur() }
                if (event.key === 'ArrowDown' && searchQuery.trim()) { event.preventDefault(); document.querySelector<HTMLButtonElement>('[aria-label="全文搜索结果"] button')?.focus() }
              }} />
            {searchQuery ? <button className={styles.crashedClose} aria-label="清除全文搜索" onClick={() => { setSearchQuery(''); searchRef.current?.focus() }}><DismissRegular /></button> : null}
            <span className={styles.kbd}>Ctrl K</span>
          </div>}
          {view !== 'diagnostics' ? <div className={styles.right} style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}>
            <Tooltip content="新建会话 (Ctrl+N)" relationship="description">
              <Button size="small" appearance="primary" onClick={() => openNewSession()}>新建会话</Button>
            </Tooltip>
            <Tooltip content="导入 Claude 普通聊天 / 新版任务历史" relationship="description">
              <Button size="small" appearance="secondary" icon={<ArrowUploadRegular />} onClick={() => setView('imports')}>导入</Button>
            </Tooltip>
            <Tooltip content="设置 (Ctrl+,)" relationship="label">
              <Button
                size="small"
                appearance="subtle"
                icon={<SettingsRegular />}
                onClick={() => setView('diagnostics')}
              />
            </Tooltip>
          </div> : null}
          <div className={styles.winControls} style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}>
            <button className={styles.winBtn} onClick={() => Window.Minimise()} title="最小化" aria-label="最小化">
              <SubtractRegular fontSize={13} />
            </button>
            <button className={styles.winBtn} onClick={() => void toggleMaximise()} title="最大化 / 还原" aria-label="最大化或还原">
              {maximised ? <SquareMultipleRegular fontSize={13} /> : <SquareRegular fontSize={12} />}
            </button>
            <button className={styles.winBtnClose} onClick={() => Window.Close()} title="关闭" aria-label="关闭">
              <DismissRegular fontSize={14} />
            </button>
          </div>
        </div>
        {crashed ? (
          <div className={styles.crashed}>
            <span>检测到上次未正常退出。如界面显示异常，请前往“设置 → 数据与诊断”重建索引。</span>
            <button className={styles.crashedClose} onClick={() => setCrashed(false)} title="关闭提示">
              <DismissRegular fontSize={14} />
            </button>
          </div>
        ) : null}
        {refreshError ? <div className={styles.crashed} role="alert">重新扫描失败：{refreshError}</div> : null}
        <main className={styles.main} onKeyDown={event => {
          if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
          if (event.key === 'Escape' && !event.nativeEvent.isComposing && event.keyCode !== 229 && searchQuery) { setSearchQuery(''); searchRef.current?.focus() }
        }}>
          {view !== 'diagnostics' ? <div className="ad-nav-slot" ref={nav.slot}>
            <Sidebar
              collapsed={collapsed}
              onToggle={() => setCollapsed((v) => !v)}
              filter={filter}
              onFilter={(f) => { setFilter(f); setSearchQuery(''); setView('sessions') }}
              onSelectAll={selectAllSessions}
              listVersion={listVersion + markVersion}
              onOpenDiagnostics={() => setView('diagnostics')}
              onRefresh={() => setListVersion(value => value + 1)}
              onNewSession={openNewSession}
              onNewProject={openNewProject}
              notify={notify}
            />
            {!collapsed ? <PaneResizer label="侧栏" width={nav} /> : null}
          </div> : null}
          {view === 'diagnostics' || view === 'imports' ? (
            <div style={{ gridColumn: view === 'diagnostics' ? '1 / -1' : 'span 3', minWidth: 0, overflow: 'hidden' }}>
              {view === 'imports' ? <ClaudeHistoryImport onImported={onImported} /> : <SettingsView revision={listVersion + markVersion} onBack={() => setView('sessions')} />}
            </div>
          ) : (
            <>
              <div style={{ display: searchQuery.trim() ? 'none' : 'contents' }}><SessionList
                filter={filter}
                onFilter={setFilter}
                resetToken={listReset}
                selectedKey={selected?.key ?? null}
                onSelect={onSelect}
                onOpenDetails={session => { onSelect(session); setInfoOpen(true) }}
                onDeleteSessions={deletion.open}
                listVersion={listVersion}
                markPatch={markPatch}
                notify={notify}
                onNewSession={() => openNewSession()}
                refreshing={refreshing}
                onRefresh={() => void refresh()}
                resizer={list}
              /></div>
              {searchQuery.trim() ? <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}><Button size="small" onClick={() => setSearchQuery('')}>返回会话列表 · 保留筛选与位置</Button><SearchResults query={searchQuery} revision={listVersion + markVersion} selectedKey={selected?.key ?? null} onFocusSearch={() => searchRef.current?.focus()} onJump={onJump} /></div> : null}
              <MessagePane session={selected} sourceVersion={sourceVersion} jumpPart={jumpPart} jumpToken={jumpToken} infoOpen={infoOpen} onToggleInfo={() => setInfoOpen(value => !value)} onOpenInfo={() => setInfoOpen(true)} />
              {infoOpen ? (
                <InfoPane
                  session={selected}
                  onChanged={() => { if (selected) queueMarkRefresh(selected.key) }}
                  notify={notify}
                  onClose={() => setInfoOpen(false)}
                  onDeleteSessions={deletion.open}
                  resizer={info}
                />
              ) : (
                <button
                  className={styles.infoStrip}
                  onClick={() => setInfoOpen(true)}
                  title="展开会话信息" aria-label="展开会话信息" aria-expanded={false}
                >
                  <PanelRightRegular fontSize={16} />
                </button>
              )}
            </>
          )}
        </main>
        <NewSessionDialog
          open={!!sessionDialog}
          project={sessionDialog?.project}
          initialAgent={sessionDialog?.agent}
          availability={availability}
          recent={preferences.recentProjects}
          onLaunched={onLaunched}
          onClose={() => setSessionDialog(null)}
        />
        <NewProjectDialog
          open={!!projectDialog}
          current={filter.project}
          initialAgent={projectDialog?.agent}
          availability={availability}
          onCreated={(path, agent) => {
            setFilter({ project: path })
            // 建完项目紧接着多半要开一个会话：给一条可以直接点的下一步，
            // 而不是让用户自己去找刚建出来的那一行。
            notify(`项目「${path}」已就绪。`, { tone: 'success', action: { label: '新建会话', run: () => openNewSession(path, agent) } })
          }}
          onChanged={() => setListVersion(value => value + 1)}
          onClose={() => setProjectDialog(null)}
        />
        <DeletedSessionsNotice deletion={deletion} />
        <ToastHost toasts={toasts} onDismiss={dismiss} />
        <WorkspaceStatus session={view === 'sessions' ? selected : null} />
        <DeleteSessionsDialog deletion={deletion} />
      </div>
    </FluentProvider>
  )
}