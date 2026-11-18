import { useEffect, useState } from 'react'
import { usePreferences } from './usePreferences'

// 跟随系统深浅色:matchMedia 监听,WebView2 的 prefers-color-scheme 随
// Windows 主题走。默认深色(工作台常驻深色环境)。
export function useTheme(): 'light' | 'dark' {
  const { preferences } = usePreferences()
  const [dark, setDark] = useState(() => {
    try {
      return !window.matchMedia('(prefers-color-scheme: light)').matches
    } catch {
      return true
    }
  })
  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: light)')
    const onChange = (e: MediaQueryListEvent) => setDark(!e.matches)
    mq.addEventListener('change', onChange)
    setDark(!mq.matches)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  return preferences.theme === 'system' ? (dark ? 'dark' : 'light') : preferences.theme
}
