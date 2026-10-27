import { useCallback, useEffect, useRef, useState } from 'react'
import type { DeletePreview, DeleteReport, SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { deletionUnavailableReason } from '../sessionDeletion'
import { usePreferences } from './usePreferences'

export function useSessionDeletion(onDeleted: (keys: string[]) => void, onRestored: () => void) {
  const [targets, setTargets] = useState<SessionView[]>([])
  const [includeChildren, setIncludeChildren] = useState(false)
  const [confirmedEnded, setConfirmedEnded] = useState(false)
  // 删除前是否先写恢复备份，由「设置 → 会话 → 删除会话」决定，对话框里不再二选一。
  const backup = usePreferences().preferences.deleteBackup
  const [preview, setPreview] = useState<DeletePreview | null>(null)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<DeleteReport | null>(null)
  const [undoBusy, setUndoBusy] = useState(false)
  const [undoError, setUndoError] = useState('')
  const [restoredMessage, setRestoredMessage] = useState('')
  const request = useRef(0)
  const operation = useRef(false)
  const undoOperation = useRef(false)

  const loadPreview = useCallback(async (items: SessionView[], children: boolean) => {
    const version = ++request.current
    setPreview(null); setLoading(true); setConfirmedEnded(false)
    try {
      const next = await sessionService.PreviewDeleteSessions(items.map(item => item.key), children)
      if (request.current === version) setPreview(next)
    } catch (reason) {
      if (request.current === version) setError(current => [current, `无法预览删除范围：${String(reason)}`].filter(Boolean).join('\n'))
    } finally { if (request.current === version) setLoading(false) }
  }, [])

  useEffect(() => () => { request.current++ }, [])

  const open = (items: SessionView[]) => {
    if (operation.current || undoOperation.current || targets.length) return
    const reason = deletionUnavailableReason(items)
    if (reason) return
    const unique = [...new Map(items.map(item => [item.key, item])).values()]
    setTargets(unique); setIncludeChildren(false); setError(''); setConfirmedEnded(false)
    void loadPreview(unique, false)
  }
  const close = () => {
    if (operation.current) return
    request.current++; setTargets([]); setPreview(null); setLoading(false); setError(''); setConfirmedEnded(false)
  }
  const changeChildren = (value: boolean) => {
    if (operation.current) return
    setIncludeChildren(value); setError('')
    void loadPreview(targets, value)
  }
  const retryPreview = () => {
    if (operation.current) return
    setError(''); void loadPreview(targets, includeChildren)
  }
  // 可撤销（默认会先备份）时点「删除」即可；不保留备份时多一次「了解无法撤销」的勾选。
  const remove = async () => {
    if (operation.current || loading || !preview?.token || (!backup && !confirmedEnded)) return
    const token = preview.token
    operation.current = true; setBusy(true); setPreview(null); setError(''); setConfirmedEnded(false)
    try {
      const report = await (backup ? sessionService.DeleteSessions(token) : sessionService.DeleteSessionsPermanently(token))
      setResult(report); setUndoError(''); setRestoredMessage(''); setTargets([])
      onDeleted(report.deleted_keys)
    } catch (reason) {
      setError(`删除未完成：${String(reason)}。请检查以下最新预览后重新确认。`)
      // A failed operation consumes its token too; never retry a stale plan.
      await loadPreview(targets, includeChildren)
    } finally { operation.current = false; setBusy(false) }
  }
  const undo = async () => {
    if (!result?.undo_token || operation.current || undoOperation.current || targets.length) return
    undoOperation.current = true; setUndoBusy(true); setUndoError('')
    try {
      const message = await sessionService.RestoreDeletedSessions(result.undo_token)
      setRestoredMessage(message || '会话已恢复。'); setResult(null); onRestored()
    } catch (reason) { setUndoError(`撤销失败：${String(reason)}。备份仍保存在：${result.backup_path}`) }
    finally { undoOperation.current = false; setUndoBusy(false) }
  }
  return { targets, includeChildren, confirmedEnded, setConfirmedEnded, backup, preview, loading, busy, error, result, undoBusy, undoError, restoredMessage,
    open, close, changeChildren, retryPreview, remove, undo, dismissNotice: () => { if (!undoOperation.current) { setResult(null); setUndoError(''); setRestoredMessage('') } } }
}

export type SessionDeletion = ReturnType<typeof useSessionDeletion>
