import { FluentProvider, makeStyles, tokens, webDarkTheme, webLightTheme, Text, Button, Tooltip } from '@fluentui/react-components'
import { SearchRegular, ArrowClockwiseRegular, DoctorRegular } from '@fluentui/react-icons'
import { useCallback, useEffect, useState } from 'react'
import { useTheme } from './hooks/useTheme'
import { Sidebar, type SidebarFilter } from './components/Sidebar'
import { SessionList } from './components/SessionList'
import { DetailPane } from './components/DetailPane'
import { SearchPalette } from './components/SearchPalette'
import { DiagnosticsView } from './components/DiagnosticsView'
import type { SessionView } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'

const useStyles = makeStyles({
  outer: { display: 'flex', flexDirection: 'column', height: '100vh', overflow: 'hidden', backgroundColor: tokens.colorNeutralBackground1 },
  main: { display: 'grid', gridTemplateColumns: 'auto minmax(0, 1fr) auto', flex: 1, minHeight: 0 },
  topbar: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '6px 12px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke2}`,
    backgroundColor: tokens.colorNeutralBackground2,
  },
  searchBtn: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    border: `1px solid ${tokens.colorNeutralStroke1}`,
    background: tokens.colorNeutralBackground1,
    borderRadius: '6px',
    padding: '5px 10px',
    cursor: 'pointer',
    color: tokens.colorNeutralForeground3,
    fontSize: '13px',
    minWidth: '280px',
  },
  title: { fontWeight: 600, fontSize: '14px' },
  listWrap: { minWidth: 0, borderRight: `1px solid ${tokens.colorNeutralStroke2}`, overflow: 'hidden' },
  detailWrap: { minWidth: 0, overflow: 'hidden' },
  crashed: {
    padding: '6px 12px',
    fontSize: '12px',
    backgroundColor: tokens.colorStatusWarningBackground2,
    color: tokens.colorStatusWarningForeground1,
  },
})

// 三栏布局:顶栏(Ctrl+K/刷新)/左导航(可折叠)/中列表/右详情(可收起)。
export function App() {
  const themeName = useTheme()
  const styles = useStyles()
  const [collapsed, setCollapsed] = useState(false)
  const [detailOpen, setDetailOpen] = useState(true)
  const [filter, setFilter] = useState<SidebarFilter>({})
  const [selected, setSelected] = useState<SessionView | null>(null)
  const [listVersion, setListVersion] = useState(0)
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [view, setView] = useState<'sessions' | 'diagnostics'>('sessions')
  const [crashed, setCrashed] = useState(false)

  useEffect(() => {
    sessionService.LastCrashed().then(setCrashed).catch(() => {})
  }, [])

  const onSelect = (s: SessionView) => {
    setSelected(s)
    setView('sessions')
    setDetailOpen(true)
  }

  // 搜索结果跳转:按 key 取最新视图并打开详情
  const onJump = useCallback((key: string) => {
    setPaletteOpen(false)
    setView('sessions')
    sessionService.Get(key).then((v) => { if (v) onSelect(v) }).catch(() => {})
  }, [])

  // Ctrl+K 唤起全局搜索
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
          <Text className={styles.title}>AgentDeck</Text>
          <button className={styles.searchBtn} onClick={() => { setView('sessions'); setPaletteOpen(true) }}>
            <SearchRegular fontSize={14} />
            搜索会话内容…
            <span style={{ marginLeft: 'auto', fontSize: '11px' }}>Ctrl K</span>
          </button>
          <Tooltip content="重新加载会话列表" relationship="description">
            <Button appearance="subtle" icon={<ArrowClockwiseRegular />} onClick={() => setListVersion((v) => v + 1)} />
          </Tooltip>
          <span style={{ flex: 1 }} />
          <Tooltip content="能力诊断 / 备份恢复" relationship="description">
            <Button
              appearance="subtle"
              icon={<DoctorRegular />}
              onClick={() => setView(view === 'diagnostics' ? 'sessions' : 'diagnostics')}
            >
              诊断
            </Button>
          </Tooltip>
        </div>
        {crashed ? (
          <div className={styles.crashed}>检测到上次未正常退出,数据已自动恢复;如界面异常可点「诊断 → 重建索引」</div>
        ) : null}
        <div className={styles.main}>
          <Sidebar
            collapsed={collapsed}
            onToggle={() => setCollapsed((v) => !v)}
            filter={filter}
            onFilter={(f) => { setFilter(f); setView('sessions') }}
          />
          {view === 'diagnostics' ? (
            <div className={styles.listWrap}>
              <DiagnosticsView />
            </div>
          ) : (
            <div className={styles.listWrap}>
              <SessionList filter={filter} selectedKey={selected?.key ?? null} onSelect={onSelect} listVersion={listVersion} />
            </div>
          )}
          {view === 'sessions' ? (
            detailOpen ? (
              <div className={styles.detailWrap}>
                <DetailPane session={selected} onClose={() => setDetailOpen(false)} onChanged={() => setListVersion((v) => v + 1)} />
              </div>
            ) : (
              <button
                onClick={() => setDetailOpen(true)}
                title="展开详情"
                style={{ writingMode: 'vertical-rl', border: 'none', cursor: 'pointer', background: tokens.colorNeutralBackground2 }}
              >
                详情
              </button>
            )
          ) : null}
        </div>
      </div>
      <SearchPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} onJump={onJump} />
    </FluentProvider>
  )
}
