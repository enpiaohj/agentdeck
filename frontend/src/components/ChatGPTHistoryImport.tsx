import { Button, Checkbox, Input, Spinner, Text, Tooltip, makeStyles, tokens } from '@fluentui/react-components'
import { ArrowUploadRegular, DocumentTextRegular, OpenRegular, PersonRegular } from '@fluentui/react-icons'
import { openExternal } from '../openExternal'
import { useEffect, useState } from 'react'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
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
  warning: { fontSize: '13px', lineHeight: 1.7, padding: '10px 12px', borderRadius: '6px', display: 'flex', flexDirection: 'column', gap: '8px', alignItems: 'flex-start', backgroundColor: tokens.colorPaletteYellowBackground1, border: `1px solid ${tokens.colorPaletteYellowBorder1}`, color: tokens.colorNeutralForeground1 },
  footer: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '14px', flexWrap: 'wrap' },
  projectName: { display: 'flex', flexDirection: 'column', gap: '4px', minWidth: 0 },
  projectActions: { display: 'flex', flexWrap: 'wrap', gap: '2px', marginLeft: '-6px' },
  project: { display: 'grid', gridTemplateColumns: 'minmax(180px, 260px) 1fr', gap: '12px', alignItems: 'start', padding: '10px 0', borderTop: `1px solid ${tokens.colorNeutralStroke2}` },
  samples: { fontSize: '12px', lineHeight: 1.6, color: tokens.colorNeutralForeground3, minWidth: 0, overflowWrap: 'anywhere' },
})

// 推荐名的依据，显示在推荐按钮的悬停说明里
const BASIS: Record<string, string> = {
  titles: '根据该项目会话标题中反复出现的词推测',
  file: '取自该项目上传的文件名',
  latest: '取自该项目最近一个会话的标题',
}

