import { useEffect, useRef, useState } from 'react'
import { Button, Checkbox, Field, Input, MessageBar } from '@fluentui/react-components'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'
import { pickOpenPath } from '../nativeDialogs'
import { FormDialog } from './FormDialog'
import { executeLocation, performResume } from '../projectLocation'

type Request = { original: string; directory: string; notice?: string; action: 'resume' | 'copy' | 'save'; key?: string; remember?: boolean; resolve: (value: boolean) => void }
const eventName = 'agentdeck:project-location'
// 后端错误经桥传来会带「RuntimeError: 」前缀，对话框里只留说明本身。
const readable = (reason: unknown) => String(reason).replace(/^\w*Error:\s*/, '')

function chooseLocation(request: Omit<Request, 'resolve'>): Promise<boolean> {
  return new Promise(resolve => window.dispatchEvent(new CustomEvent(eventName, { detail: { ...request, resolve } })))
}

export async function resumeWithLocation(key: string, action: 'resume' | 'copy'): Promise<boolean> {
  return performResume(key, action, {
    prepare: key => resumeService.Prepare(key, ''),
    choose: preview => chooseLocation({ original: preview.original, directory: preview.directory, notice: preview.notice, key, action }),
    open: async key => { const result = await resumeService.OpenAtResult(key, ''); if (result.warning) throw new Error(result.warning) },
    copy: key => resumeService.CopyAt(key, ''),
  })
}

// 原目录好好的，只是想这一次换个目录继续（另一个检出、worktree、副本）：复用同一个对话框，
// 但默认不保存位置关联——关联是整个项目的，不该被一次临时选择悄悄改掉。
export function resumeInOtherDirectory(key: string, original: string): Promise<boolean> {
  return chooseLocation({ original, directory: original, key, action: 'resume', remember: false,
    notice: '仅本次在所选目录里继续此会话，不会修改会话记录，也不会移动文件。' })
}

export async function changeProjectLocation(original: string): Promise<boolean> {
  const directory = await projectService.Location(original)
  return chooseLocation({ original, directory, action: 'save' })
}

