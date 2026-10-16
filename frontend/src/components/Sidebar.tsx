// 左导航:视图入口 + Agent 筛选 + 项目区 + 索引状态条。可折叠 240↔48px。
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
  FolderRegular,
  SettingsRegular,
} from '@fluentui/react-icons'
import { useEffect, useState } from 'react'
import type { AgentView, IndexStatus, ProjectView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { useIndexStatus } from '../hooks/useIndexStatus'
import { agentMeta } from '../util'
import { AgentAvatar } from './AgentAvatar'

export type SidebarFilter = {
  agent?: string
  favorites?: boolean
  archived?: boolean
  project?: string // 归一化项目路径,选中后列表按项目过滤
}

const useStyles = makeStyles({
  nav: {
    width: '240px',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    backgroundColor: tokens.colorNeutralBackground2,
    borderRight: `1px solid ${tokens.colorNeutralStroke2}`,
  },
  navCollapsed: { width: '48px' },
  item: {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    width: 'calc(100% - 16px)',
    margin: '2px 8px',
    padding: '9px 12px',
    border: 'none',
    borderRadius: '8px',
    background: 'transparent',
    cursor: 'pointer',
    textAlign: 'left',
    fontSize: '14px',
    color: tokens.colorNeutralForeground1,
    ':hover': { backgroundColor: tokens.colorNeutralBackground3Hover },
  },
  itemActive: {
    backgroundColor: tokens.colorNeutralBackground1,
    fontWeight: 600,
    boxShadow: tokens.shadow2,
    ':before': {
      content: '""',
      position: 'absolute',
      left: '-8px',
      top: '6px',
      bottom: '6px',
      width: '3px',
      borderRadius: '2px',
      backgroundColor: tokens.colorBrandBackground,
    },
  },
  count: {
    marginLeft: 'auto',
    fontSize: '12px',
    color: tokens.colorNeutralForeground3,
    backgroundColor: tokens.colorNeutralBackground3,
    borderRadius: '10px',
    padding: '0 8px',
    lineHeight: '19px',
    flexShrink: 0,
  },
  spacer: { flex: 1 },
  status: { padding: '10px 14px', fontSize: '11px', color: tokens.colorNeutralForeground3, borderTop: `1px solid ${tokens.colorNeutralStroke3}` },
  sectionTitle: {
    padding: '16px 18px 6px',
    fontSize: '12.5px',
    fontWeight: 600,
    letterSpacing: '0.01em',
    color: tokens.colorNeutralForeground3,
  },
  foot: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '10px 14px',
    borderTop: `1px solid ${tokens.colorNeutralStroke3}`,
    fontSize: '11px',
    color: tokens.colorNeutralForeground3,
  },
  projectScroll: { flex: 1, overflowY: 'auto', minHeight: 0 },
  head: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '10px 10px 6px',
  },
  appName: { fontSize: '12px', fontWeight: 600, color: tokens.colorNeutralForeground3 },
})

type Props = {
  collapsed: boolean
  onToggle: () => void
  filter: SidebarFilter
  onFilter: (f: SidebarFilter) => void
  onOpenDiagnostics?: () => void
}

function isAll(f: SidebarFilter) { return !f.agent && !f.favorites && !f.archived && !f.project }

