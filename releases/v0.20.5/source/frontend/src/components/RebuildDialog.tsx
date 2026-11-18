import { useEffect, useRef, useState } from 'react'
import { Button, Checkbox, Dialog, DialogActions, DialogBody, DialogContent, DialogSurface, DialogTitle, Field, Input, MessageBar, Radio, RadioGroup, Spinner } from '@fluentui/react-components'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import type { RebuildPreview } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { pickOpenPath } from '../nativeDialogs'

// Claude Code 只保留约 30 天的会话文件，超期的会被它清理；AgentDeck 的永久库里还留着内容。
// 这个对话框提供两条继续的路：重建成会话文件后用 claude --resume 继续（会在 Claude 的数据目录里
// 新建一个文件），或不写上游、新开一个会话并让它读一份整理好的历史。

export type RebuildTarget = { key: string; title: string; cwd: string }
const eventName = 'agentdeck:rebuild-session'
export const openRebuild = (target: RebuildTarget) => window.dispatchEvent(new CustomEvent<RebuildTarget>(eventName, { detail: target }))
// 后端错误经桥传来会带「RuntimeError: 」前缀，对话框里只留说明本身。
const readable = (reason: unknown) => String(reason).replace(/^\w*Error:\s*/, '')

type Mode = 'rebuild' | 'handoff'

