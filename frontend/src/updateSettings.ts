import type { UpdateView } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

type UpdateAPI = {
  Status: () => Promise<UpdateView>
  Check: () => Promise<UpdateView>
  Download: () => Promise<UpdateView>
  RestartToInstall: () => Promise<void>
  SetAutoCheck: (enabled: boolean) => Promise<UpdateView>
  OpenDownloadFolder: () => Promise<void>
}
// 后端经 update:status 推送状态；订阅方式由调用方注入，本模块不直接依赖
// wails 运行时（这样纯 Node 环境也能测）。
type UpdateEvents = { subscribe: (handler: (view: UpdateView) => void) => () => void }
type UpdateState = { view: UpdateView | null; loading: boolean; error: string }

// 更新状态全部由后端持有：这里只缓存最后一次读到的视图。检查与下载都是
// 长动作，进度经 update:status 事件推来，因此写操作失败时重新读一次
// Status，而不是自己拼一个可能跟后端不一致的本地状态。
export function createUpdateSettings(api: UpdateAPI, events: UpdateEvents) {
  let state: UpdateState = { view: null, loading: false, error: '' }
  const listeners = new Set<() => void>()
  const publish = (next: UpdateState) => {
    state = next
    listeners.forEach(listener => listener())
  }
  const refresh = async (error: string) => {
    try {
      publish({ view: await api.Status(), loading: false, error })
    } catch (reason) {
      publish({ view: null, loading: false, error: `${error}；重新读取状态失败：${String(reason)}` })
    }
  }
  return {
    getSnapshot: () => state,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    async load() {
      if (state.loading) return
      publish({ ...state, loading: true, error: '' })
      try {
        publish({ view: await api.Status(), loading: false, error: '' })
      } catch (reason) {
        publish({ view: null, loading: false, error: `读取更新状态失败：${String(reason)}` })
      }
    },
    async check() {
      try {
        publish({ view: await api.Check(), loading: false, error: '' })
      } catch (reason) {
        await refresh(`检查更新失败：${String(reason)}`)
      }
    },
    async download() {
      try {
        publish({ view: await api.Download(), loading: false, error: '' })
      } catch (reason) {
        await refresh(`下载更新失败：${String(reason)}`)
      }
    },
    async setAutoCheck(enabled: boolean) {
      try {
        publish({ view: await api.SetAutoCheck(enabled), loading: false, error: '' })
      } catch (reason) {
        await refresh(`保存自动检查设置失败：${String(reason)}`)
      }
    },
    async restart() {
      try {
        await api.RestartToInstall()
      } catch (reason) {
        await refresh(`重启失败：${String(reason)}`)
      }
    },
    async openFolder() {
      try {
        await api.OpenDownloadFolder()
      } catch (reason) {
        publish({ ...state, error: `打开下载目录失败：${String(reason)}` })
      }
    },
    // 订阅后端推送的进度与结论，返回取消订阅函数。
    watch(): () => void {
      return events.subscribe(view => publish({ view, loading: false, error: '' }))
    },
  }
}
