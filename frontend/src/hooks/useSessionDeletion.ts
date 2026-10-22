import { useCallback, useEffect, useRef, useState } from 'react'
import type { DeletePreview, DeleteReport, SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import { deletionUnavailableReason } from '../sessionDeletion'

export function useSessionDeletion(onDeleted: (keys: string[]) => void, onRestored: () => void) {
  const [targets, setTargets] = useState<SessionView[]>([])
  const [includeChildren, setIncludeChildren] = useState(false)
  const [confirmedEnded, setConfirmedEnded] = useState(false)
  const [confirmedPermanent, setConfirmedPermanent] = useState(false)
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
    setPreview(null); setLoading(true); setConfirmedEnded(false); setConfirmedPermanent(false)
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
    setTargets(unique); setIncludeChildren(false); setError(''); setConfirmedEnded(false); setConfirmedPermanent(false)
    void loadPreview(unique, false)
  }
  const close = () => {
    if (operation.current) return
    request.current++; setTargets([]); setPreview(null); setLoading(false); setError(''); setConfirmedEnded(false); setConfirmedPermanent(false)
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
  // permanent=true 是「直接删除」：不保留恢复备份、无法撤销，需要额外确认。
  const remove = async (permanent = false) => {
    if (operation.current || loading || !preview?.token || !confirmedEnded) return
    if (permanent && !confirmedPermanent) return
    const token = preview.token
    operation.current = true; setBusy(true); setPreview(null); setError(''); setConfirmedEnded(false); setConfirmedPermanent(false)
    try {
      const report = await (permanent ? sessionService.DeleteSessionsPermanently(token) : sessionService.DeleteSessions(token))
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
  return { targets, includeChildren, confirmedEnded, setConfirmedEnded, confirmedPermanent, setConfirmedPermanent, preview, loading, busy, error, result, undoBusy, undoError, restoredMessage,
    open, close, changeChildren, retryPreview, remove, undo, dismissNotice: () => { if (!undoOperation.current) { setResult(null); setUndoError(''); setRestoredMessage('') } } }
}

export type SessionDeletion = ReturnType<typeof useSessionDeletion>
