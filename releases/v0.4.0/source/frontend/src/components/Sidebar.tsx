// 左导航:视图入口 + Agent 筛选 + 项目区 + 索引状态条。宽度随窗口变化，可折叠为 56px。
import {
  makeStyles,
  mergeClasses,
  tokens,
  Text,
  Menu,
  MenuPopover,
  MenuList,
  MenuItem,
  Input,
} from '@fluentui/react-components'
import {
  ArchiveRegular,
  ChevronDoubleLeftRegular,
  ChevronDoubleRightRegular,
  AppsRegular,
  StarRegular,
  FolderRegular,
  SettingsRegular,
  OpenRegular,
  EditRegular,
  DeleteRegular,
} from '@fluentui/react-icons'
import { useEffect, useState } from 'react'
import type { AgentView, ProjectView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import { agentMeta } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { NewProjectDialog } from './NewProjectDialog'

export type SidebarFilter = {
  agent?: string
  favorites?: boolean
  archived?: boolean
  project?: string // 归一化项目路径,选中后列表按项目过滤
}

const useStyles = makeStyles({
  nav: {
    width: 'var(--ad-nav-width)',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    overflow: 'hidden',
    backgroundColor: tokens.colorNeutralBackground2,
    borderRight: `1px solid ${tokens.colorNeutralStroke2}`,
  },
  navCollapsed: { width: '56px', overflowY: 'auto' },
  agentScroll: { overflowY: 'auto', maxHeight: '42%', minHeight: '80px', flexShrink: 1 },
  item: {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    gap: '9px',
    minHeight: '30px',
    width: 'calc(100% - 16px)',
    margin: '1px 8px',
    padding: '5px 8px',
    border: 'none',
    borderRadius: '6px',
    background: 'transparent',
    cursor: 'pointer',
    textAlign: 'left',
    fontSize: '13px',
    color: tokens.colorNeutralForeground2,
    ':hover': { backgroundColor: tokens.colorNeutralBackground3Hover },
  },
  itemActive: {
    backgroundColor: tokens.colorBrandBackground2,
    color: tokens.colorBrandForeground1,
    fontWeight: 600,
    ':before': {
      content: '""',
      position: 'absolute',
      left: '0',
      top: '8px',
      bottom: '8px',
      width: '3px',
      borderRadius: '2px',
      backgroundColor: tokens.colorBrandBackground,
    },
  },
  count: {
    marginLeft: 'auto',
    fontSize: '12px',
    color: tokens.colorNeutralForeground3,
    backgroundColor: 'transparent',
    fontVariantNumeric: 'tabular-nums',
    padding: '0 2px',
    lineHeight: '19px',
    flexShrink: 0,
  },
  spacer: { flex: 1 },
  status: { padding: '10px 14px', fontSize: '11px', color: tokens.colorNeutralForeground3, borderTop: `1px solid ${tokens.colorNeutralStroke3}` },
  sectionTitle: {
    padding: '18px 16px 7px',
    fontSize: '11px',
    fontWeight: 600,
    letterSpacing: '0.025em',
    color: tokens.colorNeutralForeground3,
  },
  foot: {
    display: 'flex',
    alignItems: 'flex-start',
    flexDirection: 'column',
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
    padding: '8px 12px 6px',
  },
  appName: { fontSize: '12px', fontWeight: 600, color: tokens.colorNeutralForeground3 },
})

type Props = {
  collapsed: boolean
  listVersion: number
  onToggle: () => void
  filter: SidebarFilter
  onFilter: (f: SidebarFilter) => void
  onOpenDiagnostics?: () => void
  /** 在某个项目目录里开新的 agent 会话（后端起 CLI）。 */
  onNewSession: (project: string) => void
  /** 项目清单或标注变化后重新拉取。 */
  onRefresh: () => void
}

function isAll(f: SidebarFilter) { return !f.favorites && !f.archived }

// pointTarget 把「右键的那个坐标点」伪装成一个定位目标：Menu 的
// positioning 只认元素，而这里只有坐标（写法同 SessionContextMenu）。
function pointTarget(point: { x: number; y: number }) {
  return { getBoundingClientRect: () => DOMRect.fromRect({ x: point.x, y: point.y, width: 0, height: 0 }) }
}

export function Sidebar({ collapsed, listVersion, onToggle, filter, onFilter, onOpenDiagnostics, onNewSession, onRefresh }: Props) {
  const styles = useStyles()
  const agents = useAgents(listVersion)
  const projects = useProjects(listVersion)
  const [menu, setMenu] = useState<{ path: string; name: string; declared: boolean; x: number; y: number } | null>(null)
  const [renaming, setRenaming] = useState<{ path: string; name: string } | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [notice, setNotice] = useState('')

  const renameProject = async (m: { path: string; name: string }) => {
    setRenameDraft(m.name)
    setRenaming({ path: m.path, name: m.name })
  }

  const removeProject = async (m: { path: string; name: string }) => {
    try {
      await projectService.Remove(m.path)
      setNotice(`已取消声明「${m.name}」，文件没有动。`)
      onRefresh()
    } catch (reason) {
      setNotice(String(reason))
    }
  }

  const pick = (f: SidebarFilter) => () => {
    if ('agent' in f) onFilter({ ...filter, agent: filter.agent === f.agent ? undefined : f.agent })
    else if ('project' in f) onFilter({ ...filter, project: filter.project === f.project ? undefined : f.project })
    else onFilter({ ...filter, favorites: !!f.favorites, archived: !!f.archived })
  }

  if (collapsed) {
    return (
      <nav className={mergeClasses(styles.nav, styles.navCollapsed)} aria-label="会话导航">
        <button className={styles.item} onClick={onToggle} title="展开导航" style={{ justifyContent: 'center' }}>
          <ChevronDoubleRightRegular />
        </button>
        <button className={mergeClasses(styles.item, isAll(filter) && styles.itemActive)} onClick={pick({})} title="全部会话" style={{ justifyContent: 'center' }}>
          <AppsRegular />
        </button>
        <button className={mergeClasses(styles.item, filter.favorites && styles.itemActive)} onClick={pick({ favorites: true })} title="收藏" style={{ justifyContent: 'center' }}>
          <StarRegular />
        </button>
        <button className={mergeClasses(styles.item, filter.archived && styles.itemActive)} onClick={pick({ archived: true })} title="归档" style={{ justifyContent: 'center' }}>
          <ArchiveRegular />
        </button>
        {agents.map((a) => (
          <button
            key={a.agent}
            className={mergeClasses(styles.item, filter.agent === a.agent && styles.itemActive)}
            onClick={pick({ agent: a.agent })}
            title={`${agentMeta(a.agent).label}(${a.count})`} aria-label={`${agentMeta(a.agent).label}，${a.count} 个会话`}
            style={{ justifyContent: 'center' }}
          >
            <AgentMark agent={a.agent} size={18} />
          </button>
        ))}
        <div style={{ flex: 1 }} />
        <button className={styles.item} onClick={onOpenDiagnostics} title="设置" aria-label="设置" style={{ justifyContent: 'center' }}><SettingsRegular /></button>
      </nav>
    )
  }

  return (
    <nav className={`ad-sidebar ${styles.nav}`} aria-label="会话导航">
      <div className={styles.head}>
        <Text className={styles.appName}>工作空间</Text>
        <button className={styles.item} style={{ width: 'auto', margin: 0, padding: '4px 6px' }} onClick={onToggle} title="折叠导航">
          <ChevronDoubleLeftRegular />
        </button>
      </div>
      <button className={mergeClasses(styles.item, isAll(filter) && styles.itemActive)} onClick={pick({})}>
        <AppsRegular fontSize={16} />
        全部会话
        <span className={styles.count}>{agents.reduce((n, a) => n + a.count, 0)}</span>
      </button>
      <button className={mergeClasses(styles.item, filter.favorites && styles.itemActive)} onClick={pick({ favorites: true })}>
        <StarRegular fontSize={16} />
        收藏
      </button>
      <button className={mergeClasses(styles.item, filter.archived && styles.itemActive)} onClick={pick({ archived: true })}>
        <ArchiveRegular fontSize={16} />
        归档
      </button>

      <div className={styles.sectionTitle}>智能体</div>
      <div className={styles.agentScroll}>
      {agents.map((a) => (
        <button
          key={a.agent}
          className={mergeClasses(styles.item, filter.agent === a.agent && styles.itemActive)}
          onClick={pick({ agent: a.agent })}
          title={`${agentMeta(a.agent).label} · ${a.count} 个会话`}
        >
          <AgentMark agent={a.agent} size={18} />
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{agentMeta(a.agent).label}</span>
          <span className={styles.count}>{a.count}</span>
        </button>
      ))}
      </div>

      <div className={styles.sectionTitle} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingRight: '8px' }}>
        <span>项目</span>
        <NewProjectDialog
          current={filter.project}
          onCreated={p => onFilter({ ...filter, project: p })}
          onChanged={onRefresh}
        />
      </div>
      <div className={styles.projectScroll}>
        {projects.map((p) => (
          <button
            key={p.path}
            className={mergeClasses(styles.item, filter.project === p.path && styles.itemActive)}
            onClick={pick({ project: p.path })}
            onContextMenu={event => { event.preventDefault(); event.stopPropagation(); setMenu({ path: p.path, name: p.name, declared: p.declared, x: event.clientX, y: event.clientY }) }}
            title={`${p.path}(${p.favorites} 收藏)${p.declared ? ' · AgentDeck 新建' : ''}`}
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
      {menu ? (
        <Menu open closeOnScroll
          positioning={{ target: pointTarget(menu), position: 'below', align: 'start', offset: 2 }}
          onOpenChange={(_, d) => { if (!d.open) setMenu(null) }}>
          <MenuPopover onKeyDown={event => event.stopPropagation()}>
            <MenuList aria-label="项目操作菜单">
              <div className="ad-context-heading">{menu.name}</div>
              <MenuItem icon={<OpenRegular />} onClick={() => { const m = menu; setMenu(null); void onNewSession(m.path) }}>
                在此新建会话
              </MenuItem>
              {menu.declared ? <>
                <MenuItem icon={<EditRegular />} onClick={() => { const m = menu; setMenu(null); void renameProject(m) }}>重命名</MenuItem>
                <MenuItem icon={<DeleteRegular />} onClick={() => { const m = menu; setMenu(null); void removeProject(m) }}>取消声明（不删文件）</MenuItem>
              </> : null}
            </MenuList>
          </MenuPopover>
        </Menu>
      ) : null}
      {notice ? <div role="status" style={{ margin: '0 8px 8px', fontSize: 12, color: tokens.colorNeutralForeground2 }}>{notice}</div> : null}
      {renaming ? (
        <div style={{ padding: '6px 8px 10px' }}>
          <Input
            autoFocus
            size="small"
            value={renameDraft}
            aria-label="项目显示名"
            onChange={(_, d) => setRenameDraft(d.value)}
            onKeyDown={async event => {
              if (event.key === 'Enter') {
                const next = renameDraft.trim()
                if (next && next !== renaming.name) {
                  try {
                    await projectService.Rename(renaming.path, next)
                    onRefresh()
                  } catch (reason) { setNotice(String(reason)) }
                }
                setRenaming(null)
              }
              if (event.key === 'Escape') setRenaming(null)
            }}
          />
        </div>
      ) : null}
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
      </div>
    </nav>
  )
}

// agent 清单加载(独立小 hook,避免与列表加载耦合)。
function useAgents(listVersion: number): AgentView[] {
  const [agents, setAgents] = useState<AgentView[]>([])
  useEffect(() => {
    sessionService.Agents().then(setAgents).catch(() => {})
  }, [listVersion])
  return agents
}

// 项目聚合(按会话数排序,来自后端 Projects())。
function useProjects(listVersion: number): ProjectView[] {
  const [ps, setPs] = useState<ProjectView[]>([])
  useEffect(() => {
    sessionService.Projects().then(setPs).catch(() => {})
  }, [listVersion])
  return ps
}
