import { useCallback, useEffect, useRef, useState } from 'react'

export type ToastAction = { label: string; run: () => void }
export type Toast = { id: number; text: string; tone: 'info' | 'success' | 'error'; action?: ToastAction }

// useToasts 是全局操作反馈的唯一出口：成功、失败、可撤销的动作都走这里，
// 不再占用顶部那条「上次未正常退出」的警示条（两者混在一起时，用户分不清
// 是本次操作的结果还是启动时的状态）。
export function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)
  const timers = useRef(new Map<number, number>())
  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id)
    if (timer !== undefined) { clearTimeout(timer); timers.current.delete(id) }
    setToasts(list => list.filter(t => t.id !== id))
  }, [])
  const notify = useCallback((text: string, options?: { tone?: Toast['tone']; action?: ToastAction; duration?: number }) => {
    const id = nextId.current++
    const tone = options?.tone ?? 'info'
    // 带撤销的提示要留够反应时间；失败比成功停久一些。
    const duration = options?.duration ?? (options?.action ? 15000 : tone === 'error' ? 10000 : 6000)
    setToasts(list => [...list, { id, text, tone, action: options?.action }].slice(-3))
    timers.current.set(id, window.setTimeout(() => dismiss(id), duration))
    return id
  }, [dismiss])
  useEffect(() => {
    const pending = timers.current
    return () => { pending.forEach(clearTimeout); pending.clear() }
  }, [])
  return { toasts, notify, dismiss }
}

export type Notify = ReturnType<typeof useToasts>['notify']