import { Button, Checkbox, Dialog, DialogActions, DialogBody, DialogContent, DialogSurface, DialogTitle, Spinner, Text, tokens } from '@fluentui/react-components'
import { ArrowUndoRegular, DeleteRegular, DismissRegular } from '@fluentui/react-icons'
import type { SessionDeletion } from '../hooks/useSessionDeletion'

export function DeleteSessionsDialog({ deletion: d }: { deletion: SessionDeletion }) {
  return <Dialog open={d.targets.length > 0} onOpenChange={(_, data) => { if (!data.open) d.close() }}>
    <DialogSurface className="ad-delete-dialog" onKeyDown={event => event.stopPropagation()}><DialogBody>
      <DialogTitle>删除会话</DialogTitle>
      <DialogContent className="ad-delete-content">
        <p>将删除所选会话的 <strong>Codex / Claude 原始记录</strong>，以及 AgentDeck 保存的全部内容版本、收藏、归档、标签和备注。</p>
        <p>请先在原应用或终端中结束这些会话。删除前会自动生成可恢复备份；备份失败时不会开始删除。</p>
        <Checkbox checked={d.includeChildren} disabled={d.busy} onChange={(_, data) => d.changeChildren(data.checked === true)} label="同时删除这些会话的已知子会话" />
        <p className="ad-delete-hint">默认保留未选中的独立子会话。Claude 主记录专属的子 Agent 文件属于该记录的删除范围，会列在下方。</p>
        {d.loading ? <div className="ad-delete-loading" role="status"><Spinner size="tiny" />正在核对原始记录与删除范围…</div> : null}
        {d.preview ? <>
          <dl className="ad-backup-summary ad-delete-summary">
            <dt>将删除会话</dt><dd>{d.preview.sessions} 个（明确选择 {d.targets.length} 个）</dd>
            <dt>已知额外子会话</dt><dd>{d.preview.children} 个{d.includeChildren ? '（已包含）' : '（保留）'}</dd>
            <dt>涉及原始文件</dt><dd>{d.preview.native_files} 个（含相关索引）</dd>
            <dt>涉及文件总大小</dt><dd>{(d.preview.bytes / 1024 / 1024).toFixed(2)} MB</dd>
          </dl>
          <details className="ad-delete-scope" open><summary>核对会话与路径</summary>
            <ul>{d.preview.titles.map((title, index) => <li key={index}>{title || '未命名会话'}</li>)}</ul>
            {d.preview.paths.length ? <ul className="ad-delete-paths">{d.preview.paths.map((path, index) => <li key={index}>{path}</li>)}</ul> : <p>没有仍存在的原始文件，将清理 AgentDeck 中的保留记录。</p>}
          </details>
          <Checkbox checked={d.confirmedEnded} disabled={d.busy} onChange={(_, data) => d.setConfirmedEnded(data.checked === true)} label="我确认以上原始会话均已结束，并了解原始记录和全部保留版本将被删除" />
        </> : null}
        {d.error ? <Text block role="alert" className="ad-delete-error">{d.error}</Text> : null}
      </DialogContent>
      <DialogActions>
        {!d.preview && !d.loading && !d.busy ? <Button onClick={d.retryPreview}>重新预览</Button> : null}
        <Button disabled={d.busy} onClick={d.close}>取消</Button>
        <Button appearance="primary" className="ad-delete-confirm" icon={d.busy ? <Spinner size="extra-tiny" /> : <DeleteRegular />} disabled={d.busy || d.loading || !d.preview?.token || !d.confirmedEnded} onClick={() => void d.remove()}>{d.busy ? '正在备份并删除…' : '备份并删除'}</Button>
      </DialogActions>
    </DialogBody></DialogSurface>
  </Dialog>
}

export function DeletedSessionsNotice({ deletion: d }: { deletion: SessionDeletion }) {
  if (!d.result && !d.restoredMessage) return null
  return <div className="ad-deletion-notice" role="status" style={{ backgroundColor: tokens.colorBrandBackground2, color: tokens.colorNeutralForeground1 }}>
    <div>
      <span>{d.result ? d.result.message || `已删除 ${d.result.deleted_keys.length} 个会话。` : d.restoredMessage}</span>
      {d.result ? <><div className="ad-delete-backup">恢复备份：{d.result.backup_path}</div><div className="ad-delete-hint">撤销仅在本次运行期间可用；备份保留在磁盘。再次删除前请保存备份位置。</div></> : null}
      {d.undoError ? <div role="alert" className="ad-delete-error">{d.undoError}</div> : null}
    </div>
    {d.result?.undo_token ? <Button size="small" icon={d.undoBusy ? <Spinner size="extra-tiny" /> : <ArrowUndoRegular />} disabled={d.undoBusy || d.busy || !!d.targets.length} onClick={() => void d.undo()}>{d.undoBusy ? '正在恢复…' : '撤销删除'}</Button> : null}
    <Button appearance="subtle" size="small" icon={<DismissRegular />} disabled={d.undoBusy} aria-label="关闭删除提示" onClick={d.dismissNotice} />
  </div>
}
