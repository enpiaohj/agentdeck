// AppMark —— AgentDeck 自身的应用标志。
//
// 用的是 build/appicon.png 的 256px 渲染(frontend/public/appicon-256.png),
// 与 exe 图标同源同版:图标由 tools/gen_appicon.ps1 画出并同时落盘到
// build/appicon.png、build/windows/icon.ico 与这里,不会再出现「界面里的
// 标志和任务栏图标是两个东西」的情况。
type Props = { size?: number }

export function AppMark({ size = 34 }: Props) {
  return (
    <img
      src="/appicon-256.png"
      alt="AgentDeck"
      width={size}
      height={size}
      style={{
        width: size,
        height: size,
        display: 'block',
        flexShrink: 0,
        userSelect: 'none',
      }}
    />
  )
}
