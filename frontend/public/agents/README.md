# Agent 品牌资源来源

本目录只包含 [Lobe Icons](https://github.com/lobehub/lobe-icons) 提供的 SVG，
固定提交 `8a1b2821bb768c484fe8da20c5e1838b6bf551a9`，下载路径为该提交下的
`packages/static-svg/icons/<文件名>`，保留原始 SVG 内容。

图标库的 MIT 许可证保存在同目录 `LOBE-ICONS-LICENSE`。品牌标识的商标归各品牌
所有；这里使用的是开源图标库提供的资源，不宣称本项目自绘，也不宣称品牌方授权
或代言。

| 文件 | 用途 |
| --- | --- |
| claude.svg | Claude Code / Claude Desktop（单色，界面着白放在品牌色底板上） |
| openai.svg | Codex（Codex 桌面版已并入 ChatGPT 应用，开始菜单与任务栏显示的是黑色 OpenAI 标志） |
| opencode.svg | OpenCode |
| gemini-color.svg | Gemini CLI |
| grok.svg | Grok |
| kimi-color.svg | Kimi |
| cursor.svg | Cursor |
| minimax-color.svg | MiniMax Code |
| cline.svg | Cline |
| qoder-color.svg | Qoder / Qoder CN |
| codebuddy-color.svg | CodeBuddy |
| deepseek-color.svg | DeepSeek Harness（dsh） |
| workbuddy-color.svg | WorkBuddy / WorkBuddy AI（见下） |
| nousresearch.svg | Hermes（出品方 Nous Research 的标识，产品本身没有开源图标） |

`workbuddy-color.svg` 在上述固定提交之后才收录，单独取自提交
`9046e389bf50067adbb4dd794eba2164cac41197`（同一路径、同一 MIT 许可证），内容未改动。

ZCode 没有可再分发的图标，界面用自绘字母标（深色底板 + 系统字体「Z」），不仿画
其标志。Pi、oh-my-pi、Alma 等没有可再分发的开源图标，`agents.ts` 里这些项为
`icon: null`，由 `AgentMark` 回退到 Fluent 终端图标并保留可访问名称。此前从
各产品安装包提取的 PNG 已全部移除，不再随仓库或安装包分发。

应用只访问随包分发的本地文件，不使用在线图标 CDN。
