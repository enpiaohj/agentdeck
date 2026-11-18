# AgentDeck 图标体系

## 应用图标

新标志采用蓝色折叠 A 形浮雕，结合玻璃边缘、金属高光与深蓝底座，背后的标签代表历史上下文。
图标由内置 imagegen 生成；透明母版为 `build/appicon-source.png`，生成提示词与来源
记录在 `build/appicon-source.json`。运行以下命令统一生成界面与 Windows 图标：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tools/gen_appicon.ps1
wails3 generate syso -icon build/windows/icon.ico -info build/windows/info.json `
  -manifest build/windows/wails.exe.manifest -arch amd64 `
  -out cmd/agentdeck-desktop/resource_windows_amd64.syso
```

输出为 `build/appicon.png`（1024px）、`frontend/public/appicon-256.png`，以及
`build/windows/icon.ico`（256/128/64/48/32/24/16px）。界面和 exe 使用同一母版。
可用 `-Sheet _probe/icon-review` 检查各尺寸，`-Out _probe/icon-candidate` 不会改写界面图标。
显式 `-Design rows|deck|panes` 仅用于重现旧候选方案；默认使用新母版。

## Agent 标识

`agents.ts` 是展示名与资源路径的唯一数据源，`AgentMark` 统一轮廓、尺寸和留白：

- 新增或替换的 SVG 来自固定版本的 Lobe Icons，文件与许可证随应用分发，运行时无需联网。
- SVG 使用白底和适量内边距，深色模式下也能辨识黑色品牌符号。
- 图片使用 `object-fit: contain`，避免裁切标志；故障状态按图片路径记录，切换 Agent 后可以恢复。
- 容器统一细内边缘与轻微投影；品牌原始图形不重画，兜底底色跟随 Fluent 深浅色主题。
- 没有确认品牌图标的产品，以及加载失败的资源，使用 Fluent 终端图标兜底并保留可访问名称。
  Pi Coding Agent 与 Inflection Pi 不是同一产品，不借用同名标志。

品牌 SVG 的来源、固定提交与文件映射见 `frontend/public/agents/README.md`。
Claude 按官方应用图标的形态呈现：品牌色满幅底板 + Lobe Icons 单色图形（CSS mask 着白，不改形）。
Codex 桌面版已并入 ChatGPT 应用，与用户在开始菜单、任务栏看到的一致：白底黑色 OpenAI 标志。
ZCode 没有可再分发的图标，使用自绘字母标；Hermes 使用出品方 Nous Research 在 Lobe Icons 中的标识；Pi、Alma 等仍无可再分发的开源图标，使用通用兜底图标；此前从各产品
安装包提取的 PNG 已移除。

新增 Agent 时登记 key / label / color / icon；使用透明品牌 SVG 时设置 `glyph: true`。

## 界面图标

导航、按钮、收藏和索引状态均使用现有 `@fluentui/react-icons`。
不以星号、圆点、emoji 或首字母代替图标，不手绘近似品牌标志。

Kimi 的固定 SVG 使用白色字形，界面为其配上深色底以保证浅色和深色主题下均可辨识；原始 SVG 保持不变。
