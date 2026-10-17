import { Button, Input, Spinner, Text, makeStyles, tokens } from '@fluentui/react-components'
import { ArrowUploadRegular, DocumentTextRegular, PersonRegular, OpenRegular } from '@fluentui/react-icons'
import { Browser, Dialogs } from '@wailsio/runtime'
import { useState } from 'react'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import type { ClaudeImportReport } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

const exportHelp = 'https://support.claude.com/en/articles/9450526-export-your-claude-data'
const useStyles = makeStyles({
  wrap: { height: '100%', overflowY: 'auto', backgroundColor: tokens.colorNeutralBackground2, padding: '28px' },
  content: { maxWidth: '760px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '20px' },
  eyebrow: { color: tokens.colorNeutralForeground3, fontSize: '12px', fontWeight: 600 },
  title: { fontSize: '20px', letterSpacing: '-0.025em', lineHeight: 1.3, margin: '8px 0 12px' },
  description: { color: tokens.colorNeutralForeground2, fontSize: '13px', lineHeight: 1.8 },
  card: { padding: '20px', borderRadius: '8px', backgroundColor: tokens.colorNeutralBackground1, border: `1px solid ${tokens.colorNeutralStroke2}`, display: 'flex', flexDirection: 'column', gap: '20px' },
  field: { display: 'flex', flexDirection: 'column', gap: '8px' },
  label: { fontSize: '13px', fontWeight: 600 },
  file: { display: 'flex', alignItems: 'center', gap: '10px', padding: '12px', border: `1px dashed ${tokens.colorNeutralStroke1}`, borderRadius: '6px', minWidth: 0 },
  fileName: { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '13px' },
  note: { fontSize: '12px', lineHeight: 1.8, color: tokens.colorNeutralForeground3 },
  footer: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '14px', flexWrap: 'wrap' },
})

export function ClaudeHistoryImport({ onImported }: { onImported: (report: ClaudeImportReport) => Promise<void> }) {
  const styles = useStyles()
  const [account, setAccount] = useState('')
  const [file, setFile] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const choose = async () => {
    try {
      const selected = await Dialogs.OpenFile({
        CanChooseFiles: true, CanChooseDirectories: false, Title: '选择 Claude 数据导出',
        Filters: [{ DisplayName: 'Claude 导出 ZIP', Pattern: '*.zip' }, { DisplayName: 'Claude 会话 JSON', Pattern: '*.json' }],
      })
      if (selected && typeof selected === 'string') { setFile(selected); setError(''); setMessage('') }
    } catch (reason) { setError(String(reason)) }
  }

  const run = async () => {
    if (busy || !file || !account.trim()) return
    setBusy(true); setError(''); setMessage('')
    let saved = false
    try {
      const result = await sessionService.ImportClaudeHistory(file, account.trim())
      saved = true
      const prefix = result.already_imported ? '该文件已导入，无需重复导入' : `已导入 ${result.conversations} 个会话`
      setMessage(`${prefix}${result.skipped_blocks ? `；${result.skipped_blocks} 个非文本内容块未纳入` : ''}`)
      await onImported(result)
    } catch (reason) { setError(`${saved ? '数据已保存，但界面刷新失败：' : ''}${String(reason)}`.slice(0, 400)) }
    finally { setBusy(false) }
  }

  return <section className={styles.wrap} aria-label="导入 Claude 聊天历史">
    <div className={styles.content}>
      <header>
        <div className={styles.eyebrow}>数据来源 / Claude Desktop</div>
        <h1 className={styles.title}>导入 Claude 聊天记录</h1>
        <p className={styles.description}>Claude 的普通聊天及新版聊天任务不一定保存在 Code / Cowork 本地目录。通过官方数据导出，可将会话导入本机并按账号检索。</p>
      </header>
      <div className={styles.card}>
        <div className={styles.field}>
          <Text weight="semibold">1. 从对应账号导出记录</Text>
          <div className={styles.description}>在 Claude 桌面端或网页版进入「设置 → 隐私 → 导出数据」，下载邮件中的导出文件。不同账号分别导出。</div>
          <Button appearance="subtle" icon={<OpenRegular />} style={{ alignSelf: 'flex-start' }} onClick={() => Browser.OpenURL(exportHelp).catch(reason => setError(String(reason)))}>查看官方导出说明</Button>
        </div>
        <div className={styles.field}>
          <label className={styles.label} htmlFor="claude-import-account">2. 设置账号名称</label>
          <Input id="claude-import-account" value={account} maxLength={80} disabled={busy} contentBefore={<PersonRegular />} placeholder="例如：工作账号、个人账号" onChange={(_, data) => setAccount(data.value)} />
          <div className={styles.note}>同一账号后续导入请沿用相同名称，便于更新和去重。</div>
        </div>
        <div className={styles.field}>
          <span className={styles.label}>3. 选择导出文件</span>
          <div className={styles.file}>
            <DocumentTextRegular />
            <span className={styles.fileName} title={file}>{file ? file.split(/[\\/]/).pop() : 'ZIP 或 conversations.json · 最大 512 MiB'}</span>
            <Button size="small" disabled={busy} onClick={() => void choose()}>选择文件</Button>
          </div>
        </div>
        {error ? <div role="alert" style={{ color: tokens.colorPaletteRedForeground1, fontSize: 13, overflowWrap: 'anywhere' }}>{error}</div> : null}
        {message ? <div role="status" className={styles.description}>{message}</div> : null}
        <div className={styles.footer}>
          <span className={styles.note}>导入后可预览、全文检索、收藏和导出 Markdown。</span>
          <Button appearance="primary" icon={busy ? <Spinner size="extra-tiny" /> : <ArrowUploadRegular />} disabled={busy || !file || !account.trim()} onClick={() => void run()}>{busy ? '正在导入…' : '导入聊天记录'}</Button>
        </div>
      </div>
      <p className={styles.note}>导入副本保存在本机，不会自动同步云端。当前读取导出中的文本和思考内容；附件文件本体、图片及部分工具内容不在导入范围内。导入记录可查看和检索，不能用 CLI 恢复命令继续。</p>
    </div>
  </section>
}
