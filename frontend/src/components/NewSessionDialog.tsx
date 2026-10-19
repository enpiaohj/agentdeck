import { useEffect, useRef, useState } from 'react'
import { Dialog, DialogSurface, DialogBody, DialogTitle, DialogContent, DialogActions, Button, Field, Input, MessageBar } from '@fluentui/react-components'
import { Dialogs } from '@wailsio/runtime'
import { cliAgent, type CliAgent } from '../agentOptions'
import { usePreferences } from '../hooks/usePreferences'
import { AgentSelect } from './AgentSelect'

export function NewSessionDialog({ open, project, initialAgent, onLaunch, onClose }: {
  open: boolean
  project?: string
  initialAgent?: string
  onLaunch: (project: string, agent: string) => Promise<void>
  onClose: () => void
}) {
  const { preferences } = usePreferences()
  const [path, setPath] = useState(project ?? '')
  const [agent, setAgent] = useState<CliAgent>(preferences.defaultAgent)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const pending = useRef(false)
  const generation = useRef(0)
  useEffect(() => {
    generation.current++
    if (open) { setPath(project ?? ''); setAgent(cliAgent(initialAgent, preferences.defaultAgent)); setError('') }
    return () => { generation.current++ }
  }, [open, project, initialAgent])
  const browse = async () => {
    if (pending.current) return
    pending.current = true; setBusy(true)
    const request = generation.current
    try {
      const picked = await Dialogs.OpenFile({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: false, Title: '选择会话工作目录', ButtonText: '选择目录' })
      if (request === generation.current && typeof picked === 'string' && picked) { setPath(picked); setError('') }
    } catch (reason) { if (request === generation.current) setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const submit = async () => {
    if (pending.current || !path.trim()) return
    pending.current = true; setBusy(true); setError('')
    try { await onLaunch(path.trim(), agent); onClose() }
    catch (reason) { setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  return <Dialog open={open} onOpenChange={(_, data) => { if (!data.open && !pending.current) onClose() }}>
    <DialogSurface onKeyDown={event => {
      event.stopPropagation()
      if (event.key === 'Enter' && event.target instanceof HTMLInputElement && !event.nativeEvent.isComposing && event.nativeEvent.keyCode !== 229) { event.preventDefault(); void submit() }
    }}><DialogBody><DialogTitle>新建会话</DialogTitle>
      <DialogContent>
        <p>在所选工作目录中打开终端并启动智能体。生成会话记录后，可刷新列表查看。</p>
        <Field label="工作目录" required>
          <div style={{ display: 'flex', gap: 6 }}>
            <Input value={path} readOnly={!!project} disabled={busy} onChange={(_, data) => setPath(data.value)} style={{ flex: 1, minWidth: 0 }} />
            {!project ? <Button disabled={busy} onClick={() => void browse()}>浏览…</Button> : null}
          </div>
        </Field>
        <Field label="智能体" style={{ marginTop: 12 }} validationMessage="仅用于本次会话，不更改项目或应用的默认智能体。" validationState="none">
          <AgentSelect value={agent} onChange={setAgent} disabled={busy} />
        </Field>
        {error ? <MessageBar intent="error" style={{ marginTop: 12 }}>{error}</MessageBar> : null}
      </DialogContent>
      <DialogActions><Button disabled={busy} onClick={onClose}>取消</Button><Button appearance="primary" disabled={busy || !path.trim()} onClick={() => void submit()}>{busy ? '启动中…' : '新建会话'}</Button></DialogActions>
    </DialogBody></DialogSurface>
  </Dialog>
}
