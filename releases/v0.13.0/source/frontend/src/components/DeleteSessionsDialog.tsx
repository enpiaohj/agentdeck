import { Button, Checkbox, Dialog, DialogActions, DialogBody, DialogContent, DialogSurface, DialogTitle, Spinner, Text, tokens } from '@fluentui/react-components'
import { ArrowUndoRegular, DeleteRegular, DismissRegular } from '@fluentui/react-icons'
import type { SessionDeletion } from '../hooks/useSessionDeletion'
import { deleteSummary, deleteTitle, deletionTitles, irreversibleAck } from '../sessionDeletion'

// 删除确认，照成熟桌面应用的写法：问句标题、一句话后果、条目列表、两个按钮。
// 确认门槛跟风险挂钩——默认会先备份、可以撤销，点「删除」即可；设置里关掉
// 备份后不可撤销，才多一次勾选。核对用的文件数、大小、路径默认折叠。
export function DeleteSessionsDialog({ deletion: d }: { deletion: SessionDeletion }) {
  const preview = d.preview
  const count = preview?.sessions ?? d.targets.length
  const { shown, more } = deletionTitles(preview?.titles ?? d.targets.map(target => target.title))
  const ready = !!preview?.token && !d.loading && !d.busy
  return <Dialog open={d.targets.length > 0} onOpenChange={(_, data) => { if (!data.open) d.close() }}>
    <DialogSurface className="ad-delete-dialog" onKeyDown={event => event.stopPropagation()}><DialogBody>
      <DialogTitle>{deleteTitle(count)}</DialogTitle>
      {/* 焦点先落在滚动容器上，而不是第一个控件：打开时不会把说明滚出视野。 */}
      <DialogContent className="ad-delete-content" tabIndex={-1} ref={node => node?.focus({ preventScroll: true })}>
        {d.error ? <Text block role="alert" className="ad-delete-error">{d.error}</Text> : null}
        <p className={d.backup ? 'ad-delete-lead' : 'ad-delete-lead ad-delete-lead-direct'}>{deleteSummary(d.backup)}</p>
        <ul className="ad-delete-items" aria-label="将删除的会话">
          {shown.map((title, index) => <li key={index}>{title}</li>)}
          {more ? <li className="ad-delete-more">以及另外 {more} 个会话</li> : null}
        </ul>
        <p className="ad-delete-note">删除前，请在原应用或终端中结束相关会话。备份选项可在“设置 › 会话”中更改。</p>
        {d.loading ? <div className="ad-delete-loading" role="status"><Spinner size="tiny" />正在核对原始记录与删除范围…</div> : null}
        {preview && (preview.children > 0 || d.includeChildren) ? <Checkbox checked={d.includeChildren} disabled={d.busy} onChange={(_, data) => d.changeChildren(data.checked === true)} label={`同时删除已知子会话（${preview.children} 个）`} /> : null}
        {preview ? <details className="ad-delete-scope"><summary>详细信息（{preview.native_files} 个文件，{(preview.bytes / 1024 / 1024).toFixed(2)} MB）</summary>
          <dl className="ad-backup-summary ad-delete-summary">
            <dt>将删除会话</dt><dd>{preview.sessions} 个（明确选择 {d.targets.length} 个）</dd>
            <dt>已知额外子会话</dt><dd>{preview.children} 个{d.includeChildren ? '（已包含）' : '（保留）'}</dd>
            <dt>将删除文件</dt><dd>{preview.native_files} 个，共 {(preview.bytes / 1024 / 1024).toFixed(2)} MB</dd>
            {preview.metadata_files ? <><dt>改写元数据</dt><dd>{preview.metadata_files} 个（状态库与共享索引，只删行不删文件）</dd></> : null}
          </dl>
          {preview.paths.length ? <ul className="ad-delete-paths">{preview.paths.map((path, index) => <li key={index}>{path}</li>)}</ul> : <p>没有仍存在的原始文件，将清理 AgentDeck 中的保留记录。</p>}
          <p className="ad-delete-hint">未选中的独立子会话将保留。Claude 主记录专属的子 Agent 文件属于该记录，已包含在上述范围内。</p>
        </details> : null}
        {preview && !d.backup ? <Checkbox className="ad-delete-ack" checked={d.confirmedEnded} disabled={d.busy} onChange={(_, data) => d.setConfirmedEnded(data.checked === true)} label={irreversibleAck} /> : null}
      </DialogContent>
      <DialogActions>
        {!preview && !d.loading && !d.busy ? <Button onClick={d.retryPreview}>重新预览</Button> : null}
        <Button disabled={d.busy} onClick={d.close}>取消</Button>
        <Button appearance="primary" className="ad-delete-confirm" icon={d.busy ? <Spinner size="extra-tiny" /> : <DeleteRegular />} disabled={!ready || (!d.backup && !d.confirmedEnded)} onClick={() => void d.remove()}>{d.busy ? '正在删除…' : '删除'}</Button>
      </DialogActions>
    </DialogBody></DialogSurface>
  </Dialog>
}

export function DeletedSessionsNotice({ deletion: d }: { deletion: SessionDeletion }) {
  if (!d.result && !d.restoredMessage) return null
  return <div className="ad-deletion-notice" role="status" style={{ backgroundColor: tokens.colorBrandBackground2, color: tokens.colorNeutralForeground1 }}>
    <div>
      <span>{d.result ? d.result.message || `已删除 ${d.result.deleted_keys.length} 个会话。` : d.restoredMessage}</span>
      {d.result?.backup_path ? <><div className="ad-delete-backup">{d.result.undo_token ? '恢复备份' : '未清理的临时备份'}：{d.result.backup_path}</div>{d.result.undo_token ? <div className="ad-delete-hint">可在本次运行期间撤销。备份文件将保留在本地磁盘。</div> : null}</> : null}
      {d.undoError ? <div role="alert" className="ad-delete-error">{d.undoError}</div> : null}
    </div>
    {d.result?.undo_token ? <Button size="small" icon={d.undoBusy ? <Spinner size="extra-tiny" /> : <ArrowUndoRegular />} disabled={d.undoBusy || d.busy || !!d.targets.length} onClick={() => void d.undo()}>{d.undoBusy ? '正在恢复…' : '撤销删除'}</Button> : null}
    <Button appearance="subtle" size="small" icon={<DismissRegular />} disabled={d.undoBusy} aria-label="关闭删除提示" onClick={d.dismissNotice} />
  </div>
}
