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
      if (!openAfter) { setDone(`已重建会话文件：${result.file}。现在可以在会话列表里点“继续会话”。`); return }
      try {
        const opened = await resumeService.OpenAtResult(target.key, preview.directory)
        if (opened.warning) { setDone(`会话已重建，终端已打开，但有一条提示：${opened.warning}`); return }
        close()
      } catch (reason) {
        // 文件已经写好，只是终端没起来：如实说明，别让用户以为重建失败了
        setDone(`会话已重建（${result.file}），但打开终端失败：${readable(reason)}。可在会话列表里再点“继续会话”。`)
      }
    } catch (reason) { setError(readable(reason)) }
    finally { setBusy(false) }
  }

  const ready = !!preview?.token
  const dirty = !!preview && path.trim() !== (preview.directory || '')
  const content = preview ? `${preview.turns} 轮对话、${preview.kept} 条消息${preview.tools ? `，${preview.tools} 次工具调用转成文字` : ''}${preview.omitted ? `，较早的 ${preview.omitted} 条已省略` : ''}` : ''
  const submitLabel = mode === 'handoff' ? '新开会话并带上历史' : openAfter ? '重建并继续会话' : '重建会话'
  return <Dialog open={!!target} onOpenChange={(_, data) => { if (!data.open && !busy) close() }}>
    <DialogSurface onKeyDown={event => event.stopPropagation()}>
      <DialogBody>
        <DialogTitle>继续已被清理的会话</DialogTitle>
        <DialogContent>
          <p className="ad-form-note">Claude Code 只保留约 30 天的会话，这个会话的原记录已被它清理。AgentDeck 的永久库里还留着内容，可以用下面两种方式继续。</p>
          <div className="ad-form">
            <Field label="会话"><Input readOnly value={target?.title || '未命名会话'} /></Field>
            <Field label="原目录"><Input readOnly value={preview?.original || target?.cwd || '未记录'} /></Field>
            <Field label="在此目录继续" validationMessage={preview?.needs_directory ? '原目录已不存在，请选择一个已有的目录。' : undefined} validationState={preview?.needs_directory ? 'warning' : 'none'}>
              <div className="ad-form-row">
                <Input aria-label="在此目录继续" value={path} disabled={busy || !!done} onChange={(_, data) => setPath(data.value)} />
                <Button disabled={busy || !!done} onClick={() => void browse()}>选择目录…</Button>
                {dirty && target ? <Button disabled={busy || loading} onClick={() => void load(target.key, path.trim())}>应用</Button> : null}
              </div>
            </Field>
            {loading ? <p className="ad-form-note" role="status"><Spinner size="extra-tiny" /> 正在读取保留的会话内容…</p> : null}
            {ready && !done ? <>
              <RadioGroup value={mode} disabled={busy} onChange={(_, data) => setMode(data.value as Mode)} aria-label="继续方式">
                <Radio value="rebuild" label="重建后继续此会话（推荐）" />
                <p className="ad-form-note" style={{ margin: '0 0 8px 28px' }}>在 Claude Code 的数据目录里新建一个会话文件（只新建，不会覆盖任何已有文件），然后用 claude --resume 接着聊。工具调用与结果只保留文字描述，思考内容不会带回。重建后 Claude Code 会重新按保留期计时。</p>
                <Radio value="handoff" label="新开会话并带上历史" />
                <p className="ad-form-note" style={{ margin: '0 0 8px 28px' }}>不写 Claude 的目录：把历史整理成一份文档，新开一个 Claude 会话，让它先读完再继续。不是真正的续接，历史很长时只带最近的部分。</p>
              </RadioGroup>
              <p className="ad-form-note">将带回：{content}。</p>
              {mode === 'rebuild' ? <>
                <Field label="将写入的会话文件"><Input readOnly value={preview?.file ?? ''} /></Field>
                <Checkbox checked={openAfter} disabled={busy} label="完成后立即在终端里继续" onChange={(_, data) => setOpenAfter(data.checked === true)} />
              </> : null}
            </> : null}
            {done ? <MessageBar intent="success">{done}</MessageBar> : null}
            {error ? <MessageBar intent="error">{error}</MessageBar> : null}
          </div>
        </DialogContent>
        <DialogActions>
          <Button disabled={busy} onClick={close}>{done ? '关闭' : '取消'}</Button>
          {!done ? <Button appearance="primary" disabled={!ready || busy || loading || dirty} onClick={() => void submit()}>{busy ? '处理中…' : submitLabel}</Button> : null}
        </DialogActions>
      </DialogBody>
    </DialogSurface>
  </Dialog>
}
