import { Tab, TabList, makeStyles, tokens } from '@fluentui/react-components'
import { useState } from 'react'
import type { ChatGPTImportReport, ClaudeImportReport } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { ChatGPTHistoryImport } from './ChatGPTHistoryImport'
import { ClaudeHistoryImport } from './ClaudeHistoryImport'

const useStyles = makeStyles({
  root: { height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0 },
  tabs: { padding: '8px 28px 0', backgroundColor: tokens.colorNeutralBackground2, borderBottom: `1px solid ${tokens.colorNeutralStroke2}` },
  body: { flex: 1, minHeight: 0 },
})

type Source = 'claude' | 'chatgpt'

/** 导入页：按数据来源切换。每个来源各自走官方数据导出，互不影响。 */
export function ImportsView({ onClaudeImported, onChatGPTImported }: {
  onClaudeImported: (report: ClaudeImportReport) => Promise<void>
  onChatGPTImported: (report: ChatGPTImportReport) => Promise<void>
}) {
  const styles = useStyles()
  const [source, setSource] = useState<Source>('claude')
  return <div className={styles.root}>
    <div className={styles.tabs}>
      <TabList selectedValue={source} onTabSelect={(_, data) => setSource(data.value as Source)} aria-label="导入来源">
        <Tab value="claude">Claude</Tab>
        <Tab value="chatgpt">ChatGPT</Tab>
      </TabList>
    </div>
    <div className={styles.body}>
      {source === 'claude' ? <ClaudeHistoryImport onImported={onClaudeImported} /> : <ChatGPTHistoryImport onImported={onChatGPTImported} />}
    </div>
  </div>
}
