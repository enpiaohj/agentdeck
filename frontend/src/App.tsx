import {
  FluentProvider,
  makeStyles,
  shorthands,
  tokens,
  webDarkTheme,
  webLightTheme,
  Button,
  Tooltip,
} from '@fluentui/react-components'
import {
  SearchRegular,
  ArrowClockwiseRegular,
  PulseRegular,
  AppsFilled,
  DismissRegular,
  ChevronDoubleRightRegular,
  SubtractRegular,
  SquareRegular,
  SquareMultipleRegular,
} from '@fluentui/react-icons'
import { Window } from '@wailsio/runtime'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTheme } from './hooks/useTheme'
import { Sidebar, type SidebarFilter } from './components/Sidebar'
import { SessionList } from './components/SessionList'
import { MessagePane } from './components/MessagePane'
import { InfoPane } from './components/InfoPane'
import { SearchPalette } from './components/SearchPalette'
import { DiagnosticsView } from './components/DiagnosticsView'
import type { SessionView } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
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
    padding: '10px 20px',
    backgroundColor: tokens.colorNeutralBackground2,
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    minWidth: 0,
  },
  brand: { display: 'flex', alignItems: 'center', gap: '11px', flexShrink: 0 },
  brandMark: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '38px',
    height: '38px',
    ...shorthands.borderRadius('10px'),
    background: `linear-gradient(140deg, ${tokens.colorBrandBackground2}, ${tokens.colorBrandBackground})`,
    color: tokens.colorNeutralForegroundOnBrand,
    boxShadow: tokens.shadow4,
  },
  brandText: { display: 'flex', flexDirection: 'column', lineHeight: 1.3 },
  brandTitle: { fontSize: '19px', fontWeight: 700, letterSpacing: '-0.01em' },
  brandSub: { fontSize: '12px', color: tokens.colorNeutralForeground2 },
  searchSlot: {
    flex: 1,
    minWidth: 0,
    maxWidth: '640px',
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    margin: '0 auto',
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke2),
    ...shorthands.borderRadius('10px'),
    backgroundColor: tokens.colorNeutralBackground1,
    padding: '10px 14px',
    cursor: 'text',
    color: tokens.colorNeutralForeground3,
    fontSize: '14px',
    boxShadow: tokens.shadow2,
    ':hover': { ...shorthands.borderColor(tokens.colorNeutralStroke1Hover) },
  },
  searchText: { flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  kbd: {
    fontSize: '11px',
    color: tokens.colorNeutralForeground3,
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3),
    ...shorthands.borderRadius('5px'),
    padding: '1px 6px',
    backgroundColor: tokens.colorNeutralBackground3,
    flexShrink: 0,
  },
  right: { display: 'flex', alignItems: 'center', gap: '6px', flexShrink: 0 },
  // 自绘窗口控制(无边框窗口;拖拽用 Wails v3 的 --wails-draggable 属性)
  winControls: {
    display: 'flex',
    alignItems: 'center',
    gap: '2px',
    marginLeft: '6px',
    alignSelf: 'stretch',
  },
  winBtn: {
    width: '34px',
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
    width: '34px',
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
    ...shorthands.borderLeft('1px', 'solid', tokens.colorNeutralStroke3),
    backgroundColor: tokens.colorNeutralBackground2,
    cursor: 'pointer',
    padding: '0 6px',
    color: tokens.colorNeutralForeground3,
  },
})

