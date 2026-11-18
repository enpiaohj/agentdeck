import type { KeyboardEvent } from 'react'
import type { usePaneResizer } from '../hooks/usePaneResizer'

// PaneResizer 是三栏共用的分隔条：拖动改宽、双击恢复自动宽度、方向键微调。
// 它压在相邻两栏的边框上，本身只占 7px，不改变布局列宽；悬停与拖动时都不画线。
export function PaneResizer({ label, edge = 'right', width }: { label: string; edge?: 'left' | 'right'; width: ReturnType<typeof usePaneResizer> }) {
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
    event.preventDefault()
    // delta 是「分隔条往右移多少」：方向键移动分隔条本身，右侧栏随之变宽，
    // 详情栏（分隔条在它左边）随之变窄——由 hook 按 edge 换算。
    width.nudge(event, (event.key === 'ArrowRight' ? 1 : -1) * (event.shiftKey ? 32 : 8))
  }
  return <div
    className={`ad-resizer ad-resizer-${edge}`}
    role="separator"
    aria-orientation="vertical"
    aria-label={`调整${label}宽度`}
    aria-valuenow={width.width || undefined}
    tabIndex={0}
    title={`拖拽调整${label}宽度，双击恢复自动宽度`}
    onPointerDown={width.start}
    onKeyDown={onKeyDown}
  />
}
