import { ArrowSyncRegular, CheckmarkCircleRegular, DatabaseRegular, WarningRegular } from '@fluentui/react-icons'
import { useIndexStatus } from '../hooks/useIndexStatus'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

export function WorkspaceStatus({ session }: { session: SessionView | null }) {
  const status = useIndexStatus()
  const state = status?.state
  const icon = state === 'ready' ? <CheckmarkCircleRegular /> : state === 'building' ? <ArrowSyncRegular /> : state === 'failed' ? <WarningRegular /> : <DatabaseRegular />
  // building 有分母时才显示 n / total：28 万段的整库重建要跑很久，只有
  // 「0 个会话」会被读成卡死。分母为 0（还没算出来）时退化为省略号。
  const building = state === 'building' ? status?.total ? `正在构建索引 · ${status?.sessions ?? 0} / ${status.total} 个会话` : '正在构建索引…' : ''
  const label = state === 'ready' ? `全文索引就绪 · ${status?.docs ?? 0} 段`
    : state === 'building' ? building
    : state === 'failed' ? '索引暂不可用 · 使用快速搜索'
    : '全文索引尚未就绪'
  const source = session?.source === 'retained' ? '永久保留副本' : session?.source === 'import' ? '导入副本' : session?.source === 'cache' ? 'Desktop 聊天缓存' : session?.source === 'wsl' ? 'WSL' : session?.source === 'gateway' ? '网关记录' : '本地文件'
  return <footer className="ad-workspace-status" aria-label="工作台状态">
    <span className="ad-status-item" role="status" title={status?.error || label}>{icon}{label}</span>
    <span className="ad-status-trailing">
      {session ? <span>{session.account ? `${session.account} · ` : ''}来源：{source}</span> : <span>历史会话工作台</span>}
      <span className="ad-status-shortcut"><kbd>Ctrl K</kbd> 搜索</span>
    </span>
  </footer>
}
