// 阅读区右侧的提问导航条：每次提问一根刻度，位置按它在全文中的段序号成比例，
// 回答越长、两问相隔越远，一眼看出哪里讨论得多。
export type PromptMark = { index: number; text: string }
/** 一根刻度；靠得太近的相邻提问并成一根，first / last 是其中首末提问的序号（从 0 起）。 */
export type RailTick = { top: number; first: number; last: number; index: number; text: string }

/** 相邻刻度的最小间距（像素）；更近的并入同一根，免得几百问挤成一片看不清也点不中。 */
export const railMinGap = 4

export function railTicks(prompts: readonly PromptMark[], total: number, height: number, minGap = railMinGap): RailTick[] {
  const ticks: RailTick[] = []
  if (!prompts.length || height <= 0) return ticks
  const span = Math.max(1, total - 1)
  prompts.forEach((p, ordinal) => {
    const top = Math.min(height, Math.max(0, (p.index / span) * height))
    const last = ticks.at(-1)
    if (last && top - last.top < minGap) {
      last.last = ordinal
      return
    }
    ticks.push({ top, first: ordinal, last: ordinal, index: p.index, text: p.text })
  })
  return ticks
}

/** 正在读的是第几问（从 0 起）：视口顶部那一段之前、最近的一次提问；在第一问之前为 -1。 */
export function currentPrompt(prompts: readonly PromptMark[], part: number): number {
  let lo = 0, hi = prompts.length - 1, found = -1
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (prompts[mid].index <= part) { found = mid; lo = mid + 1 } else hi = mid - 1
  }
  return found
}

/** 刻度的悬停说明：单问显示序号与摘要，合并的刻度显示序号范围与首问摘要。 */
export function tickLabel(tick: RailTick, count: number): string {
  const head = tick.first === tick.last ? `第 ${tick.first + 1} / ${count} 问` : `第 ${tick.first + 1}–${tick.last + 1} 问（共 ${count} 问）`
  return `${head}\n${tick.text}`
}
