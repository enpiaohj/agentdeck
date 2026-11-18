import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { clampPaneWidth, type PaneWidthRange } from '../preferences'

// 拖动时至少给阅读区留这么多像素，三栏怎么拖都不会把正文挤没。
// 1280px 宽的窗口里另两栏取默认值时，任一栏仍能放宽 100px 以上。
export const readerMinWidth = 320

// usePaneResizer 把「拖动分隔条改一栏宽度」收在一处：拖动期间只更新本地预览，
// 松手才写入偏好；双击恢复自动宽度；方向键按 8px、Shift+方向键按 32px 微调。
// saved 为 0 表示跟随布局默认（窄窗口下由媒体查询收窄）。
// edge 是分隔条所在的一侧：右侧分隔条向右拖变宽，左侧（会话详情栏）反之。
// reserve 返回「本次拖动最多能占多少之外的其余部分」——即另两栏当前实际宽度
// 加上阅读区下限；窗口变窄时它跟着变大，于是这里能拖到的上限自动收紧。
export function usePaneResizer(
  saved: number,
  range: PaneWidthRange,
  commit: (width: number) => void,
  edge: 'left' | 'right' = 'right',
  reserve: () => number = () => 0,
) {
  const [resizing, setResizing] = useState(false)
  const [preview, setPreview] = useState(0)
  const [viewport, setViewport] = useState(() => window.innerWidth)
  const previewRef = useRef(0)
  const drag = useRef({ startX: 0, startWidth: 0 })
  const reserveRef = useRef(reserve)
  reserveRef.current = reserve
  // 导航栏的分隔条在包装节点内部，需要把包装节点交给调用方挂 ref。
  const slot = useRef<HTMLDivElement>(null)
  // commit 每次渲染都是新函数，放进 ref，免得拖动途中重建事件监听。
  const commitRef = useRef(commit)
  commitRef.current = commit
  const sign = edge === 'right' ? 1 : -1

  useEffect(() => {
    const onResize = () => setViewport(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  const clamp = useCallback((value: number, reserved: number) => clampPaneWidth(value, range, viewport, reserved), [range, viewport])

  useEffect(() => {
    if (!resizing) return
    const move = (event: PointerEvent) => {
      const next = clamp(drag.current.startWidth + sign * (event.clientX - drag.current.startX), reserveRef.current())
      previewRef.current = next
      setPreview(next)
    }
    const end = () => {
      setResizing(false)
      // 只点一下没拖动时不写偏好：否则「跟随布局默认」会被悄悄钉成当前像素值。
      // 拖动过就清掉连击计时，免得拖完顺手再点一下被当成双击复位。
      if (previewRef.current && previewRef.current !== drag.current.startWidth) {
        commitRef.current(previewRef.current)
        lastClick.current = { time: 0, x: 0, y: 0 }
      }
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', end)
    window.addEventListener('pointercancel', end)
    return () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', end)
      window.removeEventListener('pointercancel', end)
    }
  }, [resizing, clamp, sign])

  const measure = (target: HTMLElement) => target.parentElement?.getBoundingClientRect().width ?? 0

  // 双击恢复自动宽度：在 pointerdown 上自己数连击，不用 dblclick。
  // start 里必须 preventDefault（否则拖动会选中正文），而 Chromium 会因此
  // 吞掉兼容鼠标事件，onDoubleClick 根本不触发——实测双击无反应。
  // 时间窗 600ms 覆盖系统双击阈值，位置容差 6px 容忍手抖。
  const lastClick = useRef({ time: 0, x: 0, y: 0 })

  const start = (event: ReactPointerEvent<HTMLElement>) => {
    const now = Date.now()
    const previous = lastClick.current
    lastClick.current = { time: now, x: event.clientX, y: event.clientY }
    if (now - previous.time < 600 && Math.abs(event.clientX - previous.x) < 6 && Math.abs(event.clientY - previous.y) < 6) {
      lastClick.current = { time: 0, x: 0, y: 0 }
      event.preventDefault()
      commitRef.current(0)
      return
    }
    const width = Math.round(measure(event.currentTarget) || saved || range.min)
    // pointerdown 的 preventDefault 会拦住默认聚焦，键盘微调因此拿不到焦点。
    event.currentTarget.focus()
    drag.current = { startX: event.clientX, startWidth: width }
    previewRef.current = width
    setPreview(width)
    setResizing(true)
    event.preventDefault()
  }
  // delta 表示「分隔条往右移多少」：右侧栏（分隔条在它左边）随之变宽，
  // 详情栏（分隔条在它右边）随之变窄，所以按 edge 取反。
  const nudge = (event: React.KeyboardEvent<HTMLElement>, delta: number) => {
    const current = Math.round(measure(event.currentTarget) || saved || range.min)
    commitRef.current(clamp(current + sign * delta, reserveRef.current()))
  }

  // 渲染时也过一遍上限：窗口缩小后已保存的宽度会自动收，不会把阅读区吃掉。
  const width = resizing && preview ? preview : saved ? clampPaneWidth(saved, range, viewport, reserveRef.current()) : 0
  return { resizing, width, start, reset: () => commitRef.current(0), nudge, slot }
}
