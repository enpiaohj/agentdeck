import { Select } from '@fluentui/react-components'
import { CLI_AGENTS, cliAgent, type CliAgent } from '../agentOptions'
import type { CliAvailability } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

// 平台选择器。检测到命令缺失时在名称后标注命令名，让用户在弹窗里就知道
// 该装什么，而不是启动失败后去终端里找原因。
export function AgentSelect({ value, onChange, disabled, id, availability }: {
  value: CliAgent
  onChange: (agent: CliAgent) => void
  disabled?: boolean
  id?: string
  availability?: Record<string, CliAvailability>
}) {
  return <Select id={id} aria-label="智能体" value={value} disabled={disabled} onChange={(_, data) => onChange(cliAgent(data.value))}>
    {CLI_AGENTS.map(option => {
      const row = availability?.[option.value]
      const missing = row && !row.found ? `（未检测到 ${row.command}）` : ''
      return <option key={option.value} value={option.value}>{option.label}{missing}</option>
    })}
  </Select>
}