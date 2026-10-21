# Agent 品牌资源来源

本目录只包含 [Lobe Icons](https://github.com/lobehub/lobe-icons) 提供的 SVG，
固定提交 `8a1b2821bb768c484fe8da20c5e1838b6bf551a9`，下载路径为该提交下的
`packages/static-svg/icons/<文件名>`，保留原始 SVG 内容。

图标库的 MIT 许可证保存在同目录 `LOBE-ICONS-LICENSE`。品牌标识的商标归各品牌
所有；这里使用的是开源图标库提供的资源，不宣称本项目自绘，也不宣称品牌方授权
或代言。

| 文件 | 用途 |
| --- | --- |
| claude-color.svg | Claude Code / Claude Desktop |
| codex-color.svg | Codex |
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

ZCode、Hermes、WorkBuddy 没有可再分发的开源图标，`agents.ts` 里这些项为
`icon: null`，由 `AgentMark` 回退到 Fluent 终端图标并保留可访问名称。此前从
各产品安装包提取的 PNG 已全部移除，不再随仓库或安装包分发。

应用只访问随包分发的本地文件，不使用在线图标 CDN。
