import type { UpdateView } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

type UpdateAPI = {
  Status: () => Promise<UpdateView>
  Check: () => Promise<UpdateView>
  Download: () => Promise<UpdateView>
  CancelDownload: () => Promise<UpdateView>
  RestartToInstall: () => Promise<void>
  SetAutoCheck: (enabled: boolean) => Promise<UpdateView>
  OpenDownloadFolder: () => Promise<void>
}
// 后端经 update:status 推送状态；订阅方式由调用方注入，本模块不直接依赖
// wails 运行时（这样纯 Node 环境也能测）。
type UpdateEvents = { subscribe: (handler: (view: UpdateView) => void) => () => void }
type UpdateState = { view: UpdateView | null; loading: boolean; error: string; notice: string }

// 更新状态全部由后端持有：这里只缓存最后一次读到的视图。检查与下载都是
// 长动作，进度经 update:status 事件推来，因此写操作失败时重新读一次
// Status，而不是自己拼一个可能跟后端不一致的本地状态。
//
// notice 是与 error 分开的一条中性提示（目前只有「已取消下载」）：取消不是
// 失败，但也不能一声不响，否则用户看不出取消生效了没有。
export function createUpdateSettings(api: UpdateAPI, events: UpdateEvents) {
  let state: UpdateState = { view: null, loading: false, error: '', notice: '' }
  // 用户点了「取消下载」：正在飞行中的那次 Download 会以「已取消」失败，
  // 那不是错误，不该在界面上报一条红字。
  let canceling = false
  const listeners = new Set<() => void>()
  const publish = (next: UpdateState) => {
    state = next
    listeners.forEach(listener => listener())
  }
  const refresh = async (error: string) => {
    try {
      publish({ view: await api.Status(), loading: false, error, notice: '' })
    } catch (reason) {
      publish({ view: null, loading: false, error: `${error}；重新读取状态失败：${String(reason)}`, notice: '' })
    }
  }
  return {
    getSnapshot: () => state,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    async load() {
      if (state.loading) return
      publish({ ...state, loading: true, error: '', notice: '' })
      try {
        publish({ view: await api.Status(), loading: false, error: '', notice: '' })
      } catch (reason) {
        publish({ view: null, loading: false, error: `读取更新状态失败：${String(reason)}`, notice: '' })
      }
    },
    async check() {
      try {
        publish({ view: await api.Check(), loading: false, error: '', notice: '' })
      } catch (reason) {
        await refresh(`检查更新失败：${String(reason)}`)
      }
    },
    async download() {
      try {
        publish({ view: await api.Download(), loading: false, error: '', notice: '' })
      } catch (reason) {
        // 用户主动取消下载时后端也返回「已取消下载」，那不是失败：界面应当安静地
        // 退回「有新版本」。取消标志由 cancelDownload 置位（那一次调用先返回）。
        await refresh(canceling ? '' : `下载更新失败：${String(reason)}`)
      } finally {
        canceling = false
      }
    },
    async cancelDownload() {
      canceling = true
      try {
        const view = await api.CancelDownload()
        const latest = view.latest ? `v${view.latest}` : '新版本'
        publish({ view, loading: false, error: '', notice: `已取消下载。${latest} 仍可随时下载。` })
      } catch (reason) {
        canceling = false
        await refresh(`取消下载失败：${String(reason)}`)
      }
    },
    async setAutoCheck(enabled: boolean) {
      try {
        publish({ view: await api.SetAutoCheck(enabled), loading: false, error: '', notice: '' })
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
    //
    // 不清本地 error：后端推送发生在检查/下载中途，本地那条错误（例如「打开下载
    // 目录失败」）与后端状态无关，清掉会让失败提示一闪而过、用户根本看不到。
    watch(): () => void {
      return events.subscribe(view => publish({ ...state, view, loading: false }))
    },
  }
}

// UpdateStage 是界面的显式状态：只有一处判断，标题、按钮与提示都跟着它走，
// 避免「按钮可点但其实必然失败」这类靠字段各自为政推导出来的组合。
// noAsset：有更新的版本，但发布里暂时没有可下载并校验的安装文件。
export type UpdateStage = 'loading' | 'checking' | 'downloading' | 'ready' | 'available' | 'noAsset' | 'upToDate' | 'dev' | 'idle'

export function stageOf(view: UpdateView | null, loading: boolean): UpdateStage {
  if (loading && !view) return 'loading'
  if (!view) return 'idle'
  if (view.checking) return 'checking'
  if (view.downloading) return 'downloading'
  if (view.ready_to_restart) return 'ready'
  if (view.available) return 'available'
  if (view.asset_missing) return 'noAsset'
  if (view.dev && view.checked) return 'dev'
  if (view.checked) return 'upToDate'
  return 'idle'
}