// 项目首页：浏览器里登录后会跳转到带项目名的地址，页面上也直接显示项目名
const projectPage = (id: string) => /^g-p-[0-9a-f]{32}$/i.test(id) ? `https://chatgpt.com/g/${id}/project` : ''

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
  // 链接会用设置里选的浏览器打开；这里只取名字，用于说明
  const [browserName, setBrowserName] = useState('系统默认浏览器')
  useEffect(() => { void settingsService.BrowserSettings().then(view => setBrowserName(view.name)).catch(() => undefined) }, [])

  const choose = async () => {
    try {
      const selected = await pickOpenPath({
        CanChooseFiles: true, CanChooseDirectories: false, Title: '选择 ChatGPT 数据导出文件',
        Filters: [{ DisplayName: 'ChatGPT 导出的 ZIP 文件', Pattern: '*.zip' }, { DisplayName: 'conversations JSON 文件', Pattern: '*.json' }],
      })
      if (!selected || typeof selected !== 'string') return
      setFile(selected)
      await readExport(selected, account.trim())
    } catch (reason) { setError(String(reason).slice(0, 400)) }
  }

  // 读一遍导出文件，给出会话数、项目与已有账号的重叠情况。账号名变了要重读：
  // 已为该账号命名的项目会随之带回。
  const readExport = async (path: string, accountName: string) => {
    setError(''); setMessage(''); setPreview(null); setNames({})
    setReading(true)
    try {
      const result = await sessionService.PreviewChatGPT(path, accountName)
      setPreview(result)
      // 已命名的优先；否则用本机 ChatGPT 桌面应用里的项目名（可改）；都没有则留空
      setNames(Object.fromEntries(result.projects.map(p => [p.id, p.name || p.suggested])))
    } catch (reason) { setError(String(reason).slice(0, 400)) } finally { setReading(false) }
  }
  // 与这份导出共有会话、但账号名和你填的不同的已有账号：同一个 ChatGPT 账号用了两个名字，
  // 每个会话会各存一份，收藏、标签也不互通
  const typed = account.trim().toLowerCase()
  const sameName = (preview?.accounts ?? []).find(item => item.name.toLowerCase() === typed)
  const otherAccount = sameName ? undefined : (preview?.accounts ?? [])[0]
  const useAccount = (name: string) => { setAccount(name); if (file) void readExport(file, name) }

  const openProject = async (id: string) => {
    const page = projectPage(id)
    if (!page) return
    try { await openExternal(page) } catch (reason) { setError(`无法打开项目页面：${String(reason)}`.slice(0, 300)) }
  }
  const empty = (id: string) => !(names[id] ?? '').trim()
  const useRecommended = (id: string, name: string) => setNames(prev => ({ ...prev, [id]: name }))
  const useAllRecommended = () => setNames(prev => ({ ...prev, ...Object.fromEntries((preview?.projects ?? []).filter(p => p.recommended && !(prev[p.id] ?? '').trim()).map(p => [p.id, p.recommended])) }))

  const run = async () => {
    if (busy || !file || !preview || !account.trim()) return
    setBusy(true); setError(''); setMessage('')
    let saved = false
    try {
      // 输入框里有名字就保存；清空了原来已保存的名字，则传空串把它清掉（回到默认名称）
      const chosen: Record<string, string> = {}
      for (const project of preview.projects) {
        const value = (names[project.id] ?? '').trim()
        if (value || project.name) chosen[project.id] = value
      }
      const result = await sessionService.ImportChatGPT(file, account.trim(), chosen, withImages && (preview.images ?? 0) > 0)
      saved = true
      const prefix = result.already_imported ? '此导出文件已导入过，已更新项目名' : `已导入 ${result.conversations} 个会话（${result.projects} 个项目）`
      setMessage(`${prefix}${result.images ? `；${result.images} 张图片` : ''}${result.skipped_blocks ? `；${result.skipped_blocks} 处图片或音频内容未导入` : ''}${result.reclaimed ? `；已清理 ${result.reclaimed} 个被取代的旧批次，释放 ${fmtBytes(result.reclaimed_bytes)}` : ''}`)
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
          <Button appearance="subtle" icon={<OpenRegular />} style={{ alignSelf: 'flex-start' }} onClick={() => openExternal(exportHelp).catch(reason => setError(String(reason)))}>查看官方导出说明</Button>
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
          {otherAccount ? <div className={styles.warning} role="alert">
            这份导出里有 {otherAccount.overlap} 个会话已在账号“{otherAccount.name}”下导入过。如果它们属于同一个 ChatGPT 账号，请使用同样的账号名称，否则每个会话会各存一份，收藏和标签也不互通。
            <div><Button size="small" appearance="primary" disabled={busy || reading} onClick={() => useAccount(otherAccount.name)}>改用“{otherAccount.name}”</Button></div>
          </div> : null}
          {sameName && sameName.missing > 0 ? <div className={styles.note} role="status">
            账号“{sameName.name}”下已导入的会话里，有 {sameName.missing} 个不在这份导出里，可能已在 ChatGPT 中删除。它们会继续保留在这里；如需清理，可在“设置”&gt;“会话”里开启“允许删除导入的聊天记录”，再到会话列表中选择删除。
          </div> : null}
          {preview && preview.images > 0 ? <Checkbox checked={withImages} disabled={busy} onChange={(_, data) => setWithImages(data.checked === true)}
            label={`同时导入图片（约 ${fmtBytes(preview.image_bytes)}，${preview.images} 张）`} /> : null}
          {preview ? <div className={styles.note}>{preview.images > 0 ? '图片会复制到本机，并直接显示在阅读区；如果取消选中，则只显示“图片”占位符。' : '此导出文件中没有可导入的图片。'}不会复制你上传的文件（文档、代码、表格等），只保留文件名。语音对话只有文字转录，导出文件中不含音频。</div> : null}
        </div>
        {preview && preview.projects.length > 0 ? <div className={styles.field}>
          <Text weight="semibold">4. 命名项目（可选）</Text>
          <div className={styles.note}>{preview.projects.some(p => p.suggested)
            ? `导出文件中没有项目名。已从本机的 ChatGPT 桌面应用读取到 ${preview.projects.filter(p => p.suggested).length} 个项目的名称并填入，你可以直接使用或修改。`
            : '导出文件中没有项目名，只有编号。'}右侧列出各项目中最近的会话标题和上传的文件名，可据此识别项目。没有现成的名称时，会根据会话标题和上传的文件名给出“推荐名”，只是推测，请确认后再使用。也可以点“在 ChatGPT 中打开”，用 {browserName} 打开项目页面查看名称（可在“设置”&gt;“常规”中更改浏览器）。留空则使用默认名称；之后可随时重命名，无需重新导入。</div>
          {preview.projects.some(p => p.recommended && empty(p.id)) ? <Button size="small" appearance="secondary" style={{ alignSelf: 'flex-start' }} disabled={busy} onClick={useAllRecommended}>全部使用推荐名</Button> : null}
          <div>
            {preview.projects.map(project => <div key={project.id} className={styles.project}>
              <div className={styles.projectName}>
                <Input aria-label={`项目名（${project.conversations} 个会话）`} value={names[project.id] ?? ''} maxLength={60} disabled={busy} placeholder={project.default}
                  onChange={(_, data) => setNames(prev => ({ ...prev, [project.id]: data.value }))} />
                <div className={styles.projectActions}>
                  {project.recommended && empty(project.id) ? <Tooltip content={`${BASIS[project.recommended_basis] ?? '推测的名称'}，仅供参考`} relationship="description">
                    <Button size="small" appearance="subtle" disabled={busy} onClick={() => useRecommended(project.id, project.recommended)}>使用推荐名：{project.recommended}</Button>
                  </Tooltip> : null}
                  <Tooltip content={`用 ${browserName} 打开该项目的页面，页面上会显示项目名`} relationship="description">
                    <Button size="small" appearance="subtle" icon={<OpenRegular />} disabled={busy || !projectPage(project.id)} onClick={() => void openProject(project.id)}>在 ChatGPT 中打开</Button>
                  </Tooltip>
                </div>
              </div>
              <div className={styles.samples}>
                <div>{project.conversations} 个会话</div>
                {project.samples.map((title, index) => <div key={index}>· {title}</div>)}
                {project.files.length ? <div>项目文件：{project.files.join('、')}</div> : null}
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
