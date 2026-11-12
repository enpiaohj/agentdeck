import { Select, Text, tokens } from '@fluentui/react-components'
import { useCallback, useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import type { BrowserOption, BrowserSettingsView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'

// 打开网页链接所用的浏览器。
//
// 取值由 Go 侧持有（appconfig 的 browser），因为打开发生在 Go 侧。用户常在某个
// 特定的浏览器里登录了 ChatGPT，而系统默认浏览器往往不是它，所以由用户选。
// 本机检测只看已安装的浏览器、哪些有窗口在运行、窗口标题里有没有 ChatGPT，
// 不读任何浏览器的数据；标题只是提示（只看得到每个窗口当前的标签页），选择权在用户。
const SYSTEM = ''

function describe(option: BrowserOption): string {
  const notes: string[] = []
  if (option.chatgpt) notes.push('检测到 ChatGPT 窗口')
  else if (option.running) notes.push('正在运行')
  if (option.default) notes.push('系统默认')
  return notes.length ? `${option.name}（${notes.join('，')}）` : option.name
}

export function BrowserControl({ id }: { id: string }) {
  const [view, setView] = useState<BrowserSettingsView | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const load = useCallback((keepError = false) => {
    return settingsService.BrowserSettings().then(next => {
      setView(next)
      if (!keepError) setError('')
    }).catch(reason => setError(`读取浏览器列表失败：${String(reason)}`))
  }, [])
  useEffect(() => { void load() }, [load])
  const save = async (value: string) => {
    if (saving) return
    setSaving(true)
    setError('')
    try {
      setView(await settingsService.SetBrowser(value))
    } catch (reason) {
      setError(String(reason))
      // 保存失败时以实际生效的取值为准，避免界面显示一个并未生效的选择
      await load(true)
    } finally {
      setSaving(false)
    }
  }
  const options = view?.options ?? []
  // 已保存的浏览器本机找不到了：仍列出它，免得下拉框显示成一个并非当前取值的项
  const missing = !!view?.setting && !options.some(option => option.id === view.setting)
  const hint = options.find(option => option.chatgpt && !option.default)
  return <div style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'flex-start' }}>
    <Select id={id} aria-describedby={`${id}-description`} disabled={!view || saving}
      value={view?.setting ?? SYSTEM} onChange={(_, data) => void save(data.value)}>
      <option value={SYSTEM}>系统默认浏览器{view && !view.setting ? `（${view.name}）` : ''}</option>
      {options.map(option => <option key={option.id} value={option.id}>{describe(option)}</option>)}
      {missing && view ? <option value={view.setting}>{view.setting}（未找到）</option> : null}
    </Select>
    {view?.error ? <Text size={200} role="alert" style={{ color: tokens.colorPaletteRedForeground1 }}>{view.error}</Text> : null}
    {!view?.setting && hint ? <Text size={200}>检测到 {hint.name} 里有 ChatGPT 窗口；如果你在那里登录了 ChatGPT，建议选它。</Text> : null}
    {error ? <Text size={200} role="alert" style={{ color: tokens.colorPaletteRedForeground1 }}>{error}</Text> : null}
  </div>
}
