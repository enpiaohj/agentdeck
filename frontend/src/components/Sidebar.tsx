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
  Input, Button, Field,
} from '@fluentui/react-components'
import {
  ArchiveRegular,
  ChevronDoubleLeftRegular,
  ChevronDoubleRightRegular,
  ChevronDownRegular,
  ChevronRightRegular,
  AppsRegular,
  StarRegular,
  FolderRegular,
  FolderOpenRegular,
  AddRegular,
  EyeRegular,
  EyeOffRegular,
  SettingsRegular,
  OpenRegular,
  EditRegular,
  DeleteRegular,
  PinRegular,
  PinOffRegular,
  PinFilled,
} from '@fluentui/react-icons'
import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react'
import { changeProjectLocation } from './ProjectLocationDialog'
import type { AgentView, NavCounts, ProjectView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { usePreferences } from '../hooks/usePreferences'
import { agentRange } from '../sidebarSelection'
import { isPinned, pinnedFirst, togglePins } from '../pins'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { agentMeta } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { isCliAgent, platformLabel } from '../agentOptions'
import { EmptyState } from './EmptyState'
import { FormDialog } from './FormDialog'
import type { Notify } from '../hooks/useToasts'

type Project = ProjectView

// 项目区默认列出的个数；只多出一两个时直接全列，免得「显示全部」只多一行
const PROJECT_PREVIEW = 8
type MenuTarget = { kind: 'agent'; agent: string } | { kind: 'project'; project: Project }
type ProjectAction = { kind: 'rename' | 'remove'; project: Project }

export type SidebarFilter = {
  /** 选中的智能体；Ctrl+单击可多选，列表取并集。空或缺省表示不按智能体过滤。 */
  agents?: string[]
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
  // 折叠态同样：展开按钮在顶、「设置」在底，都不随中间的图标列滚动
  navCollapsed: { width: '56px' },
  // 整个导航只有这一个滚动区：此前智能体区、项目区和整栏各有一条滚动条，嵌套
  // 滚动既难看又难用。分区标题吸顶，「设置」固定在滚动区之外的底部。
  scroll: { flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', paddingBottom: '6px' },
  sectionHead: {
    position: 'sticky', top: 0, zIndex: 1,
    display: 'flex', alignItems: 'center', gap: '2px',
    padding: '12px 10px 4px 8px',
    backgroundColor: tokens.colorNeutralBackground2,
  },
  sectionToggle: {
    display: 'flex', alignItems: 'center', gap: '4px', flex: 1, minWidth: 0,
    padding: '3px 4px', border: 'none', borderRadius: '4px', background: 'transparent', cursor: 'pointer',
    fontSize: '11px', fontWeight: 600, letterSpacing: '0.025em', color: 'var(--ad-muted)', textAlign: 'left',
    ':hover': { backgroundColor: tokens.colorNeutralBackground3Hover, color: tokens.colorNeutralForeground2 },
    ':focus-visible': { outline: '2px solid var(--ad-accent)', outlineOffset: '-2px' },
  },
  sectionCount: { fontWeight: 400, fontVariantNumeric: 'tabular-nums' },
  more: {
    display: 'block', width: 'calc(100% - 16px)', margin: '2px 8px 0', padding: '5px 8px 5px 33px',
    border: 'none', borderRadius: '6px', background: 'transparent', cursor: 'pointer', textAlign: 'left',
    fontSize: '12px', color: tokens.colorBrandForeground1,
    ':hover': { backgroundColor: tokens.colorNeutralBackground3Hover },
  },
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
    color: 'var(--ad-muted)',
    backgroundColor: 'transparent',
    fontVariantNumeric: 'tabular-nums',
    padding: '0 2px',
    lineHeight: '19px',
    flexShrink: 0,
  },
  status: { padding: '10px 14px', fontSize: '11px', color: 'var(--ad-muted)', borderTop: `1px solid ${tokens.colorNeutralStroke3}` },
  foot: {
    display: 'flex',
    alignItems: 'flex-start',
    flexDirection: 'column',
    gap: '8px',
    padding: '10px 14px',
    borderTop: `1px solid ${tokens.colorNeutralStroke3}`,
    fontSize: '11px',
    color: 'var(--ad-muted)',
  },
  projectAdd: { flexShrink: 0, color: 'var(--ad-muted)' },
  projectList: { padding: '0 8px' },
  // 固定入口：全部会话 / 收藏 / 归档始终可见，与下方滚动区以细线分开
  pinned: { flexShrink: 0, paddingBottom: '6px', borderBottom: `1px solid ${tokens.colorNeutralStroke3}` },
  footCollapsed: { flexShrink: 0, padding: '6px 0', borderTop: `1px solid ${tokens.colorNeutralStroke3}` },
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
  head: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '8px 12px 6px',
  },
  appName: { fontSize: '12px', fontWeight: 600, color: 'var(--ad-muted)' },
})

