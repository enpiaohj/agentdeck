import { Menu, MenuPopover, MenuList, MenuItem, MenuDivider } from '@fluentui/react-components'
import { OpenRegular, PanelRightRegular, EditRegular, StarRegular, ArchiveRegular, ArchiveArrowBackRegular, CopyRegular, ArrowDownloadRegular, CodeRegular, DeleteRegular } from '@fluentui/react-icons'
import { useMemo } from 'react'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { deletionUnavailableReason } from '../sessionDeletion'

export type SessionAction = 'open' | 'details' | 'rename' | 'resume' | 'copyResume' | 'copyTitle' | 'copyCwd' | 'export' | 'favorite' | 'archive' | 'delete'

export function SessionContextMenu({ items, point, onClose, onAction }: {
  items: SessionView[]; point: { x: number; y: number }; onClose: () => void; onAction: (action: SessionAction) => void
}) {
  const target = useMemo(() => ({ getBoundingClientRect: () => DOMRect.fromRect({ x: point.x, y: point.y, width: 0, height: 0 }) }), [point.x, point.y])
  const single = items.length === 1
  const favorite = items.every(item => item.favorite)
  const archived = items.every(item => item.archived)
  const canResume = single && !!items[0].resume
  const deleteReason = deletionUnavailableReason(items)
  return <Menu open positioning={{ target, position: 'below', align: 'start', offset: 2 }} closeOnScroll onOpenChange={(_, data) => { if (!data.open) onClose() }}>
    <MenuPopover className="ad-context-menu" onKeyDown={event => event.stopPropagation()}>
      <MenuList aria-label="会话操作菜单">
        <div className="ad-context-heading">{single ? items[0].title || '未命名会话' : `已选择 ${items.length} 个会话`}</div>
        {single ? <>
          <MenuItem icon={<OpenRegular />} onClick={() => onAction('open')}>打开会话</MenuItem>
          <MenuItem icon={<PanelRightRegular />} onClick={() => onAction('details')}>管理标签与备注</MenuItem>
          <MenuItem icon={<EditRegular />} onClick={() => onAction('rename')}>重命名</MenuItem>
          <MenuItem icon={<CodeRegular />} disabled={!canResume || items[0].source === 'wsl'} onClick={() => onAction('resume')}>在终端中继续会话</MenuItem>
          <MenuItem icon={<CopyRegular />} disabled={!canResume} onClick={() => onAction('copyResume')}>复制恢复命令</MenuItem>
          <MenuDivider />
        </> : null}
        <MenuItem icon={<StarRegular />} onClick={() => onAction('favorite')}>{favorite ? '取消收藏' : '收藏'}</MenuItem>
        <MenuItem icon={archived ? <ArchiveArrowBackRegular /> : <ArchiveRegular />} onClick={() => onAction('archive')}>{archived ? '取消归档' : '归档'}</MenuItem>
        <MenuDivider />
        <MenuItem icon={<ArrowDownloadRegular />} disabled={items.some(item => !item.transcript)} onClick={() => onAction('export')}>{single ? '导出为 Markdown…' : `导出 ${items.length} 个会话为 Markdown…`}</MenuItem>
        <MenuItem icon={<CopyRegular />} onClick={() => onAction('copyTitle')}>{single ? '复制标题' : '复制所选标题'}</MenuItem>
        {single ? <MenuItem icon={<CopyRegular />} disabled={!items[0].cwd} onClick={() => onAction('copyCwd')}>复制工作目录</MenuItem> : null}
        <MenuDivider />
        <MenuItem icon={<DeleteRegular />} disabled={!!deleteReason} title={deleteReason || '删除原始记录及 AgentDeck 中的全部保留版本'} onClick={() => onAction('delete')}>删除会话…</MenuItem>
        {deleteReason ? <div className="ad-context-delete-reason">{deleteReason}</div> : null}
      </MenuList>
    </MenuPopover>
  </Menu>
}
