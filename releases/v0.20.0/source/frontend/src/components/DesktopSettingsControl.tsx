import { Button, Spinner } from '@fluentui/react-components'
import { useEffect, useState, useSyncExternalStore } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import { createDesktopSettings } from '../desktopSettings'
import { matchesSetting } from '../settingsSearch'
import { SettingRow } from './SettingRow'

const startDescription = '登录 Windows 后自动在系统托盘运行，手动打开时显示主窗口。'
const closeDescription = '关闭窗口后继续在后台运行。可通过托盘菜单重新打开或退出应用。'
const matchesStart = (query: string) => matchesSetting(query, '常规 启动与退出 开机启动 自启动', startDescription)
const matchesClose = (query: string) => matchesSetting(query, '常规 启动与退出 关闭时最小化到托盘', closeDescription)
export const matchesDesktopSettings = (query: string) => matchesStart(query) || matchesClose(query)

export function DesktopSettingsControl({ id, query = '' }: { id: string; query?: string }) {
  const [settings] = useState(() => createDesktopSettings(settingsService))
  const { view, operation, error } = useSyncExternalStore(settings.subscribe, settings.getSnapshot)
  useEffect(() => { void settings.load() }, [settings])
  return <div className="ad-desktop-settings" aria-busy={!!operation}>
    <div className="ad-setting-card">
    {matchesStart(query) ? <SettingRow id={id} title="开机启动" description={startDescription}>
      <input id={id} type="checkbox" role="switch" aria-label="开机启动" checked={view?.start_at_login ?? false}
        aria-describedby={`${id}-description`}
        disabled={!!operation || !view?.start_at_login_supported} onChange={event => void settings.save('start_at_login', event.target.checked)} />
    </SettingRow> : null}
    {matchesClose(query) ? <SettingRow id={`${id}-close-to-tray`} title="关闭时最小化到托盘" description={closeDescription}>
      <input id={`${id}-close-to-tray`} type="checkbox" role="switch" aria-label="关闭时最小化到托盘" checked={view?.close_to_tray ?? false}
        aria-describedby={`${id}-close-to-tray-description`}
        disabled={!!operation || !view?.tray_supported} onChange={event => void settings.save('close_to_tray', event.target.checked)} />
    </SettingRow> : null}
    </div>
    <p className="ad-settings-note">两项默认关闭；关闭到托盘未启用时，关闭窗口将退出应用。</p>
    {view && (!view.start_at_login_supported || !view.tray_supported) ? <p className="ad-settings-note">当前环境不支持{!view.start_at_login_supported ? '开机启动' : ''}{!view.start_at_login_supported && !view.tray_supported ? '和' : ''}{!view.tray_supported ? '系统托盘' : ''}，相关选项已停用。</p> : null}
    {operation ? <div className="ad-desktop-settings-status" role="status"><Spinner size="extra-tiny" />{operation === 'loading' ? '正在读取设置…' : '正在保存设置…'}</div> : null}
    {error ? <div className="ad-desktop-settings-error" role="alert">{error}{!view ? <Button size="small" disabled={!!operation} onClick={() => void settings.load()}>重试读取</Button> : null}</div> : null}
  </div>
}
