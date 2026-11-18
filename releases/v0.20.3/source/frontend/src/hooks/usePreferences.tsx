import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'
import { defaultPreferences, parsePreferences, preferencesKey, type Preferences } from '../preferences'

type Value = { preferences: Preferences; update: (patch: Partial<Preferences>) => void; reset: () => void; error: string }
const Context = createContext<Value | null>(null)

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState(() => {
    try { return parsePreferences(localStorage.getItem(preferencesKey)) } catch { return { ...defaultPreferences } }
  })
  const current = useRef(preferences)
  const [error,setError] = useState('')
  const save = useCallback((value: Preferences) => {
    try { localStorage.setItem(preferencesKey,JSON.stringify(value));setError('') }
    catch { setError('设置已在本次窗口生效，但无法保存到本机。') }
  },[])
  const update = useCallback((patch: Partial<Preferences>) => {
    const next = parsePreferences(JSON.stringify({ ...current.current, ...patch }))
    current.current = next
    setPreferences(next);save(next)
  },[save])
  const reset = useCallback(() => { const next = { ...defaultPreferences };current.current = next;setPreferences(next);save(next) },[save])
  return <Context.Provider value={{preferences,update,reset,error}}>{children}</Context.Provider>
}

export function usePreferences() {
  const value = useContext(Context)
  if (!value) throw new Error('PreferencesProvider missing')
  return value
}
