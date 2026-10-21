# AgentDeck v0.10.0

发布日期：2026-10-21。平台：Windows x64。版本资源：ProductVersion `0.10.0`，FileVersion / 应用清单 `0.10.0.0`。

本版新增 Claude Desktop 聊天缓存与 WorkBuddy AI 会话来源、阅读区提问导航和在线更新，整理侧栏、多选与品牌图标，并修复 v0.9.0 发布程序中残留旧资源和标题栏版本的问题。

## 新增

- **Claude Desktop 普通聊天与项目会话**：只读解析 Desktop 内嵌 claude.ai 页面的 IndexedDB 缓存，将本机已有正文转成 AgentDeck 自有只读副本。支持文本、思考摘要、工具调用及结果，按当前消息分支读取；会话列表提供标题、模型、账号显示名与工作区信息。来源显示为「Desktop 聊天缓存」，不可原生恢复或删除上游记录。未在 Desktop 中打开、或已从缓存移除的正文仍需使用官方导出导入。
- **缓存解析加固**：解析 LevelDB 日志、表文件及 blob，支持 Snappy 与 V8 序列化的中文 UTF-16LE 内容；校验 CRC32C，按最新键值与删除标记合并；文件被运行中的 Desktop 合并移除时重扫，限制解码深度和数组长度。直接只读扫描文件，不通过 LevelDB 库打开上游数据库。
- **WorkBuddy AI（国际版）**：发现 `~/.workbuddy-ai/projects/`，可用 `WORKBUDDY_AI_CONFIG_DIR` 覆盖；作为独立智能体提供标题、项目、正文与 Token 统计。
- **提问导航条**：阅读区右侧刻度按非空用户正文段的全文分段序号定位，悬停显示序号及首行摘要，点击跳转，当前问高亮。相邻刻度过近时合并，点击跳到组内首问；导航条可收起并记住偏好。顶部显示「第 k / n 问」，支持上一问、下一问；无提问时保留分段导航。后端在读取正文窗口时同时返回全文提问标记。
- **智能体多选**：Ctrl+单击增减选择，Shift+单击按侧栏顺序连选，Ctrl+Shift+单击追加区间；多个智能体取并集，并可通过列表顶部标签单独移除。后端保留原 `Agent` 字段兼容性，新增 `Agents`。
- **上游删除提示**：本机 WorkBuddy / WorkBuddy AI 的数据库将会话标记为删除、但文件仍在时，列表与详情以灰色状态图标提示，悬停显示说明；仅只读查询 `deleted_at`，不修改上游。其他智能体及 WSL 来源暂不检测此状态。
- **在线更新**：设置新增「更新」，从 GitHub 最新正式 Release 查询版本，按需下载到数据目录 `updates/vX.Y.Z/`。校验发布页 `SHA256SUMS` 中的分发包与 EXE，拒绝越界路径及哈希不符的内容。自动检查可关闭，同一运行版本每天最多检查一次；手动检查不受此限制。只下载，不自动替换或启动程序。当前版本从 EXE 的 VERSIONINFO 读取，开发构建显示开发版本，不额外保存 Go 版本常量。
- 新增 `SECURITY.md` 与 Dependabot 周期依赖检查配置。

## 改进与修复

- **Claude Desktop 3.x 新数据目录**：同时发现 `%LOCALAPPDATA%\Claude-Data\claude_<publisher>` 与旧 MSIX / Roaming 目录，修复迁移后 Code / Cowork 会话完全读不到的问题。
- **WorkBuddy 消息统计**：累计实际用户提问与带文字的助手回复；从附加上下文中的 `<user_query>` 提取提问，修复消息数恒为 0、标题遗漏。解析缓存版本升到 23，旧记录自动重读。
- **侧栏单一滚动区**：顶部「全部会话 / 收藏 / 归档」和底部「设置」固定，中间智能体与项目统一滚动；分区可折叠并记住偏好，标题吸顶，项目默认展示前 8 个，当前筛选项目始终可见。
- **列表与详情**：单智能体或项目筛选下仍显示完整的智能体、账号、项目信息；精简分组计数，统一折叠箭头，备注保存状态靠右显示，删除操作使用提示色。
- **刷新入口**：从窗口顶栏移到会话列表头部，新增 F5，并阻止 WebView 默认整页刷新；设置中的快捷键说明同步更新。
- **图标与深色主题**：Claude 为橙色底板与白色星芒；Codex 使用 Lobe Icons 的 OpenAI 标志；WorkBuddy 两版使用 MIT SVG；Hermes 使用 Nous Research 标识；ZCode 使用系统字体自绘 Z。单色图形通过 CSS mask 随主题着色，彩色图形保留可辨底板。开源来源与商标边界见 NOTICE。
- **README 详细重写**：补充来源、提问导航、多选、数据保留、备份迁移、更新、快捷键、隐私、删除边界和构建说明；加入主界面、中文搜索、深色多选、外观设置四张截图，全部使用隔离演示数据。
- 更新第三方图标来源记录与项目测试、截图隔离说明；第三方许可清单重新生成，含 21 个 Go 模块与 191 个前端生产依赖。

