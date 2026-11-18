// 与 UI 分离的恢复流程，确保取消时不执行动作。
export async function executeLocation(action: 'save' | 'resume' | 'copy', original: string, directory: string, remember: boolean, services: {
  save: (original: string, directory: string) => Promise<void>
  saved: (directory: string) => void
  open: (directory: string) => Promise<{ warning: string }>
  copy: (directory: string) => Promise<void>
}): Promise<string> {
  if (action === 'save' || (remember && original)) {
    await services.save(original, directory)
    services.saved(directory)
  }
  if (action === 'resume') return (await services.open(directory)).warning
  if (action === 'copy') await services.copy(directory)
  return ''
}

export type LocationPreview = { original: string; directory: string; needsLocation: boolean; command?: string; notice?: string }
export async function performResume(key: string, action: 'resume' | 'copy', services: {
  prepare: (key: string) => Promise<LocationPreview>
  choose: (preview: LocationPreview) => Promise<boolean>
  open: (key: string) => Promise<void>
  copy: (key: string) => Promise<void>
}): Promise<boolean> {
  const preview = await services.prepare(key)
  if (preview.needsLocation && !(action === 'copy' && preview.command)) return services.choose(preview)
  if (action === 'resume') await services.open(key)
  else await services.copy(key)
  return true
}
