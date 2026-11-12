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
  const [customMode, setCustomMode] = useState(false)
  const load = useCallback((keepError = false) => {
    return settingsService.TerminalShell().then(next => {
      setView(next)
      setCustom(isCustom(next.setting) ? next.setting : '')
      setCustomMode(isCustom(next.setting))
      if (!keepError) setError('')
    }).catch(reason => setError(`读取终端设置失败：${String(reason)}`))
  }, [])
  useEffect(() => { void load() }, [load])
  const save = async (value: string) => {
    if (saving) return
    setSaving(true)
    setError('')
    try {
      const next = await settingsService.SetTerminalShell(value)
      setView(next)
      setCustom(isCustom(next.setting) ? next.setting : '')
      setCustomMode(isCustom(next.setting))
    } catch (reason) {
      setError(String(reason))
      // 保存失败时以实际生效的取值为准，避免界面显示一个并未生效的值。
      await load(true)
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
  const selected = view ? (customMode ? CUSTOM : view.setting) : ''
  const note = error || (view?.error ?? '') || (view?.version ? `已检测到 ${view.version}` : '')
  return <div className="ad-terminal-shell">
    <Select id={id} aria-label="终端 PowerShell" value={selected} disabled={saving || !view}
      onChange={(_, data) => { if (data.value === CUSTOM) { setCustomMode(true); setCustom(view?.setting && isCustom(view.setting) ? view.setting : ''); return } void save(data.value) }}>
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
    {!view && error ? <Button size="small" onClick={() => void load()}>重试读取终端设置</Button> : null}
  </div>
}

// isCustom 判断取值是否为自定义可执行文件路径（而非预置值）。
function isCustom(setting: string): boolean {
  return !!setting && !(setting in SHELL_LABELS)
}

// useTerminalSettings 是「继续会话」两项偏好的共用读写状态。
//
// 两项取值都存在 Go 侧的 appconfig（resume_maximized / resume_open_mode），
// 因为打开终端发生在 Go 侧：只有那里能把它交给 wt 的 -M、-w 0 nt，或回退
// shell 的 -WindowStyle。所有显示值都来自后端，保存失败时回读实际生效值，
// 避免界面显示一个并未生效的选择。
function useTerminalSettings() {
  const [view, setView] = useState<TerminalShellView | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const load = useCallback((keepError = false) => {
    return settingsService.TerminalShell().then(next => {
      setView(next)
      if (!keepError) setError('')
    }).catch(reason => setError(`读取终端设置失败：${String(reason)}`))
  }, [])
  useEffect(() => { void load() }, [load])
  const save = async (action: () => Promise<TerminalShellView>) => {
    if (saving || !view) return
    setSaving(true)
    setError('')
    try {
      setView(await action())
    } catch (reason) {
      setError(String(reason))
      await load(true)
    } finally {
      setSaving(false)
    }
  }
  return { view, saving, error, load, save }
}

// OPEN_MODE_LABELS 是打开方式的中文名称，与 appconfig 的取值一一对应。
// 选项名保持简短，「在活动终端窗口里开」这层含义放在设置行的说明里，
// 并由 aria-describedby 关联到控件上。
const OPEN_MODE_LABELS: Record<string, string> = {
  window: '新窗口',
  tab: '新标签',
}

// OPEN_MODE_HINTS 是各选项的悬浮说明：选项名很短，具体落在哪个窗口由这里说清。
const OPEN_MODE_HINTS: Record<string, string> = {
  window: '每次继续会话都新开一个终端窗口。',
  tab: '在活动的 Windows Terminal 窗口中新开一个标签；没有打开的终端窗口时新建一个。',
}

// ResumeOpenModeControl 是继续会话时终端的打开方式：新窗口，或在活动的终端
// 窗口里开新标签（wt 的 -w 0 nt）。没有 Windows Terminal 时「新标签」置灰——
// 回退路径只能开新窗口，不该让用户选一个不生效的值。
export function ResumeOpenModeControl({ id }: { id: string }) {
  const { view, saving, error, load, save } = useTerminalSettings()
  const tabSupported = view?.tab_supported ?? false
  return <div className="ad-terminal-shell">
    <Select id={id} aria-label="继续会话时的打开方式" value={view?.resume_open_mode || 'window'}
      disabled={saving || !view}
      onChange={(_, data) => void save(() => settingsService.SetResumeOpenMode(data.value))}>
      {Object.entries(OPEN_MODE_LABELS).map(([value, label]) => <option key={value} value={value} title={OPEN_MODE_HINTS[value]} disabled={value === 'tab' && !tabSupported}>
        {label}{value === 'tab' && !tabSupported ? '（未检测到 Windows Terminal）' : ''}
      </option>)}
    </Select>
    {saving ? <Text block className="ad-terminal-shell-note" role="status"><Spinner size="extra-tiny" />正在保存…</Text> : null}
    {error ? <Text block className="ad-terminal-shell-note ad-terminal-shell-note-error" role="alert">{error}
      {!view ? <Button size="small" onClick={() => void load()}>重试读取终端设置</Button> : null}
    </Text> : null}
  </div>
}

// ResumeMaximizedControl 是「继续会话时最大化打开终端」开关。默认关闭：
// 铺满屏幕不能替用户默认做主。
//
// 实测限制：-M 只在 Windows Terminal 新建窗口时生效，复用已有窗口开标签时
// 被忽略（目标窗口尺寸由用户自己决定，不去改它）。
export function ResumeMaximizedControl({ id }: { id: string }) {
  const { view, saving, error, load, save } = useTerminalSettings()
  return <div className="ad-terminal-shell">
    <input id={id} type="checkbox" role="switch" aria-label="继续会话时最大化打开终端"
      aria-describedby={`${id}-description`} checked={view?.resume_maximized ?? false}
      disabled={saving || !view} onChange={event => void save(() => settingsService.SetResumeMaximized(event.target.checked))} />
    {saving ? <Text block className="ad-terminal-shell-note" role="status"><Spinner size="extra-tiny" />正在保存…</Text> : null}
    {error ? <Text block className="ad-terminal-shell-note ad-terminal-shell-note-error" role="alert">{error}
      {!view ? <Button size="small" onClick={() => void load()}>重试读取终端设置</Button> : null}
    </Text> : null}
  </div>
}
