// 索引状态订阅:监听 Go 侧 EventIndexProgress / EventIndexReady 推送。
// 事件经 @wailsio/runtime 的 Events 总线分发。
import { useEffect, useState } from 'react'
import { Events } from '@wailsio/runtime'
import type { IndexStatus } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as search from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import { indexStatusFromEvent } from '../eventPayload'

export function useIndexStatus() {
  const [status, setStatus] = useState<IndexStatus | null>(null)
  useEffect(() => {
    search.Status().then(setStatus).catch(() => {})
    const offProg = Events.On('index:progress', (e: unknown) => {
      setStatus(indexStatusFromEvent(e))
    })
    const offReady = Events.On('index:ready', (e: unknown) => {
      setStatus(indexStatusFromEvent(e))
    })
    return () => {
      offProg()
      offReady()
    }
  }, [])
  return status
}