## 对 v0.9.0 的更正

- v0.9.0 的 EXE 属性为 0.9.0，但内嵌前端在提版本前构建，标题栏仍为 0.8.0。资源同步现在验证 bundle 中的版本与 `info.json` 一致，不一致拒绝打包。
- v0.9.0 的 CHANGELOG 称已移除 7 张从安装包提取的品牌 PNG，但它们实际仍残留于打包目录并被嵌入 EXE，界面未引用。同步脚本现清空除 `.gitkeep` 外的全部生成内容，复制后核对文件清单与 `frontend/dist` 一致；读取文件显式使用 UTF-8，兼容 Windows PowerShell 5.1。
- v0.9.0 已发布快照和产物保持不变，其 GitHub Release 已补充更正说明。

## 安装与升级

1. 下载 Windows x64 EXE，或包含程序与许可说明的分发 ZIP；用 `SHA256SUMS` 校验下载文件。
2. 退出旧程序后，将新 EXE 放到固定位置并启动。已有 AgentDeck 数据与偏好继续使用；升级前可在「备份与恢复」创建完整备份。
3. 若移动了 EXE 且启用了开机启动，在新程序中关闭并重新启用该选项以更新路径。
4. 首次读取会重建相关解析缓存。Desktop 缓存来源只反映本机已有内容，不会从云端同步或代替官方导出。

## 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...`、`go build ./...` 通过。
- `go test ./... -count=1`：29 个包，其中 27 个含测试，全部通过；本轮 `filememo` 也通过。准备阶段曾出现其 Windows 文件替换时序测试偶发失败，未删除测试或降低断言。
- 前端 `npm test`：62 项通过；`npm run build`（含 TypeScript 编译与 Vite）通过；资源同步通过。
- 前端命令输出 Node.js `EnvHttpProxyAgent` 实验性警告及 Wails 浏览器测试环境提示，未隐藏，未阻止构建。
- 重新生成 Windows syso，确认正式 EXE 的 ProductVersion `0.10.0`、FileVersion `0.10.0.0`；标题栏运行截图显示 `v0.10.0`。
- 正式构建使用 Go 1.27.0、`-trimpath`、`production`、Windows GUI 子系统；`go version -m` 记录 `GOOS=windows`、`GOARCH=amd64`、`CGO_ENABLED=0`、`vcs.modified=false`。
- 正式 EXE 在全新隔离用户目录启动，显示一条虚构会话及正文；点击设置后正常显示设置页面，关闭窗口正常退出（exit 0）。未操作开机启动注册项或真实会话。
- README 四张截图逐张检查，未见真实账号、会话标识或用户路径。

## 已知问题与未执行

- 未做代码签名，Windows SmartScreen 可能显示未知发布者；SHA-256 校验仅保证与发布清单一致。
- Desktop 聊天缓存属于上游内部格式，升级后可能变化；仅支持当前可解析的缓存正文，不包含附件文件本体。
- 提问刻度按分段序号而非文本字数或显示高度分布，合并刻度点击跳到首问。
- `internal/filememo` 的 `TestReadWhileReplaced` 在 Windows 上曾偶发失败，本轮完整测试通过，后续仍应保留失败证据并调查。
- 本轮未执行真实用户数据迁移 / 备份恢复、真实开机登录、多 DPI 完整走查、macOS / Linux 构建与运行验证。
- 本轮正式 EXE 冒烟验证采用点击设置入口；合成 Ctrl+, 未观察到页面切换，未据此宣称该快捷键通过运行验证。

## 产物与 Git

- `AgentDeck-v0.10.0-win-x64.exe`：Windows x64 单文件程序。
- `AgentDeck-v0.10.0-win-x64.zip`：EXE、CHANGELOG、LICENSE、NOTICE、THIRD-PARTY-NOTICES 与内层 EXE 校验值。
- `AgentDeck-v0.10.0-source.zip`：完整源码快照，排除 `releases/`、依赖安装目录和编译缓存。
- `SHA256SUMS`：以上三份产物的 SHA-256。
- 源码冻结及 EXE 构建提交：`a5b3da56013002fa5ed3ab4ff3f07ae10e9b9454`。
- Git Tag：`v0.10.0`，指向仅增加本版发布快照的发布提交；冻结源码位于该 Tag 的 `releases/v0.10.0/source/`。
