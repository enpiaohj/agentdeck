// 事件载荷解码。
//
// Wails 的 EmitEvent(name, data...) 只传一个参数时把该参数直接当作 data，
// 传多个才是数组（见 wails 的 WebviewWindow.EmitEvent）。Go 侧这几个事件
// 都是单参数，所以前端拿到的是对象/切片本身，不是包一层的数组。两种形状
// 都收下：只认一种会在形状变化时静默丢事件，而丢事件不报错——状态条只会
// 一直停在「全文索引尚未就绪」。
import type { IndexStatus } from '../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

export function indexStatusFromEvent(payload: unknown): IndexStatus | null {
  const data = (payload as { data?: unknown } | null | undefined)?.data
  if (!data) return null
  if (Array.isArray(data)) return (data[0] as IndexStatus) ?? null
  return data as IndexStatus
}