export function RebuildDialog() {
  const [target, setTarget] = useState<RebuildTarget | null>(null)
  const [preview, setPreview] = useState<RebuildPreview | null>(null)
  const [path, setPath] = useState('')
  const [mode, setMode] = useState<Mode>('rebuild')
  const [openAfter, setOpenAfter] = useState(true)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState('')
  const generation = useRef(0)

  const load = async (key: string, dir: string) => {
    const sequence = ++generation.current
    setLoading(true); setError(''); setPreview(null)
    try {
      const next = await resumeService.PreviewRebuild(key, dir)
      if (sequence !== generation.current) return
      setPreview(next)
      setPath(next.directory || dir || next.original || '')
    } catch (reason) {
      if (sequence === generation.current) { setError(readable(reason)); if (dir) setPath(dir) }
    } finally { if (sequence === generation.current) setLoading(false) }
  }

  useEffect(() => {
    const listener = (event: Event) => {
      const next = (event as CustomEvent<RebuildTarget>).detail
      setTarget(next); setMode('rebuild'); setOpenAfter(true); setDone(''); setBusy(false)
      setPath(next.cwd || '')
      void load(next.key, '')
    }
    window.addEventListener(eventName, listener)
    return () => { window.removeEventListener(eventName, listener); generation.current++ }
  }, [])

  const close = () => { generation.current++; setTarget(null); setPreview(null); setError(''); setDone('') }
  const browse = async () => {
    if (!target || busy) return
    try {
      const selected = await pickOpenPath({ CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: false, Title: '选择继续此会话的目录', ButtonText: '选择目录' })
      if (typeof selected === 'string' && selected) { setPath(selected); void load(target.key, selected) }
    } catch (reason) { setError(readable(reason)) }
  }
  const submit = async () => {
    if (!target || !preview?.token || busy) return
    setBusy(true); setError('')
    try {
      if (mode === 'handoff') {
        await resumeService.ContinueWithHistory(target.key, preview.directory)
        close()
        return
      }
      const result = await resumeService.Rebuild(preview.token)
      if (!openAfter) { setDone(`已重建会话文件：${result.file}。现在可以在会话列表中选择“继续会话”。`); return }
      try {
        const opened = await resumeService.OpenAtResult(target.key, preview.directory)
        if (opened.warning) { setDone(`会话已重建，终端已打开，但有一条提示：${opened.warning}`); return }
        close()
      } catch (reason) {
        // 文件已经写好，只是终端没起来：如实说明，别让用户以为重建失败了
        setDone(`会话已重建（${result.file}），但无法打开终端：${readable(reason)}。可以稍后在会话列表中选择“继续会话”。`)
      }
    } catch (reason) { setError(readable(reason)) }
    finally { setBusy(false) }
  }

  const ready = !!preview?.token
  const dirty = !!preview && path.trim() !== (preview.directory || '')
  const content = preview ? `${preview.turns} 轮对话、${preview.kept} 条消息${preview.tools ? `，${preview.tools} 次工具调用（已转换为文本）` : ''}${preview.omitted ? `，较早的 ${preview.omitted} 条消息已省略` : ''}` : ''
  const submitLabel = mode === 'handoff' ? '新建会话并附带历史记录' : openAfter ? '重建并继续' : '重建'
  return <Dialog open={!!target} onOpenChange={(_, data) => { if (!data.open && !busy) close() }}>
    <DialogSurface onKeyDown={event => event.stopPropagation()}>
      <DialogBody>
        <DialogTitle>继续会话</DialogTitle>
        <DialogContent>
          <p className="ad-form-note">Claude Code 默认只保留约 30 天内的会话，此会话的原始记录已被它删除。AgentDeck 保留了一份副本，可通过以下两种方式继续。</p>
          <div className="ad-form">
            <Field label="会话"><Input readOnly value={target?.title || '未命名会话'} /></Field>
            <Field label="原项目目录"><Input readOnly value={preview?.original || target?.cwd || '未记录'} /></Field>
            <Field label="继续会话的目录" validationMessage={preview?.needs_directory ? '原项目目录已不存在，请选择一个现有目录。' : undefined} validationState={preview?.needs_directory ? 'warning' : 'none'}>
              <div className="ad-form-row">
                <Input aria-label="继续会话的目录" value={path} disabled={busy || !!done} onChange={(_, data) => setPath(data.value)} />
                <Button disabled={busy || !!done} onClick={() => void browse()}>选择目录…</Button>
                {dirty && target ? <Button disabled={busy || loading} onClick={() => void load(target.key, path.trim())}>应用</Button> : null}
              </div>
            </Field>
            {loading ? <p className="ad-form-note" role="status"><Spinner size="extra-tiny" /> 正在读取保留的副本…</p> : null}
            {ready && !done ? <>
              <RadioGroup value={mode} disabled={busy} onChange={(_, data) => setMode(data.value as Mode)} aria-label="继续方式">
                <Radio value="rebuild" label="重建会话文件后继续（推荐）" />
                <p className="ad-form-note" style={{ margin: '0 0 8px 28px' }}>在 Claude Code 的数据目录中新建一个会话文件（只新建，不会覆盖任何现有文件），然后使用 claude --resume 继续。工具调用与结果将转换为文本，不包含思考内容。重建后，Claude Code 将重新开始计算保留期限。</p>
                <Radio value="handoff" label="新建会话并附带历史记录" />
                <p className="ad-form-note" style={{ margin: '0 0 8px 28px' }}>不写入 Claude 的目录：将历史记录整理为一份文档，新建一个 Claude 会话，并让它先阅读该文档再继续。这不是真正的续接，历史记录很长时只包含最近的部分。</p>
              </RadioGroup>
              <p className="ad-form-note">将包含：{content}。</p>
              {mode === 'rebuild' ? <>
                <Field label="将创建的会话文件"><Input readOnly value={preview?.file ?? ''} /></Field>
                <Checkbox checked={openAfter} disabled={busy} label="完成后立即在终端中继续" onChange={(_, data) => setOpenAfter(data.checked === true)} />
              </> : null}
            </> : null}
            {done ? <MessageBar intent="success">{done}</MessageBar> : null}
            {error ? <MessageBar intent="error">{error}</MessageBar> : null}
          </div>
        </DialogContent>
        <DialogActions>
          <Button disabled={busy} onClick={close}>{done ? '关闭' : '取消'}</Button>
          {!done ? <Button appearance="primary" disabled={!ready || busy || loading || dirty} onClick={() => void submit()}>{busy ? '正在处理…' : submitLabel}</Button> : null}
        </DialogActions>
      </DialogBody>
    </DialogSurface>
  </Dialog>
}
