# AgentDeck

跨 Agent 的历史会话检索与恢复工作台。

统一索引 Claude Code、Claude Desktop、Codex 等多个 AI Coding Agent 的历史会话，
做「找到当时的工作 → 看清做到哪一步 → 在正确工具和目录里继续」。

## 定位

AgentDeck 是**历史检索与导航层**，不是又一个 Agent 客户端。它不重新实现
执行内核、不绕过登录限制、不上传会话。默认只读访问上游存储；手动删除仅在预览确认、
备份成功后修改原始记录。

与 [magpie](https://github.com/yetone/magpie) 互补：magpie 解决「模型怎么选」，
AgentDeck 解决「当时做了什么、怎么接着做」。

## 下载与安装

Windows x64 版本从 [Releases](https://github.com/enpiaohj/agentdeck/releases) 下载：

- `AgentDeck-vX.Y.Z-win-x64.exe`：单文件绿色版，双击运行；需要系统已安装
  Microsoft Edge WebView2 Runtime（Windows 11 默认自带）。
- `AgentDeck-vX.Y.Z-win-x64.zip`：同一 EXE 加 CHANGELOG、`LICENSE`、`NOTICE.md`
  与 `THIRD-PARTY-NOTICES.txt`。
- `SHA256SUMS`：上述产物的校验值。

当前发布产物未做代码签名，Windows SmartScreen 可能提示未知发布者；请用 `SHA256SUMS`
校验后再运行。

## 当前状态

**已发布 v0.9.0 — Windows 桌面工作台**（Wails v3 + React 18 + Fluent UI v9）。
版本变化见 [CHANGELOG.md](CHANGELOG.md)，本机发布快照位于 `releases/v0.9.0/`。
v0.9.0 移除从第三方安装包提取的品牌位图（改用开源图标与通用兜底图标），补齐
第三方许可声明，构建改用 `-trimpath`，不再在产物中内嵌构建机路径。

已实现：

- 会话发现与索引（Claude Code / Claude Desktop / Codex / OpenCode / ZCode 等）
- 会话列表、筛选、原生标题与正文预览
- 侧栏「全部会话」清除全部筛选与搜索关键词，回到完整列表；智能体 / 项目 / 收藏 / 归档默认单选（单击替换整个筛选），Ctrl+单击把该维度并入或移出当前筛选；智能体可 Ctrl+单击多选、Shift+单击按侧栏顺序连选、Ctrl+Shift+单击把一段并入已选（列表取并集，顶部每个智能体一个可移除的标签），列表标题如实写出组合条件
- 当前列表支持标题 / 项目 / 标签 / 导入账号与月份筛选、最近 / 最早活动排序，历史记录按具体年月分组
- Claude 普通聊天历史导入（官方 JSON / ZIP 导出），支持多账号区分、重复导入去重、正文预览和全文检索
- 顶栏直接输入全文搜索，结果与正文并排展示；Ctrl+K 聚焦、方向键选择、Enter 预览、Esc 返回列表
- 中文全文检索（SQLite FTS5 + bigram 预分词）与正文定位
- 思考、工具调用 / 结果、上下文及大段 JSON 默认折叠；长代码按需展开，搜索命中自动展开对应内容
- Markdown 标题、列表、引用、表格和行内代码排版；备注支持 Ctrl+Enter 保存
- 阅读区右侧的提问导航条：每根刻度是一次提问，悬停看摘要、点击跳转，顶部显示「第 k / n 问」并可跳到上一问 / 下一问；可在阅读导航栏收起
- 会话默认定位最新一轮问答，提供开头 / 最新消息及前后翻页；长会话从真实末尾读取，不受开头预览上限遮挡
- 设置使用常规、外观、会话、终端、数据管理、备份与恢复、诊断、更新、快捷键九个分类；支持多关键词搜索、正文预览、本机保存和恢复偏好确认，Ctrl+, 打开，Ctrl+K 返回会话搜索
- 开机启动和关闭时最小化到托盘均默认关闭；启用开机启动后登录 Windows 时在托盘运行，手动启动仍显示主窗口；托盘菜单可打开窗口或退出应用
- 项目维度聚合、收藏 / 标签 / 备注 / 归档（保存在本地标注库）
- 会话列表采用桌面选择：单击单选、Ctrl 点击切换、Shift 连选、Ctrl+Shift 追加连选；Ctrl+A 全选当前筛选结果，Esc 清空选择，表头同一个按钮在有选中项时就是清空；支持方向键与 Home / End
- 会话右键菜单支持打开、继续会话、收藏、归档、管理标签与备注、复制及 Markdown 导出；多选时支持批量整理与导出，Shift+F10 可用键盘打开菜单
- 复制恢复命令或在终端中继续会话；终端 PowerShell 可在设置中选择（自动优先 PowerShell 7+ / Windows PowerShell 5.1 / PowerShell 7+ / 自定义可执行文件），并显示检测到的版本
- Codex 子 Agent 按已确认的主会话归组，默认折叠，可展开查看；会话计数不重复计入子会话，筛选与搜索仍可访问独立子会话
- Token 与成本统计（含逐模型明细）
- 完整数据 ZIP 备份与合并恢复，覆盖已保存会话及全部版本、标注、导入历史与界面偏好；恢复前自动保存当前数据
- 在线更新检查（设置 → 更新）：启动后后台检查一次（同一版本每天最多一次），也可手动检查；发现新版本后下载到数据目录的 `updates/` 并按 `SHA256SUMS` 校验，由用户关闭程序后自行替换——程序不会自动替换正在运行的 exe
- 手动删除会话（右键菜单或信息面板「删除会话…」）：先预览范围与文件清单，确认后备份再删除原始记录与 AgentDeck 保留的全部版本；本次运行期间可撤销，备份保留在磁盘
- 默认持久化成功读取的会话，内容变化保留版本、相同正文去重，不自动清理；上游原记录删除后仍可查看、检索及导出
- 紧凑四栏工作台、两行会话列表、浅色 / 深色主题、底部索引状态栏；默认四栏与会话详情展开，左右面板可独立折叠

会话列表顶部的刷新按钮（或 F5）会重新扫描磁盘并更新列表、项目统计与全文索引。Claude Desktop
读取本地 Code / Cowork 记录（含 MSIX 目录），以及 Desktop 本机缓存的普通聊天与
项目会话（来源显示「Desktop 聊天缓存」）：Desktop 不为云端聊天写本地记录，只有在
Desktop 里打开过的会话才缓存了正文，AgentDeck 只读这份缓存，副本存在导入目录旁的
`claude-desktop-cache/`。没打开过的会话与更早的聊天请使用顶部
「导入」：从各 Claude 账号的「设置 → 隐私 → 导出数据」下载 ZIP，或选择其中的
`conversations.json`。同一账号沿用同一名称；不同账号使用不同名称。导入副本保存
在索引库旁的 `imports/claude/`，原始文件不变，云端不会自动同步。当前导入文本与
思考内容，不包含附件文件本体、图片及部分工具内容，不能通过 CLI 恢复导入会话。
magpie 的网关请求账本也不等同于完整的原生会话文件。

本地会话库默认位于 `%LOCALAPPDATA%\AgentDeck\library\history.db`，与可重建的
`index.db` 分开。启动或刷新后构建索引、打开正文、导出等读取会自动保存可解析内容；
新内容作为版本保留，没有过期时间和自动删除任务。保留库保存解析器读到的完整正文，
不沿用界面预览的截断上限；附件文件本体仍不在范围内。上游已丢失且从未读入的内容
无法补回。源记录消失后的本地副本保持原会话标识、收藏、标签和备注，不宣称可以原生恢复。
正文按 zstd 存放（小于 4 KiB 不压），内容与元数据都没变时不再重写；升级前留下的明文
正文可用「设置 → 数据管理 → 压缩会话库」重压一遍并回收文件空间，原始内容不变、
可重复执行。全文索引的 FTS 表只留一份分词结果（contentless），旧索引在首次打开时
就地重建该表，不重读会话文件。
「设置 → 数据管理」可查看库位置、会话数、版本数及保存错误；「设置 → 备份与恢复」可创建完整数据备份。
备份包含本地会话库全部版本、收藏 / 归档 / 标签 / 备注、导入历史及界面偏好；不包含原生
Agent 目录、账号凭据和附件文件，全文索引可重建。单个备份上限为 20,000 个数据文件、
解压后 4 GiB。恢复前校验格式、文件清单、SHA256 和数据库内容，并在本地会话库目录下的
`data-recovery/` 自动保存当前完整备份。恢复采用合并方式，保留现有正文及版本，同名会话
标注以备份为准，导入文件冲突时拒绝覆盖。每个数据库独立提交事务；异常时可能已补入部分
历史，但不会删除已有版本，界面会显示恢复前备份的位置。恢复后自动重新构建全文索引。

数据位置可在「设置 → 数据管理 → 存储位置」整体迁移到其他磁盘。选择的是「数据
放在哪个目录下」：数据根固定取所选目录下的 `AgentDeck\Data`（`D:\` 得到
`D:\AgentDeck\Data`，`D:\software` 得到 `D:\software\AgentDeck\Data`）；所选目录本身
就叫 `AgentDeck` 时只追加 `Data`，已经选到数据根本身时原样使用；只比末级目录名，不做
前缀匹配。确认之前界面即显示补全后的路径与目标现状——已存在且为空可直接使用，已存在且
非空则拒绝，不存在则迁移时创建。迁移对三个数据库取在线一致快照（`VACUUM INTO`），其余
数据文件逐个复制并校验 SHA-256，全部通过后才写入位置指针，重启后生效；旧位置的数据一律
保留，确认新位置运行正常后再用「清理旧位置数据」显式删除（预览清单绑定内容哈希，内容
变化则整批拒绝）。全文索引可重建，不参与复制，重启后按新位置重建。位置指针保存在
`%APPDATA%\AgentDeck\app-config.json`，该文件始终留在原地，迁移与清理都不会动它。

待实现：

- 生成新摘要、交接到其他 Agent
- 项目 / 文件 / 工具调用独立视图

## 启动与系统托盘

在「设置 → 常规 → 启动与退出」分别设置开机启动和关闭时最小化到托盘，两项默认关闭。
开机启动时主窗口保持隐藏；手动打开程序、单击托盘图标或选择「打开 AgentDeck」会显示窗口。
启用关闭到托盘后，使用托盘菜单「退出」结束应用；未启用时关闭窗口即退出。

开机启动使用当前用户的 Windows 启动注册项，记录启用时的程序绝对路径。建议将正式 EXE
放在固定目录后再启用；移动或更换 EXE 路径后，在新程序中关闭并重新开启开机启动以更新路径。
窗口关闭偏好保存在 `%APPDATA%\AgentDeck\app-config.json`，旧配置缺少新字段时均按关闭处理。

## 架构

```
cmd/agentdeck/       命令行入口
cmd/agentdeck-desktop/ Wails 桌面入口
frontend/           React 桌面界面与生成的服务 bindings
internal/app/       桌面服务层与装配
internal/appconfig/  数据位置与终端设置的引导配置
internal/search/    中文全文索引与检索
internal/marks/     本地收藏、标签、备注与归档
internal/projects/  项目聚合与路径归一化
internal/sessions/   会话发现、解析、列表、恢复（核心）
internal/catalog/    模型目录与定价数据
internal/redact/     脱敏
internal/settings/   设置
internal/desktopdir/ Claude Desktop 目录发现（含 MSIX 打包路径）
internal/claudecache/ Claude Desktop 本机聊天缓存（IndexedDB）只读解析
internal/provider/   最小 provider 替代（只读打开 + 基础定价）
tools/               图标与第三方许可声明生成脚本
```

会话层不依赖任何桌面框架，可独立编译与测试。

## 构建

要求 Go 1.26.8 或更高。桌面构建另需 Node.js / npm、Wails CLI
`wails3`（与 `go.mod` 中的 `v3.0.0-beta.30` 对齐）、Task 和 PowerShell 7
（`pwsh`），运行时需要 WebView2。

命令行与 Go 编译检查：

```sh
go build ./...
go run ./cmd/agentdeck
```

Windows 桌面完整构建（生成 bindings → 构建前端 → 清理并同步资源 → 编译 exe）：

```powershell
task build
```

输出为 `bin/agentdeck-desktop.exe`。已有依赖与 bindings 时，可分步构建：

```powershell
Set-Location frontend
npm run build
Set-Location ..
pwsh -NoProfile -File frontend/scripts/sync-assets.ps1
go build -trimpath -tags production -ldflags "-H windowsgui -s -w" -o bin/agentdeck-desktop.exe ./cmd/agentdeck-desktop
```

资源同步会删除可重新生成的旧 JS / CSS bundle，再复制 `frontend/dist`；
只运行 `go build` 不会刷新前端。`frontend/scripts/rebuild.ps1` 还会重启应用并截图。

验证命令：

前端定向测试：在 `frontend/` 运行 `npm test`（使用 Node.js 22 验证）。

```powershell
gofmt -l cmd internal
go vet ./...
go build ./...
go test ./... -count=1
```

## 数据来源

AgentDeck 默认只读访问各 Agent 的本地存储。唯一例外是**用户显式发起的手动删除**
（见下节「删除会话」）：只有经过预览确认、且备份成功之后才会写入。

| Agent | 存储位置 |
| --- | --- |
| Claude Code CLI | `~/.claude/projects/<编码目录>/*.jsonl` |
| Claude Desktop | `%LOCALAPPDATA%\Claude-Data\claude_<pub>\{local-agent-mode-sessions,claude-code-sessions}\...\.claude\projects\`（3.x；2.x 在 `Packages\Claude_<pub>\LocalCache\Roaming\Claude`）；聊天缓存 `...\IndexedDB\https_claude.ai_0.indexeddb.{leveldb,blob}` |
| Codex | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` + `state_5.sqlite` |
| OpenCode | `~/.local/share/opencode/opencode.db` |
| ZCode | `~/.zcode/cli/db/db.sqlite` |
| WorkBuddy / WorkBuddy AI（国际版） | `~/.workbuddy/projects/`、`~/.workbuddy-ai/projects/` 下的 `*.jsonl` |
| DeepSeek Harness (dsh) | `~/.dsh/sessions/<编码目录>/<会话>/session.v<N>.jsonl[.zstd]` |

## 隐私与网络访问

- 会话正文、索引、标注与偏好全部保存在本机，不上传、不遥测、不需要账号。
- 不读取账号凭据文件（`~/.claude/.credentials.json`、`~/.codex/auth.json` 等），
  也不读写各 Agent 的 Cookie 或登录态。
- 获取模型目录与定价时可能向 `https://models.dev/api.json`、`https://aihubmix.com/api/v1/models`
  发起只读 GET 请求；请求不包含会话内容、本机路径或账号信息，离线时使用内置目录。
- 检查更新时向 `https://api.github.com/repos/enpiaohj/agentdeck/releases/latest` 发起一次只读
  GET 请求，User-Agent 只含产品名与版本号；不发送会话、路径或账号信息。启动后的自动检查
  同一版本每天最多一次，结果缓存在数据目录的 `updates/check.json`；关闭「启动后自动检查」
  后不再发起该请求。下载只在用户点击「下载到本地」时发生。
- 更新包按发布页公布的 `SHA256SUMS` 校验完整性。项目未做代码签名，因此该校验只能说明
  文件与发布清单一致，不能证明发布者身份；替换 exe 由用户手动完成。
- 日志与诊断包经过脱敏，不包含凭据内容。

## 删除会话

删除是**不可自动回滚**的破坏性操作，因此设计成四道闸：

1. **只对明确来源开放**：仅本机原生或永久保留的 Codex、Claude Code、Claude Desktop
   会话。WSL、网关、导入来源一律拒绝并给出原因（在 WSL 里管理、导入副本不可原生恢复）。
2. **预览即契约**：预览会核对真实路径、文件大小与内容哈希，返回一次性 token；
   执行时必须带这个 token，且源文件若在预览后发生变化，整批拒绝并要求重新预览。
3. **先备份再删**：删除前把原始文件与 AgentDeck 保留的全部版本写入
   `%LOCALAPPDATA%\AgentDeck\deletions\delete-*\`，备份失败则一个字节都不删。
   多个来源中途失败时，已删的部分会回滚。
4. **不删目录**：只移除确认过的具体文件与索引条目，绝不 `RemoveAll` 任何目录。

Codex 的标题与子会话关系存在 `~/.codex/state_5.sqlite`，删除时在一个事务内移除相关
行并重写 `session_index.jsonl`；未知的 schema 会直接拒绝，不会猜测着改。该库自带
若干维护型 trigger（仅响应 INSERT / UPDATE，把秒级时间戳同步到 `_ms` 列），它们不会
在 DELETE 时触发，因此被识别放行；其余 trigger、表、列或外键关系一律 fail closed。
撤销仅在本次运行期间提供（撤销 token 保存在内存），磁盘上的备份长期保留。

## 已知限制

- Claude Code 默认 `cleanupPeriodDays=30`，主会话转录可能被上游自动清理；
  AgentDeck 保留成功读取的本地副本，但无法补回首次读取前已删除的记录。
- Claude Desktop 会话无原生恢复入口。
- 当前正式产物为未签名的 Windows x64 单文件 exe，尚无安装包。
- 深浅主题已通过隔离浏览器验证；原生窗口及不同 DPI 尚未完整走查，macOS / Linux 尚未构建和运行验证。
- v0.1.2 发布时记录过偶发测试失败，详见该版 CHANGELOG；后续验证结果不改写历史快照。
- 移植自 magpie 的代码为一次性搬运，不跟随上游更新。

## 许可

MIT。包含移植自 [magpie](https://github.com/yetone/magpie) 的代码
（MIT, Copyright (c) 2026 yetone）与 [Lobe Icons](https://github.com/lobehub/lobe-icons)
的图标（MIT, Copyright (c) 2023 LobeHub）。依赖许可文本见
[NOTICE.md](NOTICE.md) 与 [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt)。
品牌名称与标识归各自权利人所有，本项目与这些产品没有隶属或赞助关系。
