import { Button, Dialog, DialogSurface, DialogBody, DialogTitle, DialogContent, DialogActions, Spinner, Text } from '@fluentui/react-components'
import { DatabaseRegular, ArrowUploadRegular } from '@fluentui/react-icons'
import { pickOpenPath } from '../nativeDialogs'
import { useState } from 'react'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as searchService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import type { BackupInfo } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { usePreferences } from '../hooks/usePreferences'
import { parsePreferences } from '../preferences'

export function DataBackup({ onRestored }: { onRestored: () => void }) {
  const { preferences, update } = usePreferences()
  const [busy, setBusy] = useState<'backup' | 'inspect' | 'restore' | null>(null)
  const [pending, setPending] = useState<{ path: string; info: BackupInfo } | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const backup = async () => {
    if (busy) return
    setError(''); setMessage(''); setBusy('backup')
    try {
      const dir = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: '选择完整备份保存目录', ButtonText: '备份到此目录' })
      if (!dir || typeof dir !== 'string') return
      const path = await sessionService.BackupData(dir, JSON.stringify(preferences))
      setMessage(`完整备份已保存：${path}`)
    } catch (reason) { setError(`备份失败：${String(reason)}`) }
    finally { setBusy(null) }
  }
  const chooseRestore = async () => {
    if (busy) return
    setError(''); setMessage(''); setBusy('inspect')
    try {
      const path = await pickOpenPath({ CanChooseDirectories: false, CanChooseFiles: true, Title: '选择 AgentDeck 完整备份', Filters: [{ DisplayName: 'AgentDeck 完整备份 ZIP', Pattern: '*.zip' }] })
      if (!path || typeof path !== 'string') return
      const info = await sessionService.InspectBackup(path)
      setPending({ path, info })
    } catch (reason) { setError(`备份校验失败：${String(reason)}`) }
    finally { setBusy(null) }
  }
  const restore = async () => {
    if (!pending || busy) return
    setBusy('restore'); setError(''); setMessage('')
    try {
      const report = await sessionService.RestoreData(pending.path, JSON.stringify(preferences))
      update(parsePreferences(report.preferences))
      setPending(null)
      setMessage(`数据恢复完成。恢复前备份：${report.rollback_path}`)
      onRestored()
      try { await searchService.Rebuild() }
      catch (reason) { setError(`数据已恢复，但全文索引重建未能启动：${String(reason)}`) }
    } catch (reason) { setPending(null); setError(`恢复未完成：${String(reason)}`) }
    finally { setBusy(null) }
  }
  return <section className="ad-backup-section" aria-label="完整数据备份与恢复">
    <h2>完整备份</h2>
    <p>将本地会话库及全部内容版本、收藏、归档、标签、备注、导入历史、项目设置、位置关联和界面偏好保存为一个 ZIP 文件。</p>
    <div className="ad-backup-actions">
      <Button size="small" icon={busy === 'backup' ? <Spinner size="extra-tiny" /> : <DatabaseRegular />} disabled={!!busy} onClick={() => void backup()}>{busy === 'backup' ? '正在备份…' : '创建完整备份…'}</Button>
      <Button size="small" icon={busy === 'inspect' ? <Spinner size="extra-tiny" /> : <ArrowUploadRegular />} disabled={!!busy} onClick={() => void chooseRestore()}>{busy === 'inspect' ? '正在校验…' : '从完整备份恢复…'}</Button>
    </div>
    <p>恢复采用合并方式，保留当前新增会话与内容版本；同一会话的收藏、归档、标签和备注以备份为准。恢复前自动创建当前数据的完整备份。</p>
    <p>备份范围为 AgentDeck 已保存的数据，不包含原生智能体的账号凭据、原始目录和附件文件。全文索引在恢复后重新构建。</p>
    <p>单个备份最多包含 20,000 个数据文件，解压后总大小不超过 4 GiB。</p>
    {message ? <Text block role="status" className="ad-backup-message">{message}</Text> : null}
    {error ? <Text block role="alert" className="ad-backup-error">{error}</Text> : null}
    <Dialog open={!!pending} onOpenChange={(_, data) => { if (!data.open && !busy) setPending(null) }}>
      <DialogSurface className="ad-backup-dialog"><DialogBody>
        <DialogTitle>恢复完整备份</DialogTitle>
        <DialogContent>
          {pending ? <>
            <p>备份已通过完整性校验。请确认以下信息：</p>
            <dl className="ad-backup-summary">
              <dt>备份时间</dt><dd>{new Date(pending.info.created_at).toLocaleString('zh-CN')}</dd>
              <dt>会话数量</dt><dd>{pending.info.sessions} 个</dd>
              <dt>内容版本</dt><dd>{pending.info.versions} 个</dd>
              <dt>文件数量</dt><dd>{pending.info.files} 个</dd>
              <dt>数据大小</dt><dd>{(pending.info.bytes / 1024 / 1024).toFixed(1)} MB</dd>
              <dt>备份文件</dt><dd>{pending.path}</dd>
            </dl>
            <p>当前新增会话和历史版本将予以保留。同一会话的标注、同一项目的设置与位置关联及界面偏好将采用备份中的内容；存在冲突的导入文件不会被覆盖。</p>
            <p>恢复前将自动保存当前数据，完成后重新构建全文索引。</p>
          </> : null}
        </DialogContent>
        <DialogActions>
          <Button disabled={!!busy} onClick={() => setPending(null)}>取消</Button>
          <Button appearance="primary" disabled={!!busy} icon={busy === 'restore' ? <Spinner size="extra-tiny" /> : undefined} onClick={() => void restore()}>{busy === 'restore' ? '正在恢复…' : '恢复数据'}</Button>
        </DialogActions>
      </DialogBody></DialogSurface>
    </Dialog>
  </section>
}
