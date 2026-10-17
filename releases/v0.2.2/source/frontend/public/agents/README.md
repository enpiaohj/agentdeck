# Agent 品牌资源来源

新增 SVG 来自 [Lobe Icons](https://github.com/lobehub/lobe-icons)，固定提交：
`8a1b2821bb768c484fe8da20c5e1838b6bf551a9`。

下载路径为该提交下的 `packages/static-svg/icons/<文件名>`，保留原始 SVG 内容。
仓库 MIT 许可证保存在同目录 `LOBE-ICONS-LICENSE`，品牌标识的商标归各品牌所有。
这些是开源图标库提供的品牌资源，不宣称是本项目自绘或品牌方授权代言。

| 文件 | 用途 |
| --- | --- |
| claude-color.svg | Claude Code / Claude Desktop |
| codex-color.svg | 上一轮候选，已停用；不作为当前 Codex 标志 |
| opencode.svg | OpenCode |
| gemini-color.svg | Gemini CLI |
| grok.svg | Grok |
| kimi-color.svg | Kimi |
| cursor.svg | Cursor |
| minimax-color.svg | MiniMax Code |
| cline.svg | Cline |
| qoder-color.svg | Qoder / Qoder CN |
| codebuddy-color.svg | CodeBuddy |

当前 Codex 使用 `codex.png`：来源为本机较新包 `OpenAI.Codex_26.1013.3656.0_x64__2p2nqsd0c76g0`
的 `assets/Square44x44Logo.targetsize-256_altform-unplated.png`，白色 OpenAI 结形标志；
按 `AppxManifest.xml` 的 `BackgroundColor="#3143FF"` 合成底色，不使用 Electron 默认图标。
原始标志 SHA256 见同目录 `codex-source.json`。此处记录的是本机核对版本，不声称为全球最新发行版。

其他 PNG 来自本机已安装产品，提取方式见 `tools/gen_agenticons.ps1`。
应用只访问随包分发的本地文件，不使用在线图标 CDN。
