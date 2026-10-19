// 新建项目对话框：选/填一个目录与名字，后端建目录并记进声明清单。
//
// 目录用「浏览…」选而不是只让手填：手打路径在这个 WebView2 输入框里会
// 撞上 IME 与全选失效（见 CLAUDE.md 陷阱 #11），打出来的值和看到的不一致。
// 系统目录对话框给出的路径没有这个问题；输入框保留，便于在选中的基础上改。
import { useState } from 'react'
import {
  Dialog, DialogTrigger, DialogSurface, DialogTitle, DialogBody,
  DialogContent, DialogActions, Button, Input, Field, MessageBar,
} from '@fluentui/react-components'
import { DismissRegular, FolderRegular } from '@fluentui/react-icons'
import { Dialogs } from '@wailsio/runtime'
import * as projectService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/projectservice'

export function NewProjectDialog({ current, onCreated, onChanged }: {
  current?: string
  onCreated?: (path: string) => void
  onChanged: () => void
}) {
  const [open, setOpen] = useState(false)
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const start = async () => {
    // 初始路径：当前项目目录下的一个新子目录；没有就文档目录下的子目录。
    try {
      const suggestion = await projectService.DefaultPathFor(current ?? '', '新项目')
      setPath(suggestion)
      setName('')
      setError('')
      setOpen(true)
    } catch (reason) {
      setError(String(reason))
    }
  }

  // 选目录：允许在对话框里直接新建文件夹，路径由系统给出。
  const browse = async () => {
    try {
      const picked = await Dialogs.OpenFile({
        CanChooseDirectories: true,
        CanChooseFiles: false,
        CanCreateDirectories: true,
        Title: '选择新项目的目录',
        ButtonText: '用这个目录',
      })
      if (picked && typeof picked === 'string') {
        setPath(picked)
        setError('')
      }
    } catch (reason) {
      setError(String(reason))
    }
  }

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      const created = await projectService.Create(path.trim(), name.trim())
      setOpen(false)
      onChanged()
      onCreated?.(created)
    } catch (reason) {
      setError(String(reason))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(_, d) => { if (!busy) setOpen(d.open) }}>
      <DialogTrigger disableButtonEnhancement>
        <Button
          size="small"
          appearance="subtle"
          icon={<span aria-hidden="true" style={{ fontSize: 15, lineHeight: 1 }}>＋</span>}
          onClick={() => void start()}
          title="新建项目（建目录并加入列表）"
        >
          新建项目
        </Button>
      </DialogTrigger>
      <DialogSurface>
        <DialogBody>
          <DialogTitle>新建项目</DialogTitle>
          <DialogContent>
            目录会被创建出来，之后可以在里面开新的 Agent 会话。
            <Field label="目录" required style={{ marginTop: 12 }}>
              <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                <Input
                  value={path}
                  disabled={busy}
                  onChange={(_, d) => setPath(d.value)}
                  placeholder="D:\Work\新项目"
                  style={{ flex: 1, minWidth: 0 }}
                />
                <Button
                  size="small"
                  appearance="subtle"
                  icon={<FolderRegular />}
                  disabled={busy}
                  onClick={() => void browse()}
                  title="选择目录（可在弹出的对话框里直接新建文件夹）"
                >
                  浏览…
                </Button>
              </div>
            </Field>
            <Field label="显示名（留空用目录名）" style={{ marginTop: 10 }}>
              <Input value={name} disabled={busy} onChange={(_, d) => setName(d.value)} />
            </Field>
            {error ? <MessageBar intent="error" style={{ marginTop: 10 }}>{error}</MessageBar> : null}
          </DialogContent>
          <DialogActions>
            <Button appearance="subtle" icon={<DismissRegular />} disabled={busy} onClick={() => setOpen(false)}>取消</Button>
            <Button appearance="primary" disabled={busy || !path.trim()} onClick={() => void submit()}>
              {busy ? '创建中…' : '创建'}
            </Button>
          </DialogActions>
        </DialogBody>
      </DialogSurface>
    </Dialog>
  )
}
