// 索引状态订阅:监听 Go 侧 EventIndexProgress / EventIndexReady 推送。
// 事件经 @wailsio/runtime 的 Events 总线分发。
import { useEffect, useState } from 'react'
import { Events } from '@wailsio/runtime'
import type { IndexStatus } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as search from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'

export function useIndexStatus() {
  const [status, setStatus] = useState<IndexStatus | null>(null)
  useEffect(() => {
    search.Status().then(setStatus).catch(() => {})
    const offProg = Events.On('index:progress', (e: unknown) => {
      setStatus(extract(e))
    })
    const offReady = Events.On('index:ready', (e: unknown) => {
      setStatus(extract(e))
    })
    return () => {
      offProg()
      offReady()
    }
  }, [])
  return status
}

// 事件载荷取 data[0]:Go 侧 Emit(event, IndexStatus) 单参数。
function extract(e: unknown): IndexStatus | null {
  const d = (e as { data?: unknown[] })?.data
  return d && d[0] ? (d[0] as IndexStatus) : null
}
