// 共用对话框外壳：忙态、就地错误、Enter 提交（含输入法保护）、关闭后焦点归还。
// 三处新建/修改弹窗的行为在这里只有一份，避免各写各的又各自漏掉某个键。
import { Button, Dialog, DialogActions, DialogBody, DialogContent, DialogSurface, DialogTitle, MessageBar } from '@fluentui/react-components'
import { useEffect, useRef, type ReactNode } from 'react'

export function FormDialog({ open, title, intro, submitLabel, busyLabel, busy, error, submitDisabled, submitAppearance, onSubmit, onClose, returnFocus, children }: {
  open: boolean
  title: string
  intro?: ReactNode
  submitLabel: string
  busyLabel: string
  busy: boolean
  error?: string
  submitDisabled?: boolean
  /** 破坏性动作（如移除设置）用红色主按钮，与普通保存区分开。 */
  submitAppearance?: 'primary' | 'destructive'
  onSubmit: () => void
  onClose: () => void
  /** 关闭后焦点落到哪个元素：触发按钮多半是会被卸载的菜单项，选中它没用。 */
  returnFocus?: string
  children: ReactNode
}) {
  const opener = useRef<HTMLElement | null>(null)
  const wasOpen = useRef(false)
  useEffect(() => {
    if (open && !wasOpen.current) opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    if (!open && wasOpen.current) {
      const previous = opener.current
      opener.current = null
      // 触发器还在（按钮没被卸载）就还给它；否则退到调用方指定的锚点。
      if (previous?.isConnected) previous.focus()
      else if (returnFocus) requestAnimationFrame(() => document.querySelector<HTMLElement>(returnFocus)?.focus())
    }
    wasOpen.current = open
  }, [open, returnFocus])

  return <Dialog open={open} onOpenChange={(_, data) => { if (!data.open && !busy) onClose() }}>
    <DialogSurface onKeyDown={event => {
      event.stopPropagation()
      // 输入法组合中的 Enter 是选词，不是提交（keyCode 229 是老浏览器的常用判据）。
      if (event.key !== 'Enter' || event.shiftKey || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) return
      if (!(event.target instanceof HTMLInputElement) || event.target.type !== 'text') return
      event.preventDefault()
      if (!busy && !submitDisabled) onSubmit()
    }}>
      <DialogBody>
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>
          {intro}
          {children}
          {error ? <MessageBar intent="error" style={{ marginTop: 12 }}>{error}</MessageBar> : null}
        </DialogContent>
        <DialogActions>
          <Button disabled={busy} onClick={onClose}>取消</Button>
          <Button appearance="primary" className={submitAppearance === 'destructive' ? 'ad-delete-confirm' : undefined} disabled={busy || submitDisabled} onClick={onSubmit}>{busy ? busyLabel : submitLabel}</Button>
        </DialogActions>
      </DialogBody>
    </DialogSurface>
  </Dialog>
}