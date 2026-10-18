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
  ArrowClockwiseRegular,
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
import { usePreferences } from './hooks/usePreferences'
import { WorkspaceStatus } from './components/WorkspaceStatus'
import { ClaudeHistoryImport } from './components/ClaudeHistoryImport'
import { DeleteSessionsDialog, DeletedSessionsNotice } from './components/DeleteSessionsDialog'
import { useSessionDeletion } from './hooks/useSessionDeletion'
import { deletedSessionKeys } from './sessionDeletion'
import type { SessionView, ClaudeImportReport } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as searchService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import * as sessionService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'

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
  const { preferences } = usePreferences()
  const themeName = useTheme()
  const styles = useStyles()
  const [collapsed, setCollapsed] = useState(() => window.innerWidth < 900)
  const [infoOpen, setInfoOpen] = useState(true)
  const [filter, setFilter] = useState<SidebarFilter>({})
  const [selected, setSelected] = useState<SessionView | null>(null)
  const [listVersion, setListVersion] = useState(0)
  const [sourceVersion, setSourceVersion] = useState(0)
  const selectionRequest = useRef(0)
  const [searchQuery, setSearchQuery] = useState('')
  const searchRef = useRef<HTMLInputElement>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshError, setRefreshError] = useState('')
  const [notice, setNotice] = useState('')
  const [view, setView] = useState<'sessions' | 'diagnostics' | 'imports'>('sessions')
  const [crashed, setCrashed] = useState(false)
  const [jumpPart, setJumpPart] = useState(-1)
  const [jumpToken, setJumpToken] = useState(0)
  const [maximised, setMaximised] = useState(false)

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

  useEffect(() => {
    const off = Events.On('sessions:messages', () => refreshSelected())
    const offChanged = Events.On('sessions:changed', event => {
      setListVersion(value => value + 1)
      const key = Array.isArray(event.data) ? event.data[0] : event.data
      if (!key) setSourceVersion(value => value + 1)
    })
    const offDeleted = Events.On('sessions:deleted', event => onSessionsDeleted(deletedSessionKeys(event.data)))
    return () => { off(); offChanged(); offDeleted() }
  }, [refreshSelected, onSessionsDeleted])

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

  const onJump = useCallback((key: string, partIndex: number) => {
    setView('sessions')
    const request = ++selectionRequest.current
    sessionService.Get(key).then((v) => { if (v && request === selectionRequest.current) { setSelected(v); setJumpPart(partIndex); setJumpToken(value => value + 1) } }).catch(reason => setRefreshError(`打开搜索结果失败：${String(reason)}`))
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
      if ((e.ctrlKey || e.metaKey) && e.key === ',') { e.preventDefault(); setView('diagnostics'); return }
      if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault()
        setView('sessions')
        requestAnimationFrame(() => { searchRef.current?.focus(); searchRef.current?.select() })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

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

  const onImported = async (report: ClaudeImportReport) => {
    setListVersion(value => value + 1)
    setSourceVersion(value => value + 1)
    await searchService.Rebuild()
    const rows = await sessionService.List({ agent: 'claude-desktop', favorites: false, archived: false, project: '', tag: '', search: '' })
    const first = rows.find(row => row.source === 'import' && row.account === report.account)
    setNotice(`${report.already_imported ? '文件已导入，未重复导入' : '聊天记录已导入'} · ${report.account} · ${report.conversations} 个会话${report.skipped_blocks ? `；${report.skipped_blocks} 个非文本内容块未纳入` : ''}`)
    setFilter({ agent: 'claude-desktop' }); setSearchQuery('')
    if (first) onSelect(first)
    else setView('sessions')
  }

  return (
    <FluentProvider theme={themeName === 'dark' ? darkTheme : lightTheme}>
      <div className={`ad-app ${styles.outer}`} data-theme={themeName} style={{ '--ad-reader-font-size':`${preferences.fontSize}px`, '--ad-reader-line-height':preferences.spacing==='comfortable'?'1.85':'1.65', '--ad-reader-width':preferences.readingWidth==='wide'?'1120px':'820px' } as React.CSSProperties}>
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
          {view === 'diagnostics' ? <div className="ad-titlebar-context">偏好设置</div> : <div
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
            <Tooltip content="导入 Claude 普通聊天 / 新版任务历史" relationship="description">
              <Button size="small" appearance="secondary" icon={<ArrowUploadRegular />} onClick={() => setView('imports')}>导入</Button>
            </Tooltip>
            <Tooltip content="重新扫描本机会话" relationship="description">
              <Button
                size="small"
                appearance="subtle"
                icon={<ArrowClockwiseRegular />}
                disabled={refreshing}
                onClick={() => void refresh()}
              >{refreshing ? '扫描中…' : '刷新'}</Button>
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
        {notice ? <div className={styles.crashed} role="status" style={{ backgroundColor: tokens.colorBrandBackground2, color: tokens.colorBrandForeground1 }}>
          <span>{notice}</span><button className={styles.crashedClose} aria-label="关闭导入提示" onClick={() => setNotice('')}><DismissRegular /></button>
        </div> : null}
        <main className={styles.main} onKeyDown={event => {
          if (document.querySelector('[role="dialog"][aria-modal="true"]')) return
          if (event.key === 'Escape' && !event.nativeEvent.isComposing && event.keyCode !== 229 && searchQuery) { setSearchQuery(''); searchRef.current?.focus() }
        }}>
          {view !== 'diagnostics' ? <Sidebar
            collapsed={collapsed}
            onToggle={() => setCollapsed((v) => !v)}
            filter={filter}
            onFilter={(f) => { setFilter(f); setSearchQuery(''); setView('sessions') }}
            listVersion={listVersion}
            onOpenDiagnostics={() => setView('diagnostics')}
          /> : null}
          {view === 'diagnostics' || view === 'imports' ? (
            <div style={{ gridColumn: view === 'diagnostics' ? '1 / -1' : 'span 3', minWidth: 0, overflow: 'hidden' }}>
              {view === 'imports' ? <ClaudeHistoryImport onImported={onImported} /> : <SettingsView revision={listVersion} onBack={() => setView('sessions')} />}
            </div>
          ) : (
            <>
              <div style={{ display: searchQuery.trim() ? 'none' : 'contents' }}><SessionList
                filter={filter}
                onFilter={setFilter}
                selectedKey={selected?.key ?? null}
                onSelect={onSelect}
                onOpenDetails={session => { onSelect(session); setInfoOpen(true) }}
                onDeleteSessions={deletion.open}
                listVersion={listVersion}
              /></div>
              {searchQuery.trim() ? <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}><Button size="small" onClick={() => setSearchQuery('')}>返回会话列表 · 保留筛选与位置</Button><SearchResults query={searchQuery} revision={listVersion} selectedKey={selected?.key ?? null} onFocusSearch={() => searchRef.current?.focus()} onJump={onJump} /></div> : null}
              <MessagePane session={selected} sourceVersion={sourceVersion} jumpPart={jumpPart} jumpToken={jumpToken} infoOpen={infoOpen} onToggleInfo={() => setInfoOpen(value => !value)} onOpenInfo={() => setInfoOpen(true)} />
              {infoOpen ? (
                <InfoPane
                  session={selected}
                  onChanged={() => setListVersion((v) => v + 1)}
                  onClose={() => setInfoOpen(false)}
                  onDeleteSessions={deletion.open}
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
        <DeletedSessionsNotice deletion={deletion} />
        <WorkspaceStatus session={view === 'sessions' ? selected : null} />
        <DeleteSessionsDialog deletion={deletion} />
      </div>
    </FluentProvider>
  )
}