type Props = {
  collapsed: boolean
  listVersion: number
  onToggle: () => void
  filter: SidebarFilter
  /** 侧栏各入口默认单选（单击替换整个筛选），Ctrl+单击才把该维度并入当前筛选。 */
  onFilter: (f: SidebarFilter) => void
  /** 点「全部会话」：清除全部筛选（智能体 / 项目 / 收藏 / 归档、顶栏搜索词与列表内筛选）。 */
  onSelectAll: () => void
  onOpenDiagnostics?: () => void
  /** 打开「新建会话」弹窗；弹窗由 App 持有，快捷键与右键菜单走同一条路。 */
  onNewSession: (project?: string, agent?: string) => void
  /** 打开「新建项目」弹窗。 */
  onNewProject: (agent?: string) => void
  notify: Notify
  /** 项目清单或标注变化后重新拉取。 */
  onRefresh: () => void
}

// isAll 说当前是不是「全部会话」这个绝对视图。
//
// 四个维度（智能体 / 项目 / 收藏 / 归档）都为空才算：只要缺一项，点过项目或
// 智能体之后「全部会话」仍会高亮，与已选的项目行同时亮着，看起来就是多字段
// 筛选状态，而列表里其实只剩那个项目。
function isAll(f: SidebarFilter) { return !f.favorites && !f.archived && !f.agents?.length && !f.project }

// folderName 取目录名，用于「使用目录名」这一动作。
function folderName(path: string): string {
  const parts = path.replace(/[\\/]+$/, '').split(/[\\/]/)
  return parts[parts.length - 1] ?? ''
}

// pointTarget 把「右键的那个坐标点」伪装成一个定位目标：Menu 的
// positioning 只认元素，而这里只有坐标（写法同 SessionContextMenu）。
function pointTarget(point: { x: number; y: number }) {
  return { getBoundingClientRect: () => DOMRect.fromRect({ x: point.x, y: point.y, width: 0, height: 0 }) }
}