export function Sidebar({ collapsed, onToggle, filter, onFilter, onOpenDiagnostics }: Props) {
  const styles = useStyles()
  const agents = useAgents()
  const projects = useProjects()
  const status = useIndexStatus()

  const pick = (f: SidebarFilter) => () => onFilter(f)

  if (collapsed) {
    return (
      <nav className={`${styles.nav} ${styles.navCollapsed}`}>
        <button className={styles.item} onClick={onToggle} title="展开导航" style={{ justifyContent: 'center' }}>
          <ChevronDoubleRightRegular />
        </button>
        <button className={`${styles.item} ${isAll(filter) ? styles.itemActive : ''}`} onClick={pick({})} title="全部会话" style={{ justifyContent: 'center' }}>
          <AppsRegular />
        </button>
        <button className={`${styles.item} ${filter.favorites ? styles.itemActive : ''}`} onClick={pick({ favorites: true })} title="收藏" style={{ justifyContent: 'center' }}>
          <StarRegular />
        </button>
        <button className={`${styles.item} ${filter.archived ? styles.itemActive : ''}`} onClick={pick({ archived: true })} title="归档" style={{ justifyContent: 'center' }}>
          <ArchiveRegular />
        </button>
        {agents.map((a) => (
          <button
            key={a.agent}
            className={`${styles.item} ${filter.agent === a.agent ? styles.itemActive : ''}`}
            onClick={pick({ agent: a.agent })}
            title={`${agentMeta(a.agent).label}(${a.count})`}
            style={{ justifyContent: 'center' }}
          >
            <AgentAvatar agent={a.agent} size={18} />
          </button>
        ))}
      </nav>
    )
  }

  return (
    <nav className={styles.nav}>
      <div className={styles.head}>
        <Text className={styles.appName}>导航</Text>
        <button className={styles.item} style={{ width: 'auto', margin: 0, padding: '4px 6px' }} onClick={onToggle} title="折叠导航">
          <ChevronDoubleLeftRegular />
        </button>
      </div>
      <button className={`${styles.item} ${isAll(filter) ? styles.itemActive : ''}`} onClick={pick({})}>
        <AppsRegular fontSize={16} />
        全部会话
        <span className={styles.count}>{agents.reduce((n, a) => n + a.count, 0)}</span>
      </button>
      <button className={`${styles.item} ${filter.favorites ? styles.itemActive : ''}`} onClick={pick({ favorites: true })}>
        <StarRegular fontSize={16} />
        收藏
      </button>
      <button className={`${styles.item} ${filter.archived ? styles.itemActive : ''}`} onClick={pick({ archived: true })}>
        <ArchiveRegular fontSize={16} />
        归档
      </button>

      <div className={styles.sectionTitle}>Agent</div>
      {agents.map((a) => (
        <button
          key={a.agent}
          className={`${styles.item} ${filter.agent === a.agent ? styles.itemActive : ''}`}
          onClick={pick({ agent: a.agent })}
          title={`${agentMeta(a.agent).label} · ${a.count} 个会话`}
        >
          <AgentAvatar agent={a.agent} size={18} />
          {agentMeta(a.agent).label}
          <span className={styles.count}>{a.count}</span>
        </button>
      ))}

      <div className={styles.sectionTitle}>工作区</div>
      <div className={styles.projectScroll}>
        {projects.map((p) => (
          <button
            key={p.path}
            className={`${styles.item} ${filter.project === p.path ? styles.itemActive : ''}`}
            onClick={pick({ project: p.path })}
            title={`${p.path}(${p.favorites} 收藏)`}
          >
            <FolderRegular fontSize={16} style={{ flexShrink: 0, color: tokens.colorNeutralForeground3 }} />
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {p.drive ? `${p.drive} ` : ''}{p.name}
            </span>
            <span className={styles.count}>{p.count}</span>
          </button>
        ))}
        {projects.length === 0 ? <div className={styles.status}>暂无项目</div> : null}
      </div>
      <div className={styles.foot}>
        {onOpenDiagnostics ? (
          <button
            className={styles.item}
            style={{ width: 'auto', margin: 0, padding: '4px 6px' }}
            onClick={onOpenDiagnostics}
            title="能力诊断 / 备份恢复"
          >
            <SettingsRegular fontSize={15} />
            设置
          </button>
        ) : null}
        <IndexBadge status={status} />
      </div>
    </nav>
  )
}

function IndexBadge({ status }: { status: IndexStatus | null }) {
  if (!status) return <Text size={200}>索引状态未知</Text>
  switch (status.state) {
    case 'ready':
      return <Text size={200}>● 全文索引就绪 · {status.docs} 段</Text>
    case 'building':
      return <Text size={200}>◐ 索引构建中 · {status.sessions} 会话</Text>
    case 'failed':
      return (
        <Tooltip content={status.error || ''} relationship="description">
          <Text size={200}>○ 索引失败 · 已降级为快速搜索</Text>
        </Tooltip>
      )
    default:
      return <Text size={200}>○ 索引未开始</Text>
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

// 项目聚合(按会话数排序,来自后端 Projects())。
function useProjects(): ProjectView[] {
  const [ps, setPs] = useState<ProjectView[]>([])
  useEffect(() => {
    sessionService.Projects().then(setPs).catch(() => {})
  }, [])
  return ps
}
