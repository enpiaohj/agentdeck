import { Button, Select, Spinner, Text } from '@fluentui/react-components'
import { FolderOpenRegular } from '@fluentui/react-icons'
import { useCallback, useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import type { TerminalShellView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import { pickOpenPath } from '../nativeDialogs'

// 终端 shell 设置：恢复 / 新建会话时启动哪一个 PowerShell。
//
// 取值由 Go 侧持有（appconfig 的 terminal_shell），因为启动终端发生在
// Go 侧，而浏览器偏好对 Go 侧不可见；这里经 SettingsService 读写，改动
// 立即对新开的终端生效。
const CUSTOM = 'custom'

// SHELL_LABELS 是各预置取值的中文名称。与后端的取值常量一一对应。
const SHELL_LABELS: Record<string, string> = {
  auto: '自动（优先 PowerShell 7+）',
  powershell: 'Windows PowerShell 5.1',
  pwsh: 'PowerShell 7+',
}

export function TerminalShellControl({ id }: { id: string }) {
  const [view, setView] = useState<TerminalShellView | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  // custom 是自定义路径输入框的内容；只有当前取值不是预置值时才非空。
  const [custom, setCustom] = useState('')
  const load = useCallback(() => {
    settingsService.TerminalShell().then(next => {
      setView(next)
      setCustom(isCustom(next.setting) ? next.setting : '')
    }).catch(reason => setError(`读取终端设置失败：${String(reason)}`))
  }, [])
  useEffect(load, [load])
  const save = async (value: string) => {
    if (saving) return
    setSaving(true)
    setError('')
    try {
      const next = await settingsService.SetTerminalShell(value)
      setView(next)
      setCustom(isCustom(next.setting) ? next.setting : '')
    } catch (reason) {
      setError(String(reason))
      // 保存失败时以实际生效的取值为准，避免界面显示一个并未生效的值。
      load()
    } finally {
      setSaving(false)
    }
  }
  const choose = async () => {
    try {
      const path = await pickOpenPath({
        CanChooseDirectories: false,
        CanChooseFiles: true,
        Title: '选择 PowerShell 可执行文件',
        ButtonText: '使用此文件',
        Filters: [{ DisplayName: 'PowerShell 可执行文件', Pattern: '*.exe' }],
      })
      if (!path) return
      setCustom(path)
      await save(path)
    } catch (reason) {
      setError(`选择文件失败：${String(reason)}`)
    }
  }
  const selected = view ? (isCustom(view.setting) ? CUSTOM : view.setting) : ''
  const note = error || (view?.error ?? '') || (view?.version ? `已检测到 ${view.version}` : '')
  return <div className="ad-terminal-shell">
    <Select id={id} aria-label="终端 PowerShell" value={selected} disabled={saving || !view}
      onChange={(_, data) => { if (data.value === CUSTOM) { setCustom(view?.setting && isCustom(view.setting) ? view.setting : ''); return } void save(data.value) }}>
      {(view?.options ?? []).map(option => <option key={option.value} value={option.value} disabled={!!option.error}>
        {SHELL_LABELS[option.value] ?? option.value}{option.error ? '（未检测到）' : ''}
      </option>)}
      <option value={CUSTOM}>自定义可执行文件…</option>
    </Select>
    {selected === CUSTOM ? <div className="ad-terminal-shell-path">
      <input type="text" aria-label="PowerShell 可执行文件路径" placeholder="C:\Program Files\PowerShell\7\pwsh.exe"
        value={custom} disabled={saving}
        onChange={event => setCustom(event.target.value)}
        onBlur={() => { if (custom && custom !== view?.setting) void save(custom) }}
        onKeyDown={(event) => { if (event.key === 'Enter' && custom && custom !== view?.setting) void save(custom) }} />
      <Button size="small" appearance="subtle" icon={<FolderOpenRegular />} disabled={saving} onClick={() => void choose()}>浏览…</Button>
    </div> : null}
    {saving || note ? <Text block className={`ad-terminal-shell-note${error || view?.error ? ' ad-terminal-shell-note-error' : ''}`} role={error || view?.error ? 'alert' : 'status'}>
      {saving ? <Spinner size="extra-tiny" /> : null}{saving ? '正在检测…' : note}
    </Text> : null}
  </div>
}

// isCustom 判断取值是否为自定义可执行文件路径（而非预置值）。
function isCustom(setting: string): boolean {
  return !!setting && !(setting in SHELL_LABELS)
}