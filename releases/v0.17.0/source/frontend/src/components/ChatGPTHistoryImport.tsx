import { Button, Checkbox, Input, Spinner, Text, makeStyles, tokens } from '@fluentui/react-components'
import { ArrowUploadRegular, DocumentTextRegular, OpenRegular, PersonRegular } from '@fluentui/react-icons'
import { Browser } from '@wailsio/runtime'
import { useState } from 'react'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import type { ChatGPTImportReport, ChatGPTPreview } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { fmtBytes } from '../format'
import { pickOpenPath } from '../nativeDialogs'

const exportHelp = 'https://help.openai.com/en/articles/7260999-how-do-i-export-my-chatgpt-history-and-data'
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
  project: { display: 'grid', gridTemplateColumns: 'minmax(180px, 260px) 1fr', gap: '12px', alignItems: 'start', padding: '10px 0', borderTop: `1px solid ${tokens.colorNeutralStroke2}` },
  samples: { fontSize: '12px', lineHeight: 1.6, color: tokens.colorNeutralForeground3, minWidth: 0, overflowWrap: 'anywhere' },
})

/**
 * 导入 ChatGPT 官方数据导出。导出文件里没有项目名，只有项目 ID，所以读完导出文件后
 * 列出每个项目最近几个会话的标题，由用户命名；未命名的项目显示为“ChatGPT 项目 xxxxxx”。
 */
