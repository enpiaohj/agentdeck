import { FluentProvider, makeStyles, tokens, webDarkTheme, webLightTheme } from '@fluentui/react-components'
import { useState } from 'react'
import { useTheme } from './hooks/useTheme'
import { Sidebar, type SidebarFilter } from './components/Sidebar'
import { SessionList } from './components/SessionList'
import { DetailPane } from './components/DetailPane'
import type { SessionView } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

const useStyles = makeStyles({
  root: {
    display: 'grid',
    gridTemplateColumns: 'auto minmax(0, 1fr) auto',
    height: '100vh',
    overflow: 'hidden',
    backgroundColor: tokens.colorNeutralBackground1,
  },
  listWrap: { minWidth: 0, borderRight: `1px solid ${tokens.colorNeutralStroke2}`, overflow: 'hidden' },
  detailWrap: { minWidth: 0, overflow: 'hidden' },
})

// 三栏布局:左导航(可折叠)/ 中列表 / 右详情(可收起)。
export function App() {
  const themeName = useTheme()
  const styles = useStyles()
  const [collapsed, setCollapsed] = useState(false)
  const [detailOpen, setDetailOpen] = useState(true)
  const [filter, setFilter] = useState<SidebarFilter>({})
  const [selected, setSelected] = useState<SessionView | null>(null)
  const [listVersion, setListVersion] = useState(0)

  const onSelect = (s: SessionView) => {
    setSelected(s)
    setDetailOpen(true)
  }

  return (
    <FluentProvider theme={themeName === 'dark' ? webDarkTheme : webLightTheme}>
      <div className={styles.root}>
        <Sidebar
          collapsed={collapsed}
          onToggle={() => setCollapsed((v) => !v)}
          filter={filter}
          onFilter={setFilter}
        />
        <div className={styles.listWrap}>
          <SessionList filter={filter} selectedKey={selected?.key ?? null} onSelect={onSelect} listVersion={listVersion} />
        </div>
        {detailOpen ? (
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
        )}
      </div>
    </FluentProvider>
  )
}
