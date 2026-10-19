// 新建项目：目录、显示名称、默认智能体。显式受控的开关避免 DialogTrigger
// 与异步路径建议竞争。
//
// 目录的输入是即时校验的：已在项目列表里的目录直接拦下并指路，指向文件的
// 路径也拦下，不存在的目录明确说明「会一并创建」——不再等到点创建才报错。
import { useEffect, useMemo, useRef, useState } from 'react'
import { Button, Checkbox, Field, Input, MessageBar } from '@fluentui/react-components'
import { pickOpenPath } from '../nativeDialogs'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import type { CliAvailability, ProjectView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { cliAgent, platformLabel } from '../agentOptions'
import { usePreferences } from '../hooks/usePreferences'
import { missingCommand } from '../hooks/useAgentAvailability'
import { AgentSelect } from './AgentSelect'
import { FormDialog } from './FormDialog'

type DirState = 'unknown' | 'dir' | 'missing' | 'file'

// 路径比较键：斜杠归一、去掉尾部分隔符、大小写折叠。只用于「是不是同一个
// 目录」的提示，不改变真正提交的原文。
function pathKey(path: string): string {
  return path.replace(/\//g, '\\').replace(/\\+$/, '').toLowerCase()
}

// folderName 取目录名，用于显示名称留空时的预览。
function folderName(path: string): string {
  const parts = path.replace(/[\\/]+$/, '').split(/[\\/]/)
  return parts[parts.length - 1] ?? ''
}

export function NewProjectDialog({ open, current, initialAgent, availability, onCreated, onChanged, onClose }: {
  open: boolean
  current?: string
  initialAgent?: string
  availability: Record<string, CliAvailability>
  onCreated?: (path: string, agent: string) => void
  onChanged: () => void
  onClose: () => void
}) {
  const { preferences, update, error: preferencesError } = usePreferences()
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [agent, setAgent] = useState(() => cliAgent(initialAgent, preferences.defaultAgent))
  const [makeDefault, setMakeDefault] = useState(false)
  const [existing, setExisting] = useState<ProjectView[]>([])
  const [state, setState] = useState<DirState>('unknown')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const pathEdited = useRef(false)
  const pending = useRef(false)
  useEffect(() => {
    if (!open) return
    let active = true
    pathEdited.current = false
    setPath(''); setName(''); setError(''); setMakeDefault(false); setState('unknown')
    setAgent(cliAgent(initialAgent, preferences.defaultAgent))
    // 后端根据当前项目父目录建议同级项目；只在首次打开时请求。
    projectService.DefaultPathFor(current ?? '', '新项目').then(suggestion => {
      if (active && !pathEdited.current) setPath(suggestion)
    }).catch(reason => { if (active) setError(String(reason)) })
    // 已在项目列表里的目录要能当场认出来，所以打开时拉一次项目清单。
    sessionService.Projects().then(rows => { if (active) setExisting(rows) }).catch(() => { if (active) setExisting([]) })
    return () => { active = false }
    // 默认设置变化不应重置已输入的表单。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, current, initialAgent])

  // 目录状态随输入变化重新探测；防抖 250ms，避免逐字符敲路径时反复问后端。
  const trimmedPath = path.trim()
  useEffect(() => {
    if (!open || !trimmedPath) { setState('unknown'); return }
    let active = true
    const timer = window.setTimeout(() => {
      resumeService.DirectoryState(trimmedPath)
        .then(value => { if (active) setState(value === 'dir' || value === 'file' ? value : 'missing') })
        .catch(() => { if (active) setState('unknown') })
    }, 250)
    return () => { active = false; clearTimeout(timer) }
  }, [open, trimmedPath])

  const duplicate = useMemo(
    () => existing.find(row => pathKey(row.path) === pathKey(trimmedPath)),
    [existing, trimmedPath],
  )
  const nameError = name.trim() && (name.trim().length > 200 || /[\u0000-\u001f\u007f]/.test(name)) ? '显示名称不能超过 200 个字符，也不能包含换行等控制字符。' : ''
  const pathError = !trimmedPath ? ''
    : duplicate ? `该目录已在项目列表中${duplicate.name ? `（显示名称：${duplicate.name}）` : ''}。请改用项目菜单新建会话或修改显示名称。`
    : state === 'file' ? '该路径被一个同名文件占用，不能作为项目目录。'
    : /[\u0000-\u001f\u007f]/.test(trimmedPath) ? '路径不能包含换行等控制字符。' : ''
  const blocked = !!pathError || !!nameError
  const missing = missingCommand(availability, agent)

  const browse = async () => {
    if (pending.current) return
    pending.current = true; setBusy(true)
    try {
      const picked = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true, Title: '选择项目目录', ButtonText: '选择目录' })
      if (typeof picked === 'string' && picked) { pathEdited.current = true; setPath(picked); setError('') }
    } catch (reason) { setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const submit = async () => {
    if (pending.current || !trimmedPath || blocked) return
    pending.current = true; setBusy(true); setError('')
    try {
      const created = await projectService.CreateWithAgent(trimmedPath, name.trim(), agent)
      if (makeDefault) update({ defaultAgent: agent })
      onChanged(); onCreated?.(created, agent); onClose()
    } catch (reason) { setError(String(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const shownName = name.trim() || folderName(trimmedPath) || '项目名称'

  return <FormDialog
    open={open}
    title="新建项目"
    returnFocus='[data-return-focus="new-project"]'
    intro={<p className="ad-form-note">指定项目目录和默认智能体。目录不存在时会自动创建。</p>}
    submitLabel="创建项目"
    busyLabel="创建中…"
    busy={busy}
    error={error || preferencesError}
    submitDisabled={!trimmedPath || blocked}
    onSubmit={() => void submit()}
    onClose={onClose}
  >
    <div className="ad-form">
      <Field label="项目目录" required validationMessage={pathError} validationState={pathError ? 'error' : 'none'}>
        <div className="ad-form-row">
          <Input aria-label="项目目录" value={path} disabled={busy} onChange={(_, data) => { pathEdited.current = true; setPath(data.value) }} style={{ flex: 1, minWidth: 0 }} />
          <Button disabled={busy} onClick={() => void browse()}>浏览…</Button>
        </div>
      </Field>
      {!pathError && trimmedPath ? <p className="ad-form-note">
        {state === 'dir' ? '目录已存在，创建后直接加入项目列表。' : state === 'missing' ? '目录尚不存在，创建项目时会一并新建。' : '正在确认目录状态…'}
      </p> : null}
      <Field label="项目显示名称（可选）" validationMessage={nameError || '留空时使用目录名称。'} validationState={nameError ? 'error' : 'none'}>
        <Input aria-label="项目显示名称" value={name} disabled={busy} onChange={(_, data) => setName(data.value)} />
      </Field>
      <Field label="项目默认智能体" validationMessage={`在项目菜单新建会话时优先使用；当前为 ${platformLabel(agent)}。`} validationState="none">
        <AgentSelect value={agent} onChange={setAgent} disabled={busy} availability={availability} />
      </Field>
      <Checkbox label="同时设为应用默认智能体" checked={makeDefault} disabled={busy} onChange={(_, data) => setMakeDefault(data.checked === true)} />
      <p className="ad-form-note">项目列表中将显示为「{shownName}」。使用前请先安装所选智能体的命令行工具并完成登录。</p>
      {missing ? <MessageBar intent="warning">未在本机 PATH 中找到 {missing}。仍可创建项目，但需先安装 {platformLabel(agent)} 的命令行工具才能启动会话。</MessageBar> : null}
    </div>
  </FormDialog>
}