export function ChatGPTHistoryImport({ onImported }: { onImported: (report: ChatGPTImportReport) => Promise<void> }) {
  const styles = useStyles()
  const [account, setAccount] = useState('')
  const [file, setFile] = useState('')
  const [preview, setPreview] = useState<ChatGPTPreview | null>(null)
  const [names, setNames] = useState<Record<string, string>>({})
  // 图片默认一并导入：没有图片的对话往往看不懂；磁盘空间紧张的用户可以取消选中
  const [withImages, setWithImages] = useState(true)
  const [reading, setReading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const choose = async () => {
    try {
      const selected = await pickOpenPath({
        CanChooseFiles: true, CanChooseDirectories: false, Title: '选择 ChatGPT 数据导出文件',
        Filters: [{ DisplayName: 'ChatGPT 导出的 ZIP 文件', Pattern: '*.zip' }, { DisplayName: 'conversations JSON 文件', Pattern: '*.json' }],
      })
      if (!selected || typeof selected !== 'string') return
      setFile(selected); setError(''); setMessage(''); setPreview(null); setNames({})
      setReading(true)
      try {
        // 先填写账号名时，已为该账号命名的项目会一并带回
        const result = await sessionService.PreviewChatGPT(selected, account.trim())
        setPreview(result)
        setNames(Object.fromEntries(result.projects.map(p => [p.id, p.name])))
      } finally { setReading(false) }
    } catch (reason) { setError(String(reason).slice(0, 400)) }
  }

  const run = async () => {
    if (busy || !file || !preview || !account.trim()) return
    setBusy(true); setError(''); setMessage('')
    let saved = false
    try {
      const chosen = Object.fromEntries(Object.entries(names).filter(([, name]) => name.trim()))
      const result = await sessionService.ImportChatGPT(file, account.trim(), chosen, withImages && (preview.images ?? 0) > 0)
      saved = true
      const prefix = result.already_imported ? '此导出文件已导入过，已更新项目名' : `已导入 ${result.conversations} 个会话（${result.projects} 个项目）`
      setMessage(`${prefix}${result.images ? `；${result.images} 张图片` : ''}${result.skipped_blocks ? `；${result.skipped_blocks} 处图片或音频内容未导入` : ''}`)
      await onImported(result)
    } catch (reason) { setError(`${saved ? '数据已保存，但界面刷新失败：' : ''}${String(reason)}`.slice(0, 400)) }
    finally { setBusy(false) }
  }

  return <section className={styles.wrap} aria-label="导入 ChatGPT 聊天记录">
    <div className={styles.content}>
      <header>
        <div className={styles.eyebrow}>数据来源 / ChatGPT 网页版</div>
        <h1 className={styles.title}>导入 ChatGPT 聊天记录</h1>
        <p className={styles.description}>ChatGPT 网页版的会话保存在云端，本机无法直接读取。通过官方数据导出，可将聊天和项目中的会话导入本机，然后进行搜索、收藏，或导出为 Markdown。整个过程无需登录，也不会读取任何账号凭据。</p>
      </header>
      <div className={styles.card}>
        <div className={styles.field}>
          <Text weight="semibold">1. 从 ChatGPT 导出数据</Text>
          <div className={styles.description}>在 ChatGPT 网页版中，依次选择“设置”&gt;“数据控制”&gt;“导出数据”，确认后等待电子邮件，然后下载其中的 ZIP 文件。导出内容包含整个账号的数据，其中有项目中的会话。</div>
          <Button appearance="subtle" icon={<OpenRegular />} style={{ alignSelf: 'flex-start' }} onClick={() => Browser.OpenURL(exportHelp).catch(reason => setError(String(reason)))}>查看官方导出说明</Button>
        </div>
        <div className={styles.field}>
          <label className={styles.label} htmlFor="chatgpt-import-account">2. 设置账号名称</label>
          <Input id="chatgpt-import-account" value={account} maxLength={80} disabled={busy} contentBefore={<PersonRegular />} placeholder="例如：个人账号" onChange={(_, data) => setAccount(data.value)} />
          <div className={styles.note}>先填写账号名称，再选择文件。同一账号的后续导入请使用相同的名称，以便更新、去重，并带回已命名的项目。</div>
        </div>
        <div className={styles.field}>
          <span className={styles.label}>3. 选择导出文件</span>
          <div className={styles.file}>
            <DocumentTextRegular />
            <span className={styles.fileName} title={file}>{file ? file.split(/[\\/]/).pop() : 'ZIP 文件（或 conversations-*.json）'}</span>
            <Button size="small" disabled={busy || reading || !account.trim()} onClick={() => void choose()}>选择文件…</Button>
          </div>
          {reading ? <div className={styles.note} role="status"><Spinner size="extra-tiny" /> 正在读取导出文件，较大的文件需要几秒钟…</div> : null}
          {preview ? <div className={styles.description} role="status">共 {preview.conversations} 个会话，包含 {preview.projects.length} 个项目。</div> : null}
          {preview && preview.images > 0 ? <Checkbox checked={withImages} disabled={busy} onChange={(_, data) => setWithImages(data.checked === true)}
            label={`同时导入图片（约 ${fmtBytes(preview.image_bytes)}，${preview.images} 张）`} /> : null}
          {preview ? <div className={styles.note}>{preview.images > 0 ? '图片会复制到本机，并直接显示在阅读区；如果取消选中，则只显示“图片”占位符。' : '此导出文件中没有可导入的图片。'}不会复制你上传的文件（文档、代码、表格等），只保留文件名。语音对话只有文字转录，导出文件中不含音频。</div> : null}
        </div>
        {preview && preview.projects.length > 0 ? <div className={styles.field}>
          <Text weight="semibold">4. 命名项目（可选）</Text>
          <div className={styles.note}>导出文件中没有项目名，只有编号。右侧列出各项目中最近的会话标题，可据此识别项目。留空则使用默认名称；之后可随时重命名，无需重新导入。</div>
          <div>
            {preview.projects.map(project => <div key={project.id} className={styles.project}>
              <Input aria-label={`项目名（${project.conversations} 个会话）`} value={names[project.id] ?? ''} maxLength={60} disabled={busy} placeholder={project.default}
                onChange={(_, data) => setNames(prev => ({ ...prev, [project.id]: data.value }))} />
              <div className={styles.samples}>
                <div>{project.conversations} 个会话</div>
                {project.samples.map((title, index) => <div key={index}>· {title}</div>)}
              </div>
            </div>)}
          </div>
        </div> : null}
        {error ? <div role="alert" style={{ color: tokens.colorPaletteRedForeground1, fontSize: 13, overflowWrap: 'anywhere' }}>{error}</div> : null}
        {message ? <div role="status" className={styles.description}>{message}</div> : null}
        <div className={styles.footer}>
          <span className={styles.note}>导入后，你可以预览、全文搜索、收藏，或导出为 Markdown。</span>
          <Button appearance="primary" icon={busy ? <Spinner size="extra-tiny" /> : <ArrowUploadRegular />} disabled={busy || reading || !preview || !account.trim()} onClick={() => void run()}>{busy ? '正在导入…' : '导入聊天记录'}</Button>
        </div>
      </div>
      <p className={styles.note}>导入的副本保存在本机，不会与云端自动同步。导入内容包括对话文本、语音转录、思考摘要和（可选的）图片，不包括重新生成所产生的其他分支；你上传的文件只保留文件名。导入的记录可以查看和搜索，但无法继续对话。</p>
    </div>
  </section>
}
