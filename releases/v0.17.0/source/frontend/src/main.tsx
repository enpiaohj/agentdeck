// React 18 createRoot 入口。
import { createRoot } from 'react-dom/client'
import { App } from './App'
import './index.css'
import { PreferencesProvider } from './hooks/usePreferences'

createRoot(document.getElementById('root')!).render(<PreferencesProvider><App /></PreferencesProvider>)
