export const canReusePrompt = (textOffset: number | undefined) => textOffset === 0
export const readingRevealPart = (anchor: number | undefined, focus: number) => anchor ?? focus

export type ReadingPosition = { at: number; before: boolean; scroll: number; anchor?: number; offset?: number; textOffset?: number }

// 同一窗口的内容更新恢复阅读锚点；显式导航产生的新窗口有更高优先级。
export function refreshPosition(previousVersion: number, version: number, sameDestination: boolean, position: ReadingPosition): ReadingPosition | null {
  return previousVersion !== version && sameDestination ? { ...position } : null
}
