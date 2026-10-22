import { Menu, MenuPopover, MenuList, MenuItem, MenuDivider } from '@fluentui/react-components'
import { OpenRegular, PanelRightRegular, EditRegular, StarRegular, ArchiveRegular, ArchiveArrowBackRegular, CopyRegular, ArrowDownloadRegular, CodeRegular, DeleteRegular, PinRegular, PinOffRegular } from '@fluentui/react-icons'
import { useMemo } from 'react'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { deletionUnavailableReason } from '../sessionDeletion'

export type SessionAction = 'open' | 'details' | 'rename' | 'resume' | 'copyResume' | 'copyTitle' | 'copyCwd' | 'export' | 'pin' | 'favorite' | 'archive' | 'delete'

export function SessionContextMenu({ items, point, pinned = false, onClose, onAction }: {
  items: SessionView[]; point: { x: number; y: number }; pinned?: boolean; onClose: () => void; onAction: (action: SessionAction) => void
}) {
  const target = useMemo(() => ({ getBoundingClientRect: () => DOMRect.fromRect({ x: point.x, y: point.y, width: 0, height: 0 }) }), [point.x, point.y])
  const single = items.length === 1
  const favorite = items.every(item => item.favorite)
  const archived = items.every(item => item.archived)
  const canResume = single && !!items[0].resume
  const deleteReason = deletionUnavailableReason(items)
  // shiftToCoverTarget：点击点上下两侧都放不下整份菜单时，让菜单竖直平移、盖住点击点
  // （系统右键菜单的做法），而不是截短出滚动条；只有窗口本身都放不下时 autoSize 才会滚动。
  return <Menu open positioning={{ target, position: 'below', align: 'start', offset: 2, strategy: 'fixed', autoSize: 'height', flipBoundary: 'window', overflowBoundary: 'window', overflowBoundaryPadding: 8, shiftToCoverTarget: true }} closeOnScroll onOpenChange={(_, data) => { if (!data.open) onClose() }}>
    <MenuPopover className="ad-context-menu" style={{ overflowY: 'hidden' }} onKeyDown={event => event.stopPropagation()}>
      <MenuList className="ad-context-menu-list" aria-label="会话操作菜单" onFocus={event => {
        if (event.target instanceof HTMLElement && event.target.getAttribute('role') === 'menuitem') event.target.scrollIntoView({ block: 'nearest', inline: 'nearest' })
      }}>
        <div className="ad-context-heading">{single ? items[0].title || '未命名会话' : `已选择 ${items.length} 个会话`}</div>
        {single ? <>
          <MenuItem icon={<OpenRegular />} secondaryContent="Enter" onClick={() => onAction('open')}>打开会话</MenuItem>
          <MenuItem icon={<PanelRightRegular />} onClick={() => onAction('details')}>管理标签与备注</MenuItem>
          <MenuItem icon={<EditRegular />} secondaryContent="F2" onClick={() => onAction('rename')}>重命名</MenuItem>
          <MenuItem icon={<CodeRegular />} disabled={!canResume || items[0].source === 'wsl'} title={!canResume ? '该来源的会话暂不支持在终端中继续（仅可查看历史）' : items[0].source === 'wsl' ? 'WSL 会话需在 WSL 内继续' : undefined} onClick={() => onAction('resume')}>在终端中继续会话</MenuItem>
          <MenuItem icon={<CopyRegular />} disabled={!canResume} title={!canResume ? '该来源的会话没有可用的恢复命令' : undefined} onClick={() => onAction('copyResume')}>复制恢复命令</MenuItem>
          <MenuDivider />
        </> : null}
        <MenuItem icon={pinned ? <PinOffRegular /> : <PinRegular />} onClick={() => onAction('pin')}>{pinned ? '取消置顶' : '置顶'}</MenuItem>
        <MenuItem icon={<StarRegular />} onClick={() => onAction('favorite')}>{favorite ? '取消收藏' : '收藏'}</MenuItem>
        <MenuItem icon={archived ? <ArchiveArrowBackRegular /> : <ArchiveRegular />} onClick={() => onAction('archive')}>{archived ? '取消归档' : '归档'}</MenuItem>
        <MenuDivider />
        <MenuItem icon={<ArrowDownloadRegular />} disabled={items.some(item => !item.transcript)} title={items.some(item => !item.transcript) ? '所选会话含无法读取正文的来源，不能导出' : undefined} onClick={() => onAction('export')}>{single ? '导出为 Markdown…' : `导出 ${items.length} 个会话为 Markdown…`}</MenuItem>
        <MenuItem icon={<CopyRegular />} onClick={() => onAction('copyTitle')}>{single ? '复制标题' : '复制所选标题'}</MenuItem>
        {single ? <MenuItem icon={<CopyRegular />} disabled={!items[0].cwd} title={!items[0].cwd ? '该会话没有记录工作目录' : undefined} onClick={() => onAction('copyCwd')}>复制工作目录</MenuItem> : null}
        <MenuDivider />
        <MenuItem className="ad-context-danger" icon={<DeleteRegular />} secondaryContent="Del" disabled={!!deleteReason} title={deleteReason || '删除原始记录及 AgentDeck 中的全部保留版本'} onClick={() => onAction('delete')}>删除会话…</MenuItem>
        {deleteReason ? <div className="ad-context-delete-reason">{deleteReason}</div> : null}
      </MenuList>
    </MenuPopover>
  </Menu>
}
