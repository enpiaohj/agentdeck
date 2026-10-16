// 左导航:agent 筛选 + 收藏/归档入口 + 索引状态条。可折叠 220↔48px。
import {
  makeStyles,
  tokens,
  Text,
  Tooltip,
} from '@fluentui/react-components'
import {
  ArchiveRegular,
  ChevronDoubleLeftRegular,
  ChevronDoubleRightRegular,
  AppsRegular,
  StarRegular,
} from '@fluentui/react-icons'
import { useEffect, useState } from 'react'
import type { AgentView, IndexStatus } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { useIndexStatus } from '../hooks/useIndexStatus'
import { agentLabel } from '../util'

export type SidebarFilter = {
  agent?: string
  favorites?: boolean
  archived?: boolean
}

const useStyles = makeStyles({
  nav: {
    width: '220px',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    backgroundColor: tokens.colorNeutralBackground2,
    borderRight: `1px solid ${tokens.colorNeutralStroke2}`,
  },
  navCollapsed: { width: '48px' },
  head: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 12px' },
  item: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    width: '100%',
    padding: '8px 12px',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    textAlign: 'left',
    fontSize: '13px',
    color: tokens.colorNeutralForeground1,
    ':hover': { backgroundColor: tokens.colorNeutralBackground3Hover },
  },
  itemActive: { backgroundColor: tokens.colorNeutralBackground4, fontWeight: 600 },
  count: { marginLeft: 'auto', color: tokens.colorNeutralForeground3, fontSize: '12px' },
  spacer: { flex: 1 },
  status: { padding: '10px 12px', fontSize: '12px', color: tokens.colorNeutralForeground3 },
})

type Props = {
  collapsed: boolean
  onToggle: () => void
  filter: SidebarFilter
  onFilter: (f: SidebarFilter) => void
}

function isAll(f: SidebarFilter) { return !f.agent && !f.favorites && !f.archived }

export function Sidebar({ collapsed, onToggle, filter, onFilter }: Props) {
  const styles = useStyles()
  const agents = useAgents()
  const status = useIndexStatus()

  const pick = (f: SidebarFilter) => () => onFilter(f)

  if (collapsed) {
    return (
      <nav className={`${styles.nav} ${styles.navCollapsed}`}>
        <button className={styles.item} onClick={onToggle} title="展开导航">
          <ChevronDoubleRightRegular />
        </button>
        <button className={`${styles.item} ${isAll(filter) ? styles.itemActive : ''}`} onClick={pick({})} title="全部会话">
          <AppsRegular />
        </button>
        <button className={`${styles.item} ${filter.favorites ? styles.itemActive : ''}`} onClick={pick({ favorites: true })} title="收藏">
          <StarRegular />
        </button>
        <button className={`${styles.item} ${filter.archived ? styles.itemActive : ''}`} onClick={pick({ archived: true })} title="归档">
          <ArchiveRegular />
        </button>
      </nav>
    )
  }

  return (
    <nav className={styles.nav}>
      <div className={styles.head}>
        <Text weight="semibold">AgentDeck</Text>
        <button className={styles.item} style={{ width: 'auto' }} onClick={onToggle} title="折叠导航">
          <ChevronDoubleLeftRegular />
        </button>
      </div>
      <button className={`${styles.item} ${isAll(filter) ? styles.itemActive : ''}`} onClick={pick({})}>
        <AppsRegular />
        全部会话
        <span className={styles.count}>{agents.reduce((n, a) => n + a.count, 0)}</span>
      </button>
      <button className={`${styles.item} ${filter.favorites ? styles.itemActive : ''}`} onClick={pick({ favorites: true })}>
        <StarRegular />
        收藏
      </button>
      <button className={`${styles.item} ${filter.archived ? styles.itemActive : ''}`} onClick={pick({ archived: true })}>
        <ArchiveRegular />
        归档
      </button>
      <div style={{ height: 8 }} />
      {agents.map((a) => (
        <button
          key={a.agent}
          className={`${styles.item} ${filter.agent === a.agent ? styles.itemActive : ''}`}
          onClick={pick({ agent: a.agent })}
        >
          {agentLabel(a.agent)}
          <span className={styles.count}>{a.count}</span>
        </button>
      ))}
      <div className={styles.spacer} />
      <div className={styles.status}>
        <IndexBadge status={status} />
      </div>
    </nav>
  )
}

function IndexBadge({ status }: { status: IndexStatus | null }) {
  if (!status) return <Text size={200}>索引状态未知</Text>
  switch (status.state) {
    case 'ready':
      return <Text size={200}>全文索引就绪({status.docs} 段)</Text>
    case 'building':
      return <Text size={200}>索引构建中…({status.sessions} 会话)</Text>
    case 'failed':
      return (
        <Tooltip content={status.error || ''} relationship="description">
          <Text size={200}>索引失败(降级为快速搜索)</Text>
        </Tooltip>
      )
    default:
      return <Text size={200}>索引未开始</Text>
  }
}

// agent 清单加载(独立小 hook,避免与列表加载耦合)。
function useAgents(): AgentView[] {
  const [agents, setAgents] = useState<AgentView[]>([])
  useEffect(() => {
    sessionService.Agents().then(setAgents).catch(() => {})
  }, [])
  return agents
}
