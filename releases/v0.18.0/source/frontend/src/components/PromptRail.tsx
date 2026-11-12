// 提问导航条：阅读区右侧的一列短刻度，每根是一次提问。悬停看摘要，点击跳到那一问，
// 当前读到的那一问加长并用主题色。刻度不进 Tab 顺序（可能有几百根），键盘用顶部的
// 「上一问 / 下一问」。
import { useEffect, useRef, useState } from 'react'
import { railTicks, tickLabel, type PromptMark } from '../promptRail'

type Props = { prompts: readonly PromptMark[]; total: number; current: number; onJump: (index: number) => void }

export function PromptRail({ prompts, total, current, onJump }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState(0)
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const observer = new ResizeObserver(entries => setHeight(entries[0]?.contentRect.height ?? 0))
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  const ticks = railTicks(prompts, total, height)
  return (
    <div ref={ref} className="ad-prompt-rail" role="navigation" aria-label={`提问导航，共 ${prompts.length} 问`}>
      {ticks.map(tick => {
        const active = current >= tick.first && current <= tick.last
        return (
          <button
            key={tick.first}
            type="button"
            tabIndex={-1}
            className={active ? 'ad-prompt-tick ad-prompt-tick-active' : 'ad-prompt-tick'}
            style={{ top: tick.top }}
            title={tickLabel(tick, prompts.length)}
            aria-label={tickLabel(tick, prompts.length).replace('\n', '：')}
            aria-current={active ? 'location' : undefined}
            onClick={() => onJump(tick.index)}
          />
        )
      })}
    </div>
  )
}
