// 事件载荷解码。
//
// Wails 的 EmitEvent(name, data...) 只传一个参数时把该参数直接当作 data，
// 传多个才是数组（见 wails 的 WebviewWindow.EmitEvent）。Go 侧这几个事件
// 都是单参数，所以前端拿到的是对象/切片本身，不是包一层的数组。两种形状
// 都收下：只认一种会在形状变化时静默丢事件，而丢事件不报错——状态条只会
// 一直停在「全文索引尚未就绪」。
import type { IndexStatus } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

// StorageProgress 与 Go 侧 app.StorageProgress 的 JSON 形状一致。它只作为
// storage:progress 的载荷出现，没有任何服务方法引用它，因此 bindings 不会
// 生成这个类型——改动 Go 侧字段时这里要同步。
export type StorageProgress = {
  stage: string
  done: number
  total: number
  files: number
  done_files: number
  message: string
}

export function indexStatusFromEvent(payload: unknown): IndexStatus | null {
  const data = (payload as { data?: unknown } | null | undefined)?.data
  if (!data) return null
  if (Array.isArray(data)) return (data[0] as IndexStatus) ?? null
  return data as IndexStatus
}

// storageProgressFromEvent 解码迁移进度（storage:progress），规则同上。
export function storageProgressFromEvent(payload: unknown): StorageProgress | null {
  const data = (payload as { data?: unknown } | null | undefined)?.data
  if (!data) return null
  if (Array.isArray(data)) return (data[0] as StorageProgress) ?? null
  return data as StorageProgress
}