export function Sidebar({ collapsed, listVersion, onToggle, filter, onFilter, onSelectAll, onOpenDiagnostics, onNewSession, onNewProject, notify, onRefresh }: Props) {
  const styles = useStyles()
  const agentList = useAgents(listVersion)
  const projectList = useProjects(listVersion)
  // 声明过却还没有会话的项目（计数 0）可收起：偏好在「设置 → 工作台」里也可改，默认不收，免得刚建好的项目消失。
  // 正被用作筛选的项目始终显示。
  const { preferences, update } = usePreferences()
  // 置顶的智能体与项目排在各自分区最前面（项目路径大小写不敏感）
  const agents = pinnedFirst(agentList, a => a.agent, preferences.pinnedAgents)
  const allProjects = pinnedFirst(projectList, p => p.path, preferences.pinnedProjects, true)
  const agentPinned = (agent: string) => isPinned(preferences.pinnedAgents, agent)
  const projectPinned = (path: string) => isPinned(preferences.pinnedProjects, path, true)
  const hideEmpty = preferences.hideEmptyProjects
  const toggleHideEmpty = () => update({ hideEmptyProjects: !hideEmpty })
  const emptyProjects = allProjects.filter(p => p.count === 0).length
  const projects = hideEmpty ? allProjects.filter(p => p.count > 0 || filter.project === p.path) : allProjects
  // 项目默认只列前几个，其余经「显示全部」展开；当前筛选的项目始终列出
  const agentsOpen = !preferences.agentsCollapsed
  const projectsOpen = !preferences.projectsCollapsed
  const showAll = preferences.showAllProjects || projects.length <= PROJECT_PREVIEW + 2
  const shownProjects = showAll ? projects
    : projects.filter((p, index) => index < PROJECT_PREVIEW || filter.project === p.path)
  const counts = useCounts(listVersion)
  const [menu, setMenu] = useState<{ target: MenuTarget; x: number; y: number } | null>(null)
  const [linkedPaths, setLinkedPaths] = useState<string[]>([])
  useEffect(() => {
    let active = true
    setLinkedPaths([])
    if (menu?.target.kind === 'project') void projectService.Locations().then(rows => {
      if (active) setLinkedPaths(rows.map(row => row.original.toLowerCase()))
    }).catch(reason => { if (active) notify(String(reason), { tone: 'error' }) })
    return () => { active = false }
  }, [menu, listVersion, notify])
  const [action, setAction] = useState<ProjectAction | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState(false)
  const pending = useRef(false)
  // held 记住最近一次操作的项目：关闭动画期间 action 已经清空，但字段还要有
  // 内容可渲染，否则会看到一瞬空白弹窗。
  const held = useRef<Project | null>(null)
  const agentAnchor = useRef<string | null>(null)
  // Shift+单击按钮时浏览器会顺带选中两次点击之间的文字，连选时拦掉
  const noShiftSelect = (event: MouseEvent<HTMLButtonElement>) => { if (event.shiftKey) event.preventDefault() }

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
    setMenu(null); setActionError(''); setRenameDraft(project.name); held.current = project; setAction({ kind, project })
  }
  const closeAction = () => { setAction(null); setActionError('') }
  const saveRename = async (project: Project, name: string) => {
    if (pending.current) return
    pending.current = true; setBusy(true); setActionError('')
    try {
      await projectService.Rename(project.path, name)
      onRefresh(); setAction(null)
      notify(`项目显示名称已改为「${name.trim()}」，会话列表同步生效。`, {
        tone: 'success',
        action: { label: '撤销', run: () => void undoRename(project, project.name) },
      })
    } catch (reason) { setActionError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  // undoRename 只是把名字改回去，不做别的动作；失败要说话，不能让用户以为撤销成功。
  const undoRename = async (project: Project, name: string) => {
    try {
      await projectService.Rename(project.path, name)
      onRefresh()
      notify(`已恢复为「${name}」。`, { tone: 'success' })
    } catch (reason) { notify(`撤销失败：${String(reason)}`, { tone: 'error' }) }
  }
  const removeProject = async (project: Project) => {
    if (pending.current) return
    pending.current = true; setBusy(true); setActionError('')
    try {
      await projectService.Remove(project.path)
      if (filter.project === project.path) onFilter({ ...filter, project: undefined })
      onRefresh(); setAction(null)
      notify(`已从项目列表移除「${project.name}」，目录、文件与会话均已保留。`, { tone: 'success' })
    } catch (reason) { setActionError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }

  const shown = action?.project ?? held.current
  const renamed = action?.kind === 'rename' ? shown : null
  const removed = action?.kind === 'remove' ? shown : null

  const renameError = (() => {
    const value = renameDraft.trim()
    if (!value) return '显示名称不能为空；可用「使用目录名」恢复为目录名称。'
    if (value.length > 200) return '显示名称不能超过 200 个字符。'
    if (/[\u0000-\u001f\u007f]/.test(renameDraft)) return '显示名称不能包含换行等控制字符。'
    return ''
  })()

  const overlays = <>
    {menu ? <Menu open closeOnScroll positioning={{ target: pointTarget(menu), position: 'below', align: 'start', offset: 2 }} onOpenChange={(_, data) => { if (!data.open) setMenu(null) }}>
      <MenuPopover onKeyDown={event => event.stopPropagation()}><MenuList aria-label="新建与项目操作">
        {menu.target.kind === 'agent' ? <>
          <div className="ad-context-heading">{agentMeta(menu.target.agent).label}</div>
          <MenuItem icon={agentPinned(menu.target.agent) ? <PinOffRegular /> : <PinRegular />} onClick={() => { if (menu.target.kind === 'agent') update({ pinnedAgents: togglePins(preferences.pinnedAgents, [menu.target.agent]) }); setMenu(null) }}>{agentPinned(menu.target.agent) ? '取消置顶' : '置顶'}</MenuItem>
          <MenuItem disabled={!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent'} onClick={() => { if (menu.target.kind === 'agent') onNewSession(undefined, menu.target.agent); setMenu(null) }}>新建会话…</MenuItem>
          <MenuItem disabled={!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent'} onClick={() => { if (menu.target.kind === 'agent') onNewProject(menu.target.agent); setMenu(null) }}>新建项目…</MenuItem>
          {!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent' ? <div className="ad-context-heading">此来源暂不支持新建会话</div> : null}
        </> : <>
          <div className="ad-context-heading">{menu.target.project.name}</div>
          <MenuItem icon={projectPinned(menu.target.project.path) ? <PinOffRegular /> : <PinRegular />} onClick={() => { if (menu.target.kind === 'project') update({ pinnedProjects: togglePins(preferences.pinnedProjects, [menu.target.project.path], true) }); setMenu(null) }}>{projectPinned(menu.target.project.path) ? '取消置顶' : '置顶'}</MenuItem>
          <MenuItem icon={<OpenRegular />} onClick={() => { if (menu.target.kind === 'project') onNewSession(menu.target.project.path, menu.target.project.agent || undefined); setMenu(null) }}>在此新建会话…</MenuItem>
          <MenuItem onClick={() => { if (menu.target.kind !== 'project') return; const path = menu.target.project.path; setMenu(null); void projectService.Open(path).catch(reason => notify(String(reason), { tone: 'error' })) }}>打开项目目录</MenuItem>
          <MenuItem onClick={() => { if (menu.target.kind !== 'project') return; const path = menu.target.project.path; setMenu(null); void changeProjectLocation(path).then(saved => { if (saved) { onRefresh(); notify('项目位置关联已保存。', { tone: 'success' }) } }).catch(reason => notify(String(reason), { tone: 'error' })) }}>更改项目位置…</MenuItem>
          {linkedPaths.includes(menu.target.project.path.toLowerCase()) ? <MenuItem onClick={() => { if (menu.target.kind !== 'project') return; const path = menu.target.project.path; setMenu(null); void projectService.RemoveLocation(path).then(() => { onRefresh(); notify('项目位置关联已移除。后续操作将使用原项目目录。', { tone: 'success' }) }).catch(reason => notify(String(reason), { tone: 'error' })) }}>移除位置关联</MenuItem> : null}
          {menu.target.project.declared ? <>
            <MenuItem icon={<EditRegular />} onClick={() => { if (menu.target.kind === 'project') openAction('rename', menu.target.project) }}>修改显示名称…</MenuItem>
            <MenuItem icon={<DeleteRegular />} onClick={() => { if (menu.target.kind === 'project') openAction('remove', menu.target.project) }}>从项目列表移除…</MenuItem>
          </> : null}
        </>}
      </MenuList></MenuPopover>
    </Menu> : null}
    <FormDialog
      open={!!renamed}
      title="修改项目显示名称"
      intro={<p className="ad-form-note">会话列表与项目列表都会用这个名称；磁盘上的目录名不会改动。</p>}
      submitLabel="保存" busyLabel="保存中…" busy={busy} error={actionError}
      submitDisabled={!!renameError || renameDraft.trim() === (renamed?.name ?? '')}
      onSubmit={() => { if (renamed && !renameError) void saveRename(renamed, renameDraft) }}
      onClose={closeAction}
    >
      <Field label="项目显示名称" required validationMessage={renameError || `当前为「${renamed?.name ?? ''}」。`} validationState={renameError ? 'error' : 'none'}>
        <div className="ad-form-row">
          <Input autoFocus aria-label="项目显示名称" value={renameDraft} disabled={busy} onChange={(_, data) => setRenameDraft(data.value)} style={{ flex: 1, minWidth: 0 }} />
          <Button disabled={busy} onClick={() => setRenameDraft(folderName(renamed?.path ?? ''))}>使用目录名</Button>
        </div>
      </Field>
      <Field label="项目目录"><Input readOnly aria-label="项目目录" value={renamed?.path ?? ''} /></Field>
    </FormDialog>
    <FormDialog
      open={!!removed}
      title="从项目列表移除"
      intro={<p className="ad-form-note">移除「{removed?.name}」在 AgentDeck 中的项目设置，包括显示名称和默认智能体。目录、文件和会话均会保留{removed?.declared ? '' : '；该目录已有会话，移除后仍会显示在列表中'}。此操作不会删除任何数据。</p>}
      submitLabel="从项目列表移除" busyLabel="移除中…" busy={busy} error={actionError}
      submitAppearance="destructive"
      onSubmit={() => { if (removed) void removeProject(removed) }}
      onClose={closeAction}
    >
      <Field label="项目目录"><Input readOnly aria-label="项目目录" value={removed?.path ?? ''} /></Field>
    </FormDialog>
  </>

  // pick 是侧栏各入口唯一的改动点：默认**单选**，按住 Ctrl（macOS 上为 Cmd）
  // 单击才**叠加**。
  //
  // 此前一律用 `{ ...filter, ... }` 扩散：点了项目再点智能体，两个字段同时
  // 生效，列表变成两者的交集（常常只剩几条甚至空），侧栏同时高亮「全部会话」
  // 「智能体」「项目」三行，看起来就是意外进入的多字段筛选状态。
  // 现在：普通单击替换整个筛选（只保留点中的那一项），Ctrl+单击把该维度并入
  // 当前筛选、再点一次只去掉这一项——「某项目的收藏」这类组合仍然可达，但是
  // 显式动作。再次单击已选中的唯一项退回全集，与「全部会话」一致。
  // 智能体同一维度内也可多选：Ctrl+单击在已选智能体里增减，列表取并集；
  // 普通单击仍是单选（替换整个筛选），再点一次唯一选中的那个退回全集。
  const pick = (f: SidebarFilter) => (event: MouseEvent<HTMLButtonElement>) => {
    const add = event.ctrlKey || event.metaKey
    if (f.agents) {
      const agent = f.agents[0]
      const current = filter.agents ?? []
      // Shift+单击：从锚点（上次普通单击或 Ctrl+单击的智能体）连选到这一个，按侧栏顺序；
      // Ctrl+Shift 把这一段并入已选。与会话列表的多选手势一致；锚点在连选时不动。
      if (event.shiftKey) {
        onFilter({ ...filter, agents: agentRange(agents.map(a => a.agent), agentAnchor.current, current, agent, add) })
        return
      }
      agentAnchor.current = agent
      if (add) {
        const agents = current.includes(agent) ? current.filter(a => a !== agent) : [...current, agent]
        const next: SidebarFilter = { ...filter, agents }
        if (!agents.length) delete next.agents
        onFilter(next)
      } else {
        const only = current.length === 1 && current[0] === agent
        onFilter(only ? {} : { agents: [agent] })
      }
      return
    }
    const key = 'project' in f ? 'project' : f.favorites ? 'favorites' : 'archived'
    const on = key === 'project' ? filter.project === f.project
      : key === 'favorites' ? !!filter.favorites
      : !!filter.archived
    const next: SidebarFilter = add ? { ...filter } : {}
    if (on) delete next[key]
    else if (key === 'project') next.project = f.project
    else if (key === 'favorites') next.favorites = true
    else next.archived = true
    onFilter(next)
  }

  if (collapsed) {
    return (
      <nav className={mergeClasses(styles.nav, styles.navCollapsed)} aria-label="会话导航">
        <button className={styles.item} onClick={onToggle} title="展开导航" style={{ justifyContent: 'center' }}>
          <ChevronDoubleRightRegular />
        </button>
        <div className={styles.pinned}>
        <button className={mergeClasses(styles.item, isAll(filter) && styles.itemActive)} onClick={onSelectAll} title="全部会话" style={{ justifyContent: 'center' }}>
          <AppsRegular />
        </button>
        <button className={mergeClasses(styles.item, filter.favorites && styles.itemActive)} onClick={pick({ favorites: true })} title="收藏（Ctrl+单击与其他筛选叠加）" style={{ justifyContent: 'center' }}>
          <StarRegular />
        </button>
        <button className={mergeClasses(styles.item, filter.archived && styles.itemActive)} onClick={pick({ archived: true })} title="归档（Ctrl+单击与其他筛选叠加）" style={{ justifyContent: 'center' }}>
          <ArchiveRegular />
        </button>
        </div>
        <div className={`ad-nav-scroll ${styles.scroll}`}>
        {agents.map((a) => (
          <button
            onContextMenu={event => contextMenu(event, { kind: 'agent', agent: a.agent })}
            onKeyDown={event => keyboardMenu(event, { kind: 'agent', agent: a.agent })}
            key={a.agent}
            className={mergeClasses(styles.item, filter.agents?.includes(a.agent) && styles.itemActive)}
            onClick={pick({ agents: [a.agent] })}
          onMouseDown={noShiftSelect}
            title={`${agentMeta(a.agent).label}(${a.count})`} aria-label={`${agentMeta(a.agent).label}，${a.count} 个会话`}
            style={{ justifyContent: 'center' }}
          >
            <AgentMark agent={a.agent} size={18} />
          </button>
        ))}
        </div>
        <div className={styles.footCollapsed}>
          <button className={styles.item} onClick={onOpenDiagnostics} title="设置" aria-label="设置" style={{ justifyContent: 'center' }}><SettingsRegular /></button>
        </div>
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
      <div className={styles.pinned}>
      <button className={mergeClasses(styles.item, isAll(filter) && styles.itemActive)} onClick={onSelectAll}>
        <AppsRegular fontSize={16} />
        全部会话
        {/* 计数来自后端 Counts()：与点进去看到的行数同口径（归档另算），
            不是把各 agent 的数字相加——那会把归档的也算进来。 */}
        {counts ? <span className={styles.count} title="未归档的会话数">{counts.sessions}</span> : null}
      </button>
      <button className={mergeClasses(styles.item, filter.favorites && styles.itemActive)} onClick={pick({ favorites: true })} title="收藏（Ctrl+单击与其他筛选叠加）">
        <StarRegular fontSize={16} />
        收藏
        {counts ? <span className={styles.count} title="未归档的收藏">{counts.favorites}</span> : null}
      </button>
      <button className={mergeClasses(styles.item, filter.archived && styles.itemActive)} onClick={pick({ archived: true })} title="归档（Ctrl+单击与其他筛选叠加）">
        <ArchiveRegular fontSize={16} />
        归档
        {counts ? <span className={styles.count} title="已归档的会话">{counts.archived}</span> : null}
      </button>
      </div>
      <div className={`ad-nav-scroll ${styles.scroll}`}>

      <div className={styles.sectionHead}>
        <button className={styles.sectionToggle} aria-expanded={agentsOpen} aria-controls="sidebar-agents" onClick={() => update({ agentsCollapsed: agentsOpen })} title={agentsOpen ? '收起智能体' : '展开智能体'}>
          {agentsOpen ? <ChevronDownRegular fontSize={12} /> : <ChevronRightRegular fontSize={12} />}
          智能体 <span className={styles.sectionCount}>{agents.length}</span>
        </button>
      </div>
      {agentsOpen ? <div id="sidebar-agents" role="group" aria-label="智能体">
      {agents.map((a) => (
        <button
          onContextMenu={event => contextMenu(event, { kind: 'agent', agent: a.agent })}
          onKeyDown={event => keyboardMenu(event, { kind: 'agent', agent: a.agent })}
          key={a.agent}
          className={mergeClasses(styles.item, filter.agents?.includes(a.agent) && styles.itemActive)}
          onClick={pick({ agents: [a.agent] })}
          onMouseDown={noShiftSelect}
          title={`${agentMeta(a.agent).label} · ${a.count} 个会话 · 右键新建会话或项目 · Ctrl+单击多选 · Shift+单击连选`}
        >
          <AgentMark agent={a.agent} size={18} />
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{agentMeta(a.agent).label}</span>
          {agentPinned(a.agent) ? <PinFilled fontSize={11} role="img" aria-label="已置顶" style={{ flexShrink: 0, color: tokens.colorBrandForeground1 }} /> : null}
          <span className={styles.count}>{a.count}</span>
        </button>
      ))}
      </div> : null}

      <div className={styles.sectionHead}>
        <button id="sidebar-projects-heading" className={styles.sectionToggle} aria-expanded={projectsOpen} aria-controls="sidebar-projects" onClick={() => update({ projectsCollapsed: projectsOpen })}
          title={`${projectsOpen ? '收起' : '展开'}项目 · ${hideEmpty ? `显示 ${projects.length} 个，另有 ${allProjects.length - projects.length} 个空项目已隐藏` : `${allProjects.length} 个项目`}`}>
          {projectsOpen ? <ChevronDownRegular fontSize={12} /> : <ChevronRightRegular fontSize={12} />}
          项目 <span className={styles.sectionCount}>{hideEmpty ? `${projects.length}/${allProjects.length}` : allProjects.length}</span>
        </button>
        {emptyProjects > 0 ? <Button size="small" appearance="subtle" icon={hideEmpty ? <EyeRegular /> : <EyeOffRegular />} title={hideEmpty ? `显示 ${emptyProjects} 个没有会话的项目` : `隐藏 ${emptyProjects} 个没有会话的项目`} aria-label={hideEmpty ? '显示空项目' : '隐藏空项目'} aria-pressed={hideEmpty} onClick={toggleHideEmpty} /> : null}
        <Button className={styles.projectAdd} data-return-focus="new-project" size="small" appearance="subtle" icon={<AddRegular />} title="新建项目 (Ctrl+Shift+N)" aria-label="新建项目" onClick={() => onNewProject()} />
      </div>
      {projectsOpen ? <div id="sidebar-projects" className={styles.projectList} role="group" aria-labelledby="sidebar-projects-heading">
        {shownProjects.map((p) => {
          const active = filter.project === p.path
          const detail = `${p.name}\n${p.path}\n${p.count} 个会话 · ${p.favorites} 个收藏${p.archived ? ` · ${p.archived} 个归档（在「归档」中查看）` : ''}${p.agent ? `\n默认智能体：${platformLabel(p.agent)}` : ''}\nCtrl+单击：与当前筛选叠加\n右键：新建会话 / 修改设置`
          return (
            <div key={p.path} className="ad-project-row">
              <button
                className={mergeClasses(styles.projectItem, active && styles.projectItemActive)}
                aria-pressed={active}
                aria-label={`${p.name}，${p.count} 个会话，${p.path}`}
                onClick={pick({ project: p.path })}
                onContextMenu={event => contextMenu(event, { kind: 'project', project: p })}
                onKeyDown={event => keyboardMenu(event, { kind: 'project', project: p })}
                title={detail}
              >
                <span aria-hidden="true" className={mergeClasses(styles.projectIcon, active && styles.projectIconActive)}>{active ? <FolderOpenRegular /> : <FolderRegular />}</span>
                <span className={styles.projectName}>{p.name}</span>
                {projectPinned(p.path) ? <PinFilled fontSize={11} role="img" aria-label="已置顶" style={{ flexShrink: 0, color: tokens.colorBrandForeground1 }} /> : null}
                <span aria-hidden="true" className={mergeClasses(styles.projectCount, 'ad-project-count')}>{p.count}</span>
              </button>
              <button
                className="ad-project-add"
                title={`在「${p.name}」新建会话`}
                aria-label={`在「${p.name}」新建会话`}
                onClick={() => onNewSession(p.path, p.agent || undefined)}
              >
                <AddRegular fontSize={14} />
              </button>
            </div>
          )
        })}
        {projects.length === 0 ? <EmptyState icon={<FolderRegular fontSize={22} />} title="暂无项目" compact>
          点「＋」新建项目并指定默认智能体；已有会话的目录会自动出现在这里。
        </EmptyState> : null}
      </div> : null}
      {projectsOpen && projects.length > PROJECT_PREVIEW + 2 ? (
        <button className={styles.more} aria-controls="sidebar-projects" onClick={() => update({ showAllProjects: !preferences.showAllProjects })}>
          {preferences.showAllProjects ? '收起' : `显示全部 ${projects.length} 个项目`}
        </button>
      ) : null}
      </div>
      {overlays}
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

// 侧栏三条入口的计数。null 表示还没读到，此时不显示数字——先显示 0 再跳到
// 真实值，比暂时空着更像出错。
function useCounts(listVersion: number): NavCounts | null {
  const [counts, setCounts] = useState<NavCounts | null>(null)
  useEffect(() => {
    let active = true
    sessionService.Counts().then(value => { if (active) setCounts(value) }).catch(reason => { if (active) console.error('加载会话计数失败', reason) })
    return () => { active = false }
  }, [listVersion])
  return counts
}