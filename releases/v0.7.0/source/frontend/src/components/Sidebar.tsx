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
import type { AgentView, NavCounts, ProjectView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { agentMeta } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { isCliAgent, platformLabel } from '../agentOptions'
import { EmptyState } from './EmptyState'
import { FormDialog } from './FormDialog'
import type { Notify } from '../hooks/useToasts'

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
    color: 'var(--ad-muted)',
    backgroundColor: 'transparent',
    fontVariantNumeric: 'tabular-nums',
    padding: '0 2px',
    lineHeight: '19px',
    flexShrink: 0,
  },
  status: { padding: '10px 14px', fontSize: '11px', color: 'var(--ad-muted)', borderTop: `1px solid ${tokens.colorNeutralStroke3}` },
  sectionTitle: {
    padding: '18px 16px 7px',
    fontSize: '11px',
    fontWeight: 600,
    letterSpacing: '0.025em',
    color: 'var(--ad-muted)',
  },
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
function isAll(f: SidebarFilter) { return !f.favorites && !f.archived && !f.agent && !f.project }

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
  const agents = useAgents(listVersion)
  const projects = useProjects(listVersion)
  const counts = useCounts(listVersion)
  const [menu, setMenu] = useState<{ target: MenuTarget; x: number; y: number } | null>(null)
  const [action, setAction] = useState<ProjectAction | null>(null)
  const [renameDraft, setRenameDraft] = useState('')
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState(false)
  const pending = useRef(false)
  // held 记住最近一次操作的项目：关闭动画期间 action 已经清空，但字段还要有
  // 内容可渲染，否则会看到一瞬空白弹窗。
  const held = useRef<Project | null>(null)

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
          <MenuItem disabled={!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent'} onClick={() => { if (menu.target.kind === 'agent') onNewSession(undefined, menu.target.agent); setMenu(null) }}>新建会话…</MenuItem>
          <MenuItem disabled={!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent'} onClick={() => { if (menu.target.kind === 'agent') onNewProject(menu.target.agent); setMenu(null) }}>新建项目…</MenuItem>
          {!isCliAgent(menu.target.agent) && menu.target.agent !== 'cursor-agent' ? <div className="ad-context-heading">此来源暂不支持新建会话</div> : null}
        </> : <>
          <div className="ad-context-heading">{menu.target.project.name}</div>
          <MenuItem icon={<OpenRegular />} onClick={() => { if (menu.target.kind === 'project') onNewSession(menu.target.project.path, menu.target.project.agent || undefined); setMenu(null) }}>在此新建会话…</MenuItem>
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
  const pick = (f: SidebarFilter) => (event: MouseEvent<HTMLButtonElement>) => {
    const add = event.ctrlKey || event.metaKey
    const key = 'agent' in f ? 'agent' : 'project' in f ? 'project' : f.favorites ? 'favorites' : 'archived'
    const on = key === 'agent' ? filter.agent === f.agent
      : key === 'project' ? filter.project === f.project
      : key === 'favorites' ? !!filter.favorites
      : !!filter.archived
    const next: SidebarFilter = add ? { ...filter } : {}
    if (on) delete next[key]
    else if (key === 'agent') next.agent = f.agent
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
        <button className={mergeClasses(styles.item, isAll(filter) && styles.itemActive)} onClick={onSelectAll} title="全部会话" style={{ justifyContent: 'center' }}>
          <AppsRegular />
        </button>
        <button className={mergeClasses(styles.item, filter.favorites && styles.itemActive)} onClick={pick({ favorites: true })} title="收藏（Ctrl+单击与其他筛选叠加）" style={{ justifyContent: 'center' }}>
          <StarRegular />
        </button>
        <button className={mergeClasses(styles.item, filter.archived && styles.itemActive)} onClick={pick({ archived: true })} title="归档（Ctrl+单击与其他筛选叠加）" style={{ justifyContent: 'center' }}>
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

      <div className={styles.sectionTitle}>智能体</div>
      <div className={styles.agentScroll}>
      {agents.map((a) => (
        <button
          onContextMenu={event => contextMenu(event, { kind: 'agent', agent: a.agent })}
          onKeyDown={event => keyboardMenu(event, { kind: 'agent', agent: a.agent })}
          key={a.agent}
          className={mergeClasses(styles.item, filter.agent === a.agent && styles.itemActive)}
          onClick={pick({ agent: a.agent })}
          title={`${agentMeta(a.agent).label} · ${a.count} 个会话 · 右键新建会话或项目 · Ctrl+单击叠加筛选`}
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
        <Button className={styles.projectAdd} data-return-focus="new-project" size="small" appearance="subtle" icon={<AddRegular />} title="新建项目 (Ctrl+Shift+N)" aria-label="新建项目" onClick={() => onNewProject()} />
      </div>
      <div className={styles.projectScroll} role="group" aria-labelledby="sidebar-projects-heading">
        {projects.map((p) => {
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