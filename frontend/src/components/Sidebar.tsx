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
  Input, Button, Dialog, DialogSurface, DialogBody, DialogTitle, DialogContent, DialogActions, Field, MessageBar,
} from '@fluentui/react-components'
import {
  ArchiveRegular,
  ChevronDoubleLeftRegular,
  ChevronDoubleRightRegular,
  AppsRegular,
  StarRegular,
  FolderRegular,
  FolderOpenRegular,
  AddRegular,
  SettingsRegular,
  OpenRegular,
  EditRegular,
  DeleteRegular,
} from '@fluentui/react-icons'
import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react'
import type { AgentView, ProjectView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import { agentMeta } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { NewProjectDialog } from './NewProjectDialog'
import { NewSessionDialog } from './NewSessionDialog'
import { isCliAgent, platformLabel } from '../agentOptions'

type Project = ProjectView
type MenuTarget = { kind: 'agent'; agent: string } | { kind: 'project'; project: Project }
type ProjectAction = { kind: 'rename' | 'remove'; project: Project }

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
  agentScroll: { overflowY: 'auto', maxHeight: '35%', minHeight: '80px', flexShrink: 1 },
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
  projectHeader: {
    display: 'flex', alignItems: 'center', gap: '7px', flexShrink: 0,
    margin: '12px 10px 6px', padding: '7px 2px 0',
    borderTop: '1px solid var(--ad-line)',
    color: 'var(--ad-muted)',
  },
  projectHeading: { margin: 0, fontSize: '12px', lineHeight: '20px', fontWeight: 600 },
  projectTotal: {
    fontSize: '10px', lineHeight: '17px', minWidth: '19px', padding: '0 4px',
    textAlign: 'center', borderRadius: tokens.borderRadiusSmall,
    fontVariantNumeric: 'tabular-nums', color: 'var(--ad-muted)',
    backgroundColor: 'transparent',
  },
  projectAdd: { marginInlineStart: 'auto', flexShrink: 0, color: 'var(--ad-muted)' },
  projectScroll: { flex: 1, overflowY: 'auto', overflowX: 'hidden', minHeight: 0, padding: '0 8px 6px', scrollPaddingBlock: '4px' },
  projectItem: {
    position: 'relative', display: 'flex', alignItems: 'center', gap: '9px',
    width: '100%', minWidth: 0, minHeight: '32px', margin: '1px 0', padding: '5px 8px',
    border: 'none', borderRadius: '6px', backgroundColor: 'transparent', color: 'var(--ad-text)',
    cursor: 'pointer', textAlign: 'left',
    ':hover': { backgroundColor: 'var(--ad-hover)' },
    ':focus-visible': { outline: '2px solid var(--ad-accent)', outlineOffset: '-2px' },
  },
  projectItemActive: {
    backgroundColor: 'var(--ad-selected)', color: 'var(--ad-accent)',
    ':hover': { backgroundColor: 'var(--ad-selected)' },
    ':before': { content: '""', position: 'absolute', left: 0, top: '8px', bottom: '8px', width: '3px', borderRadius: '2px', backgroundColor: 'var(--ad-accent)' },
  },
  projectIcon: { display: 'flex', flexShrink: 0, fontSize: '16px', color: 'var(--ad-muted)' },
  projectIconActive: { color: 'var(--ad-accent)' },
  projectName: {
    flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
    fontSize: '13px', lineHeight: '20px', fontWeight: 400,
  },
  projectCount: {
    flexShrink: 0, minWidth: '16px', fontSize: '12px', lineHeight: '19px',
    fontWeight: 400, fontVariantNumeric: 'tabular-nums', textAlign: 'right', color: 'var(--ad-muted)',
  },
  projectEmpty: {
    display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '5px',
    padding: '16px 8px', border: `1px dashed ${tokens.colorNeutralStroke1}`,
    borderRadius: tokens.borderRadiusMedium, textAlign: 'center', color: tokens.colorNeutralForeground3,
    fontSize: '11px', lineHeight: '18px',
  },
  projectEmptyTitle: { fontWeight: 600, color: tokens.colorNeutralForeground2 },

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
  onNewSession: (project: string, agent: string) => Promise<void>
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
  const [menu, setMenu] = useState<{ target: MenuTarget; x: number; y: number } | null>(null)
  const [action, setAction] = useState<ProjectAction | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState(false)
  const pending = useRef(false)
  const [newProject, setNewProject] = useState<{ agent?: string } | null>(null)
  const [newSession, setNewSession] = useState<{ project?: string; agent?: string } | null>(null)
  const [notice, setNotice] = useState('')

  const contextMenu = (event: MouseEvent<HTMLButtonElement>, target: MenuTarget) => {
    event.preventDefault(); event.stopPropagation()
    setMenu({ target, x: event.clientX, y: event.clientY })
  }
  const keyboardMenu = (event: KeyboardEvent<HTMLButtonElement>, target: MenuTarget) => {
    if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return
    event.preventDefault(); event.stopPropagation()
    const rect = event.currentTarget.getBoundingClientRect()
    setMenu({ target, x: rect.left, y: rect.bottom })
  }
  const openAction = (kind: ProjectAction['kind'], project: Project) => {
    setMenu(null); setActionError(''); setRenameDraft(project.name); setAction({ kind, project })
  }
  const saveAction = async () => {
    if (!action || pending.current) return
    pending.current = true; setBusy(true); setActionError('')
    try {
      if (action.kind === 'rename') await projectService.Rename(action.project.path, renameDraft)
      else {
        await projectService.Remove(action.project.path)
        if (filter.project === action.project.path) onFilter({ ...filter, project: undefined })
        setNotice(`已移除「${action.project.name}」的项目设置，目录、文件和会话均已保留。`)
      }
      onRefresh(); setAction(null)
    } catch (reason) { setActionError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const overlays = <>
    <NewProjectDialog open={!!newProject} current={filter.project} initialAgent={newProject?.agent}
      onCreated={(path, agent) => onFilter({ project: path, agent })} onChanged={onRefresh} onClose={() => setNewProject(null)} />
    <NewSessionDialog open={!!newSession} project={newSession?.project} initialAgent={newSession?.agent}
      onLaunch={onNewSession} onClose={() => setNewSession(null)} />
    {menu ? <Menu open closeOnScroll positioning={{ target: pointTarget(menu), position: 'below', align: 'start', offset: 2 }} onOpenChange={(_, data) => { if (!data.open) setMenu(null) }}>
      <MenuPopover onKeyDown={event => event.stopPropagation()}><MenuList aria-label="新建与项目操作">
        {menu.target.kind === 'agent' ? <>
          <div className="ad-context-heading">{agentMeta(menu.target.agent).label}</div>
          <MenuItem disabled={!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent'} onClick={() => { if (menu.target.kind === 'agent') setNewSession({ agent: menu.target.agent }); setMenu(null) }}>新建会话…</MenuItem>
          <MenuItem disabled={!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent'} onClick={() => { if (menu.target.kind === 'agent') setNewProject({ agent: menu.target.agent }); setMenu(null) }}>新建项目…</MenuItem>
          {!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent' ? <div className="ad-context-heading">此来源暂不支持新建会话</div> : null}
        </> : <>
          <div className="ad-context-heading">{menu.target.project.name}</div>
          <MenuItem icon={<OpenRegular />} onClick={() => { if (menu.target.kind === 'project') setNewSession({ project: menu.target.project.path, agent: menu.target.project.agent || undefined }); setMenu(null) }}>在此新建会话…</MenuItem>
          {menu.target.project.declared ? <>
            <MenuItem icon={<EditRegular />} onClick={() => { if (menu.target.kind === 'project') openAction('rename', menu.target.project) }}>修改显示名称…</MenuItem>
            <MenuItem icon={<DeleteRegular />} onClick={() => { if (menu.target.kind === 'project') openAction('remove', menu.target.project) }}>从项目列表移除…</MenuItem>
          </> : null}
        </>}
      </MenuList></MenuPopover>
    </Menu> : null}
    <Dialog open={!!action} onOpenChange={(_, data) => { if (!data.open && !pending.current) setAction(null) }}>
      <DialogSurface onKeyDown={event => {
        event.stopPropagation()
        if (event.key === 'Enter' && !event.nativeEvent.isComposing && event.nativeEvent.keyCode !== 229 && event.target instanceof HTMLInputElement && action?.kind === 'rename') { event.preventDefault(); void saveAction() }
      }}><DialogBody><DialogTitle>{action?.kind === 'rename' ? '修改项目显示名称' : '从项目列表移除'}</DialogTitle>
        <DialogContent>
          {action?.kind === 'rename' ? <><Field label="项目显示名称" required><Input autoFocus value={renameDraft} disabled={busy} onChange={(_, data) => setRenameDraft(data.value)} /></Field><p>此名称仅用于 AgentDeck 项目列表，不会更改磁盘上的目录名称。</p></> : <p>移除「{action?.project.name}」在 AgentDeck 中的项目设置，包括显示名称和默认智能体。目录、文件和会话均会保留。若该目录已有会话，项目仍会按目录名称显示在列表中。</p>}
          <Field label="项目目录"><Input readOnly value={action?.project.path ?? ''} /></Field>
          {actionError ? <MessageBar intent="error">{actionError}</MessageBar> : null}
        </DialogContent>
        <DialogActions><Button disabled={busy} onClick={() => setAction(null)}>取消</Button><Button appearance="primary" disabled={busy || (action?.kind === 'rename' && !renameDraft.trim())} onClick={() => void saveAction()}>{busy ? (action?.kind === 'rename' ? '保存中…' : '移除中…') : action?.kind === 'rename' ? '保存' : '移除'}</Button></DialogActions>
      </DialogBody></DialogSurface>
    </Dialog>
  </>

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
            onContextMenu={event => contextMenu(event, { kind: 'agent', agent: a.agent })}
            onKeyDown={event => keyboardMenu(event, { kind: 'agent', agent: a.agent })}
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
        {overlays}
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
          onContextMenu={event => contextMenu(event, { kind: 'agent', agent: a.agent })}
          onKeyDown={event => keyboardMenu(event, { kind: 'agent', agent: a.agent })}
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

      <div className={styles.projectHeader}>
        <h2 id="sidebar-projects-heading" className={styles.projectHeading}>项目</h2>
        <span className={styles.projectTotal} title={`${projects.length} 个项目`}>{projects.length}</span>
        <Button className={styles.projectAdd} size="small" appearance="subtle" icon={<AddRegular />} title="新建项目" aria-label="新建项目" onClick={() => setNewProject({})} />
      </div>
      <div className={styles.projectScroll} role="group" aria-labelledby="sidebar-projects-heading">
        {projects.map((p) => {
          const active = filter.project === p.path
          return (
            <button
              key={p.path}
              className={mergeClasses(styles.projectItem, active && styles.projectItemActive)}
              aria-pressed={active}
              aria-label={`${p.name}，${p.count} 个会话，${p.path}`}
              onClick={pick({ project: p.path })}
              onContextMenu={event => contextMenu(event, { kind: 'project', project: p })}
              onKeyDown={event => keyboardMenu(event, { kind: 'project', project: p })}
              title={`${p.name}\n${p.path}\n${p.count} 个会话 · ${p.favorites} 个收藏${p.agent ? `\n默认智能体：${platformLabel(p.agent)}` : ''}`}
            >
              <span aria-hidden="true" className={mergeClasses(styles.projectIcon, active && styles.projectIconActive)}>{active ? <FolderOpenRegular /> : <FolderRegular />}</span>
              <span className={styles.projectName}>{p.name}</span>
              <span aria-hidden="true" className={styles.projectCount}>{p.count}</span>
            </button>
          )
        })}
        {projects.length === 0 ? <div className={styles.projectEmpty}>
          <FolderRegular fontSize={22} aria-hidden="true" />
          <span className={styles.projectEmptyTitle}>暂无项目</span>
          <span>点击右上角 ＋ 新建项目<br />已有会话的项目会自动显示</span>
        </div> : null}
      </div>
      {overlays}
      {notice ? <div role="status" style={{ margin: '0 8px 8px', fontSize: 12, color: tokens.colorNeutralForeground2 }}>{notice}</div> : null}
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
    let active = true
    sessionService.Agents().then(value => { if (active) setAgents(value) }).catch(reason => { if (active) console.error('加载智能体失败', reason) })
    return () => { active = false }
  }, [listVersion])
  return agents
}

// 项目聚合(按会话数排序,来自后端 Projects())。
function useProjects(listVersion: number): ProjectView[] {
  const [ps, setPs] = useState<ProjectView[]>([])
  useEffect(() => {
    let active = true
    sessionService.Projects().then(value => { if (active) setPs(value) }).catch(reason => { if (active) console.error('加载项目失败', reason) })
    return () => { active = false }
  }, [listVersion])
  return ps
}
