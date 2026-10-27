import { useEffect, useState } from 'react'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import type { CliAvailability } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

// useAgentAvailability 报告各 CLI 命令在本机 PATH 上的情况。找不到只用于
// 提示，不阻止操作：命令可能装在别处，用户也可能打算先建项目再去装。
// 命令名以后端映射为准（qoder → qodercli 之类），前端不另存一份。
export function useAgentAvailability(revision: number): Record<string, CliAvailability> {
  const [rows, setRows] = useState<Record<string, CliAvailability>>({})
  useEffect(() => {
    let active = true
    resumeService.Available().then(list => {
      if (!active) return
      const next: Record<string, CliAvailability> = {}
      for (const row of list) next[row.agent] = row
      setRows(next)
    }).catch(reason => { if (active) console.error('检测本机命令行工具失败', reason) })
    return () => { active = false }
  }, [revision])
  return rows
}

// missingCommand 返回所选平台缺少的命令名；命令齐全或还没检测完时返回空串。
export function missingCommand(rows: Record<string, CliAvailability>, agent: string): string {
  const row = rows[agent]
  return row && !row.found ? row.command : ''
}