// 所有恢复入口共用同一对话框；校验结果仅应用于当前请求与当前输入。
export function ProjectLocationDialog() {
  const [request, setRequest] = useState<Request | null>(null)
  const active = useRef<Request | null>(null)
  const [path, setPath] = useState('')
  const [remember, setRemember] = useState(true)
  const [busy, setBusy] = useState(false)
  const [checking, setChecking] = useState(false)
  const [validated, setValidated] = useState('')
  const [directory, setDirectory] = useState('')
  const [command, setCommand] = useState('')
  const [validationError, setValidationError] = useState('')
  const [error, setError] = useState('')
  const [saved, setSaved] = useState('')
  const [completed, setCompleted] = useState(false)
  const pending = useRef(false)
  const generation = useRef(0)
  useEffect(() => {
    const listener = (event: Event) => {
      const next = (event as CustomEvent<Request>).detail
      if (active.current) { next.resolve(false); return }
      generation.current++
      // 「更改项目位置」预填当前有效位置：已有关联时它是关联目标。预填原目录会让
      // 「打开后直接保存」把输入值当成原目录而静默移除已有正确关联。恢复流程则
      // 预填原目录，用户要么确认它，要么另选一个。
      active.current = next; setRequest(next); setPath(next.action === 'save' ? next.directory || next.original : next.original || next.directory); setRemember(next.remember ?? !!next.original)
      setError(''); setSaved(''); setCompleted(false); setValidated(''); setDirectory(''); setCommand(''); setValidationError('')
    }
    window.addEventListener(eventName, listener)
    return () => { window.removeEventListener(eventName, listener); generation.current++; active.current?.resolve(false); active.current = null }
  }, [])
  const trimmed = path.trim()
  useEffect(() => {
    const sequence = ++generation.current
    setValidated(''); setDirectory(''); setCommand(''); setValidationError('')
    if (!request || !trimmed) { setChecking(false); return }
    setChecking(true)
    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          let resolved = trimmed, finalCommand = ''
          if (request.key) {
            const preview = await resumeService.Prepare(request.key, trimmed)
            if (preview.needsLocation) throw new Error(preview.notice || '所选目录无法用于恢复此会话，请重新选择。')
            resolved = preview.directory; finalCommand = preview.command
          } else if (await resumeService.DirectoryState(trimmed) !== 'dir') {
            throw new Error('所选目录不可用，请选择已存在的目录。')
          }
          if (sequence !== generation.current) return
          setDirectory(resolved); setCommand(finalCommand); setValidated(trimmed)
        } catch (reason) { if (sequence === generation.current) setValidationError(readable(reason)) }
        finally { if (sequence === generation.current) setChecking(false) }
      })()
    }, 250)
    return () => { clearTimeout(timer); generation.current++ }
  }, [request, trimmed])
  const close = (success = completed) => { generation.current++; active.current?.resolve(success); active.current = null; setRequest(null) }
  const browse = async () => {
    if (pending.current) return
    pending.current = true; setBusy(true)
    const current = active.current
    try {
      const selected = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: false, Title: '选择项目目录', ButtonText: '选择目录' })
      if (active.current === current && typeof selected === 'string' && selected) setPath(selected)
    } catch (reason) { if (active.current === current) setError(readable(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const submit = async () => {
    if (!request || pending.current || checking || validated !== trimmed || !trimmed || completed) return
    pending.current = true; setBusy(true); setError('')
    try {
      if (await resumeService.DirectoryState(directory) !== 'dir') throw new Error('所选目录不可用，请重新选择。')
      if (request.key) await resumeService.Prepare(request.key, directory)
      const warning = await executeLocation(request.action, request.original, directory, remember, {
        save: projectService.SetLocation, saved: setSaved,
        open: target => resumeService.OpenAtResult(request.key!, target),
        copy: target => resumeService.CopyAt(request.key!, target),
      })
      if (warning) { setCompleted(true); setError(warning); return }
      close(true)
    } catch (reason) { setError(readable(reason)) }
    finally { pending.current = false; setBusy(false) }
  }
  const editing = request?.action === 'save'
  return <FormDialog open={!!request} title={editing ? '更改项目位置' : '选择会话工作目录'}
    intro={<p className="ad-form-note">{editing ? '指定此项目的实际目录。位置关联仅保存在 AgentDeck 中，不会移动文件或修改原始会话记录。' : request?.notice || '当前项目目录无法用于此操作。请选择现有目录，并确认下方显示的工作目录和恢复命令。'}</p>}
    submitLabel={completed ? '终端已打开' : editing ? '保存' : request?.action === 'copy' ? '复制命令' : '继续会话'} busyLabel="处理中…" busy={busy} error={error}
    submitDisabled={!trimmed || checking || validated !== trimmed || completed} onSubmit={() => void submit()} onClose={() => close()}>
    <div className="ad-form">
      <Field label={editing ? '原项目目录' : '会话原项目目录'}><Input readOnly value={request?.original || '未记录'} /></Field>
      {request?.directory && request.directory.toLowerCase() !== request.original.toLowerCase() ? <Field label="当前关联目录"><Input readOnly value={request.directory} /></Field> : null}
      <Field label="所选项目目录" required validationMessage={validationError} validationState={validationError ? 'error' : 'none'}><div className="ad-form-row"><Input aria-label="所选项目目录" value={path} disabled={busy || completed} onChange={(_, data) => setPath(data.value)} style={{ flex: 1, minWidth: 0 }} /><Button disabled={busy || completed} onClick={() => void browse()}>浏览…</Button></div></Field>
      {checking ? <p className="ad-form-note" role="status">正在校验工作目录和恢复命令…</p> : null}
      {directory ? <Field label="本次工作目录"><Input readOnly value={directory} /></Field> : null}
      {command ? <Field label="将执行的恢复命令"><Input readOnly value={command} /></Field> : null}
      {!editing ? <Checkbox checked={remember} disabled={busy || completed || !request?.original} label="保存为此项目的位置关联" onChange={(_, data) => setRemember(data.checked === true)} /> : null}
      {!editing && !request?.original ? <p className="ad-form-note">此会话未记录原项目目录，无法保存位置关联。所选目录仅用于本次操作。</p> : null}
      {!editing && !remember && request?.original ? <p className="ad-form-note">本次操作使用所选目录，不更改已保存的位置关联。</p> : null}
      {saved ? <MessageBar intent="info">位置关联已保存：{saved}。关闭此窗口不会撤销保存；仅本次使用其他目录也不会更改此关联。</MessageBar> : null}
      {completed ? <p className="ad-form-note" role="status">终端已打开，无需再次启动。请关闭此窗口并在终端中继续操作。</p> : null}
    </div>
  </FormDialog>
}