// 四栏布局:顶栏(品牌/搜索/动作)+ 左导航 + 会话列表 + 消息流 + 会话信息。
export function App() {
  const themeName = useTheme()
  const styles = useStyles()
  const [collapsed, setCollapsed] = useState(false)
  const [infoOpen, setInfoOpen] = useState(true)
  const [filter, setFilter] = useState<SidebarFilter>({})
  const [selected, setSelected] = useState<SessionView | null>(null)
  const [listVersion, setListVersion] = useState(0)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [view, setView] = useState<'sessions' | 'diagnostics'>('sessions')
  const [crashed, setCrashed] = useState(false)
  const [jumpPart, setJumpPart] = useState(-1)
  const [maximised, setMaximised] = useState(false)

  // 当前选中会话 key 的引用:事件回调里需要最新值,又不希望因 selected
  // 每次更新而重订阅事件。
  const selectedRef = useRef<string | null>(null)
  selectedRef.current = selected?.key ?? null

  // 选中会话的快照刷新。消息数由后台补齐(事件 sessions:messages)、收藏/
  // 标签/备注由信息面板改动,两者都不走列表重载路径;不显式重取的话,
  // 信息面板与消息流头部的数字会一直停在打开那一刻的旧值(消息数恒为 0)。
  const refreshSelected = useCallback(() => {
    const k = selectedRef.current
    if (!k) return
    sessionService.Get(k).then((v) => { if (v) setSelected(v) }).catch(() => {})
  }, [])

  useEffect(() => { refreshSelected() }, [refreshSelected, listVersion])

  useEffect(() => {
    const off = Events.On('sessions:messages', () => refreshSelected())
    return () => { off() }
  }, [refreshSelected])

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
    setSelected(s)
    setView('sessions')
    setJumpPart(-1)
  }

  const onJump = useCallback((key: string, partIndex: number) => {
    setPaletteOpen(false)
    setView('sessions')
    setJumpPart(partIndex)
    sessionService.Get(key).then((v) => { if (v) setSelected(v) }).catch(() => {})
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault()
        setView('sessions')
        setPaletteOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <FluentProvider theme={themeName === 'dark' ? webDarkTheme : webLightTheme}>
      <div className={styles.outer}>
        {/* 无边框窗口:顶栏整体为拖拽区,交互控件标 no-drag */}
        <div className={styles.topbar} style={{ '--wails-draggable': 'drag' } as React.CSSProperties}>
          <div className={styles.brand}>
            <span className={styles.brandMark}><AppsFilled fontSize={19} /></span>
            <span className={styles.brandText}>
              <span className={styles.brandTitle}>AgentDeck</span>
              <span className={styles.brandSub}>All your coding agents. One deck.</span>
            </span>
          </div>
          <div
            className={styles.searchSlot}
            role="button"
            aria-label="搜索会话内容"
            style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}
            onClick={() => { setView('sessions'); setPaletteOpen(true) }}
          >
            <SearchRegular fontSize={15} />
            <span className={styles.searchText}>搜索会话、内容、命令…</span>
            <span className={styles.kbd}>Ctrl K</span>
          </div>
          <div className={styles.right} style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}>
            <Tooltip content="重新扫描本机会话" relationship="description">
              <Button
                appearance="primary"
                icon={<ArrowClockwiseRegular />}
                onClick={() => setListVersion((v) => v + 1)}
              >刷新</Button>
            </Tooltip>
            <Tooltip content="能力诊断 / 备份恢复" relationship="description">
              <Button
                appearance="subtle"
                icon={<PulseRegular />}
                onClick={() => setView(view === 'diagnostics' ? 'sessions' : 'diagnostics')}
              />
            </Tooltip>
          </div>
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
            <span>检测到上次未正常退出,数据已自动恢复;如界面异常可点「诊断 → 重建索引」</span>
            <button className={styles.crashedClose} onClick={() => setCrashed(false)} title="关闭提示">
              <DismissRegular fontSize={14} />
            </button>
          </div>
        ) : null}
        <div className={styles.main}>
          <Sidebar
            collapsed={collapsed}
            onToggle={() => setCollapsed((v) => !v)}
            filter={filter}
            onFilter={(f) => { setFilter(f); setView('sessions') }}
            onOpenDiagnostics={() => setView('diagnostics')}
          />
          {view === 'diagnostics' ? (
            <div style={{ gridColumn: 'span 3', minWidth: 0, overflow: 'hidden' }}>
              <DiagnosticsView />
            </div>
          ) : (
            <>
              <SessionList
                filter={filter}
                selectedKey={selected?.key ?? null}
                onSelect={onSelect}
                listVersion={listVersion}
              />
              <MessagePane session={selected} jumpPart={jumpPart} onOpenInfo={() => setInfoOpen(true)} />
              {infoOpen ? (
                <InfoPane
                  session={selected}
                  onChanged={() => setListVersion((v) => v + 1)}
                  onClose={() => setInfoOpen(false)}
                />
              ) : (
                <button
                  className={styles.infoStrip}
                  onClick={() => setInfoOpen(true)}
                  title="展开会话信息"
                >
                  <ChevronDoubleRightRegular fontSize={12} />
                </button>
              )}
            </>
          )}
        </div>
      </div>
      <SearchPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} onJump={onJump} />
    </FluentProvider>
  )
}
