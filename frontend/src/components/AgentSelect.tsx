import { Select } from '@fluentui/react-components'
import { CLI_AGENTS, cliAgent, type CliAgent } from '../agentOptions'

export function AgentSelect({ value, onChange, disabled, id }: { value: CliAgent; onChange: (agent: CliAgent) => void; disabled?: boolean; id?: string }) {
  return <Select id={id} aria-label="智能体" value={value} disabled={disabled} onChange={(_, data) => onChange(cliAgent(data.value))}>
    {CLI_AGENTS.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}
  </Select>
}
