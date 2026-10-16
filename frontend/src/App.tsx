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
} from '@fluentui/react-icons'
import { useCallback, useEffect, useState } from 'react'
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
  main: {
    display: 'grid',
    gridTemplateColumns: 'auto auto minmax(0, 1fr) auto',
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
  brand: { display: 'flex', alignItems: 'center', gap: '10px', flexShrink: 0 },
  brandMark: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '34px',
    height: '34px',
    ...shorthands.borderRadius('9px'),
    background: `linear-gradient(140deg, ${tokens.colorBrandBackground2}, ${tokens.colorBrandBackground})`,
    color: tokens.colorNeutralForegroundOnBrand,
    boxShadow: tokens.shadow4,
  },
  brandText: { display: 'flex', flexDirection: 'column', lineHeight: 1.25 },
  brandTitle: { fontSize: '16px', fontWeight: 700, letterSpacing: '-0.01em' },
  brandSub: { fontSize: '11px', color: tokens.colorNeutralForeground3 },
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
    padding: '9px 14px',
    cursor: 'text',
    color: tokens.colorNeutralForeground3,
    fontSize: '13.5px',
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
        <div className={styles.topbar}>
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
            onClick={() => { setView('sessions'); setPaletteOpen(true) }}
          >
            <SearchRegular fontSize={15} />
            <span className={styles.searchText}>搜索会话、内容、命令…</span>
            <span className={styles.kbd}>Ctrl K</span>
          </div>
          <div className={styles.right}>
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
