import { cliAgent, type CliAgent } from './agentOptions.ts'

export type Preferences = { defaultAgent: CliAgent; theme: 'system' | 'light' | 'dark'; fontSize: number; spacing: 'comfortable' | 'compact'; readingStart: 'latest' | 'start'; foldProcesses: boolean; readingWidth: 'balanced' | 'wide' }
export const defaultPreferences: Preferences = { defaultAgent:'claude', theme:'system', fontSize:15, spacing:'comfortable', readingStart:'latest', foldProcesses:true, readingWidth:'balanced' }
export const preferencesKey = 'agentdeck.preferences.v1'
export function parsePreferences(raw: string | null): Preferences {
  let value: Record<string, unknown>
  try { const parsed = JSON.parse(raw ?? '{}'); value = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {} } catch { value = {} }
  return {
    defaultAgent: cliAgent(value.defaultAgent),
    theme: value.theme === 'light' || value.theme === 'dark' ? value.theme : 'system',
    fontSize: typeof value.fontSize === 'number' && [13,14,15,16,18].includes(value.fontSize) ? value.fontSize : 15,
    spacing: value.spacing === 'compact' ? 'compact' : 'comfortable',
    readingStart: value.readingStart === 'start' ? 'start' : 'latest',
    foldProcesses: typeof value.foldProcesses === 'boolean' ? value.foldProcesses : true,
    readingWidth: value.readingWidth === 'wide' ? 'wide' : 'balanced',
  }
}
