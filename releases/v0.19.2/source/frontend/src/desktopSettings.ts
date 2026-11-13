import type { DesktopOptions } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

type DesktopKey = 'start_at_login' | 'close_to_tray'
type DesktopAPI = {
  Desktop: () => Promise<DesktopOptions>
  SetStartAtLogin: (value: boolean) => Promise<DesktopOptions>
  SetCloseToTray: (value: boolean) => Promise<DesktopOptions>
}
type DesktopState = { view: DesktopOptions | null; operation: 'loading' | 'saving' | null; error: string }

// 两个设置共用一次读写状态：禁用并发提交，所有显示值都来自后端。
export function createDesktopSettings(api: DesktopAPI) {
  let state: DesktopState = { view: null, operation: null, error: '' }
  const listeners = new Set<() => void>()
  const publish = (next: DesktopState) => {
    state = next
    listeners.forEach(listener => listener())
  }
  return {
    getSnapshot: () => state,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    async load() {
      if (state.operation) return
      publish({ ...state, operation: 'loading', error: '' })
      try {
        publish({ view: await api.Desktop(), operation: null, error: '' })
      } catch (reason) {
        publish({ view: null, operation: null, error: `读取启动与托盘设置失败：${String(reason)}` })
      }
    },
    async save(key: DesktopKey, value: boolean) {
      if (state.operation || !state.view) return
      if (key === 'start_at_login' ? !state.view.start_at_login_supported : !state.view.tray_supported) return
      publish({ ...state, operation: 'saving', error: '' })
      try {
        const view = await (key === 'start_at_login' ? api.SetStartAtLogin(value) : api.SetCloseToTray(value))
        publish({ view, operation: null, error: '' })
      } catch (reason) {
        const error = `保存启动与托盘设置失败：${String(reason)}`
        try {
          publish({ view: await api.Desktop(), operation: null, error })
        } catch (readReason) {
          publish({ view: null, operation: null, error: `${error}；重新读取失败：${String(readReason)}` })
        }
      }
    },
  }
}
