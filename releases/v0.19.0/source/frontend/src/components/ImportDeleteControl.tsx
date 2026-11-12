import { Text, tokens } from '@fluentui/react-components'
import { useEffect, useState } from 'react'
import * as settingsService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'
import { setImportDeleteEnabled } from '../sessionDeletion'

// 「允许删除导入的聊天记录」开关。取值由 Go 侧持有（appconfig 的 allow_import_delete）：
// 后端在预览和执行删除时都会检查它，界面置灰只是提示。默认关闭。
export function ImportDeleteControl({ id }: { id: string }) {
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    void settingsService.ImportDeleteEnabled().then(value => { setEnabled(value); setImportDeleteEnabled(value) })
      .catch(reason => setError(`读取设置失败：${String(reason)}`))
  }, [])
  const change = async (value: boolean) => {
    if (saving) return
    setSaving(true); setError('')
    try {
      const saved = await settingsService.SetImportDeleteEnabled(value)
      setEnabled(saved); setImportDeleteEnabled(saved)
    } catch (reason) {
      setError(String(reason))
    } finally { setSaving(false) }
  }
  return <div style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'flex-end' }}>
    <input id={id} aria-describedby={`${id}-description`} type="checkbox" role="switch" checked={enabled === true} disabled={enabled === null || saving}
      onChange={event => void change(event.target.checked)} />
    {error ? <Text size={200} role="alert" style={{ color: tokens.colorPaletteRedForeground1 }}>{error}</Text> : null}
  </div>
}
