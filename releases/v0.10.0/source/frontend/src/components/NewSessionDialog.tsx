// 新建会话：在某个工作目录里启动所选智能体。目录可手输、可浏览、可从最近
// 用过的目录里挑；路径在输入时就用后端同一套校验（预览命令）确认，所以
// 「这里能开」和「真的开」不会各说各话。
import { useEffect, useRef, useState } from 'react'
import { Button, Field, Input, MessageBar } from '@fluentui/react-components'
import { pickOpenPath } from '../nativeDialogs'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import type { CliAvailability } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { cliAgent, platformLabel, type CliAgent } from '../agentOptions'
import { usePreferences } from '../hooks/usePreferences'
import { missingCommand } from '../hooks/useAgentAvailability'
import { AgentSelect } from './AgentSelect'
import { FormDialog } from './FormDialog'

export function NewSessionDialog({ open, project, initialAgent, availability, recent, onLaunched, onClose }: {
  open: boolean
  /** 从项目进入时目录固定为该项目，不再让用户改。 */
  project?: string
  initialAgent?: string
  availability: Record<string, CliAvailability>
  recent: string[]
  onLaunched: (project: string, agent: string) => void | Promise<void>
  onClose: () => void
}) {
  const { preferences } = usePreferences()
  const [path, setPath] = useState(project ?? '')
  const [agent, setAgent] = useState<CliAgent>(preferences.defaultAgent)
  const [command, setCommand] = useState('')
  const [pathError, setPathError] = useState('')
  const [checking, setChecking] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState('')
  const pending = useRef(false)
  const generation = useRef(0)
  useEffect(() => {
    generation.current++
    if (open) { setPath(project ?? ''); setAgent(cliAgent(initialAgent, preferences.defaultAgent)); setError(''); setCommand(''); setPathError(''); setCopied('') }
    return () => { generation.current++ }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, project, initialAgent])

  // 目录与智能体共同决定这条命令能不能发出去：预览与真正启动走同一个后端
  // 入口，因此这里判定的结论就是启动时的结论。
  const trimmedPath = path.trim()
  useEffect(() => {
    if (!open || !trimmedPath) { setCommand(''); setPathError(''); setChecking(false); return }
    const request = ++generation.current
    setChecking(true)
    const timer = window.setTimeout(() => {
      resumeService.PreviewCommand(trimmedPath, agent).then(value => {
        if (request !== generation.current) return
        setCommand(value); setPathError(''); setChecking(false)
      }).catch(reason => {
        if (request !== generation.current) return
        setCommand(''); setPathError(String(reason)); setChecking(false)
      })
    }, 250)
    return () => clearTimeout(timer)
  }, [open, trimmedPath, agent])

  const missing = missingCommand(availability, agent)
  const blocked = !!pathError || checking

  const browse = async () => {
    if (pending.current) return
    pending.current = true; setBusy(true)
    const request = generation.current
    try {
      const picked = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: false, Title: '选择会话工作目录', ButtonText: '选择目录' })
      if (request === generation.current && typeof picked === 'string' && picked) setPath(picked)
    } catch (reason) { if (request === generation.current) setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command)
      setCopied('已复制')
    } catch {
      // 剪贴板不可用时说清楚怎么办，而不是静默失败。
      setCopied('复制失败，请手动选中命令后按 Ctrl+C')
    }
  }
  const submit = async () => {
    if (pending.current || !trimmedPath || blocked) return
    pending.current = true; setBusy(true); setError('')
    try { await onLaunched(trimmedPath, agent); onClose() }
    catch (reason) { setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const hints = !project && recent.length > 0 && recent.some(dir => dir.toLowerCase() !== trimmedPath.toLowerCase())

  return <FormDialog
    open={open}
    title="新建会话"
    returnFocus='[data-return-focus="new-session"]'
    intro={<p className="ad-form-note">{project ? '在该项目目录中打开终端并启动智能体。' : '在所选工作目录中打开终端并启动智能体。'}</p>}
    submitLabel="新建会话"
    busyLabel="启动中…"
    busy={busy}
    error={error}
    submitDisabled={!trimmedPath || blocked}
    onSubmit={() => void submit()}
    onClose={onClose}
  >
    <div className="ad-form">
      <Field label="工作目录" required validationMessage={pathError} validationState={pathError ? 'error' : 'none'}>
        <div className="ad-form-row">
          <Input aria-label="工作目录" value={path} readOnly={!!project} disabled={busy} onChange={(_, data) => setPath(data.value)} style={{ flex: 1, minWidth: 0 }} />
          {!project ? <Button disabled={busy} onClick={() => void browse()}>浏览…</Button> : null}
        </div>
      </Field>
      {hints ? <div className="ad-recent">
        <span className="ad-recent-label">最近使用</span>
        {recent.filter(dir => dir.toLowerCase() !== trimmedPath.toLowerCase()).map(dir => (
          <button key={dir} className="ad-recent-item" title={dir} onClick={() => setPath(dir)}>{dir}</button>
        ))}
      </div> : null}
      <Field label="智能体" validationMessage="仅用于本次会话，不更改项目或应用的默认智能体。" validationState="none">
        <AgentSelect value={agent} onChange={setAgent} disabled={busy} availability={availability} />
      </Field>
      {missing ? <MessageBar intent="warning">未在本机 PATH 中找到 {missing}。终端仍会打开，但需要先安装 {platformLabel(agent)} 的命令行工具才能启动会话。</MessageBar> : null}
      {command ? <details className="ad-command">
        <summary>将执行的命令</summary>
        <div className="ad-form-row">
          <Input aria-label="将执行的命令" readOnly value={command} style={{ flex: 1, minWidth: 0 }} />
          <Button disabled={busy} onClick={() => void copy()}>复制</Button>
        </div>
        {copied ? <p className="ad-form-note" role="status">{copied}</p> : null}
      </details> : checking && trimmedPath ? <p className="ad-form-note">正在确认目录与命令…</p> : null}
    </div>
  </FormDialog>
}