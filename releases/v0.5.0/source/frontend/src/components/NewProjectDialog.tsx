// 显式受控的开关避免 DialogTrigger 与异步路径建议竞争。
import { useEffect, useRef, useState } from 'react'
import {
  Dialog, DialogSurface, DialogTitle, DialogBody, DialogContent,
  DialogActions, Button, Input, Field, MessageBar, Checkbox,
} from '@fluentui/react-components'
import { Dialogs } from '@wailsio/runtime'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import { cliAgent } from '../agentOptions'
import { usePreferences } from '../hooks/usePreferences'
import { AgentSelect } from './AgentSelect'

export function NewProjectDialog({ open, current, initialAgent, onCreated, onChanged, onClose }: {
  open: boolean; current?: string; initialAgent?: string
  onCreated?: (path: string, agent: string) => void; onChanged: () => void; onClose: () => void
}) {
  const { preferences, update, error: preferencesError } = usePreferences()
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [agent, setAgent] = useState(() => cliAgent(initialAgent, preferences.defaultAgent))
  const [makeDefault, setMakeDefault] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const pathEdited = useRef(false)
  const pending = useRef(false)
  useEffect(() => {
    if (!open) return
    let active = true
    pathEdited.current = false
    setPath(''); setName(''); setError(''); setMakeDefault(false)
    setAgent(cliAgent(initialAgent, preferences.defaultAgent))
    // 后端根据当前项目父目录建议同级项目；只在首次打开时请求。
    projectService.DefaultPathFor(current ?? '', '新项目').then(suggestion => {
      if (active && !pathEdited.current) setPath(suggestion)
    }).catch(reason => { if (active) setError(String(reason)) })
    return () => { active = false }
    // 默认设置变化不应重置已输入的表单。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, current, initialAgent])

  const browse = async () => {
    if (pending.current) return
    pending.current = true; setBusy(true)
    try {
      const picked = await Dialogs.OpenFile({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: '选择项目目录', ButtonText: '选择目录' })
      if (typeof picked === 'string' && picked) { pathEdited.current = true; setPath(picked); setError('') }
    } catch (reason) { setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const submit = async () => {
    if (pending.current || !path.trim()) return
    pending.current = true; setBusy(true); setError('')
    try {
      const created = await projectService.CreateWithAgent(path.trim(), name.trim(), agent)
      if (makeDefault) update({ defaultAgent: agent })
      onChanged(); onCreated?.(created, agent); onClose()
    } catch (reason) { setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  return <Dialog open={open} onOpenChange={(_, data) => { if (!data.open && !pending.current) onClose() }}>
    <DialogSurface onKeyDown={event => {
      event.stopPropagation()
      if (event.key === 'Enter' && event.target instanceof HTMLInputElement && event.target.type === 'text' && !event.nativeEvent.isComposing && event.nativeEvent.keyCode !== 229) { event.preventDefault(); void submit() }
    }}><DialogBody>
      <DialogTitle>新建项目</DialogTitle>
      <DialogContent>
        <p>指定项目目录和默认智能体。目录不存在时将自动创建；创建项目后，可从项目菜单新建会话。</p>
        <Field label="项目目录" required>
          <div style={{ display: 'flex', gap: 8 }}><Input aria-label="项目目录" value={path} disabled={busy} onChange={(_, data) => { pathEdited.current = true; setPath(data.value) }} style={{ flex: 1, minWidth: 0 }} /><Button disabled={busy} onClick={() => void browse()}>浏览…</Button></div>
        </Field>
        <Field label="项目显示名称（可选）" validationMessage="留空时使用目录名称。" validationState="none" style={{ marginTop: 12 }}><Input value={name} disabled={busy} onChange={(_, data) => setName(data.value)} /></Field>
        <Field label="项目默认智能体" style={{ marginTop: 12 }}><AgentSelect value={agent} onChange={setAgent} disabled={busy} /></Field>
        <Checkbox label="同时设为应用默认智能体" checked={makeDefault} disabled={busy} onChange={(_, data) => setMakeDefault(data.checked === true)} />
        <p>使用前，请安装所选智能体的命令行工具并完成登录。此处暂不支持新建桌面客户端会话。</p>
        {error || preferencesError ? <MessageBar intent="error">{error || preferencesError}</MessageBar> : null}
      </DialogContent>
      <DialogActions><Button disabled={busy} onClick={onClose}>取消</Button><Button appearance="primary" disabled={busy || !path.trim()} onClick={() => void submit()}>{busy ? '创建中…' : '创建项目'}</Button></DialogActions>
    </DialogBody></DialogSurface>
  </Dialog>
}
