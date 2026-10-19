# AgentDeck v0.4.0

发布日期：2026-10-19。平台：Windows x64。文件版本：0.4.0.0。

## Added

- 会话重命名：列表右键和详情页提供改名入口，标题保存在 AgentDeck 的 marks.db；可清除覆盖以恢复来源标题。标题最多 200 字，拒绝控制字符。
- 新建项目：创建目录并保存到 AgentDeck 自己的 projects.db，尚无会话的项目也可出现在侧栏。支持系统目录选择、项目显示名修改与取消声明。
- 项目右键「在此新建会话」：在对应目录启动 agent CLI，不带 resume 参数，由 CLI 创建会话。复用 UTF-16LE base64 的 PowerShell -EncodedCommand 终端链。
- 恢复已改名会话时，Claude Code 通过 --name 传递标题；符合来源条件的 Codex 会话通过其 app-server 的 thread/name/set 同步名称。此操作会经 CLI 写入上游会话名称；AgentDeck 本地改名本身只写 marks.db。

## Changed / Fixed

- 列表、详情、搜索结果、删除预览及 Markdown 导出统一使用本地显示标题。
- marks.db 从 schema v1 在事务内增列升级至 v2，备份校验及删除恢复支持标题字段与旧版布局。自动迁移没有额外自动备份步骤，升级前可使用现有备份功能保留数据。
- 声明项目与会话聚合按路径合并，保留聚合统计与活动顺序。
- Codex 名称同步设置超时，失败时仍打开终端并报告名称同步错误。

## 安装与升级

关闭旧程序，解压 AgentDeck-v0.4.0-win-x64.zip 后运行其中的 exe，或直接运行单独提供的 exe。沿用现有本地数据目录，首次打开自动升级 marks.db，并创建项目声明库。取消项目声明只移除列表记录，不删除目录或会话。

## 验证

- go vet ./...、go build ./...、gofmt -l cmd internal：已执行，无报告错误，格式检查为空。
- go test ./... -count=1：首次通过；冻结提交后的复验曾有一次 internal/sessions 失败，未保留完整失败用例输出，根因未确定。随后该包单独重跑、-count=3 及两次完整测试均通过。不能据此断定首次失败与本次变更无关。
- 前端 TypeScript 检查通过；npm test：30/30 通过；生产前端及 Windows GUI 构建完成。
- 版本资源已验证：ProductVersion 0.4.0，FileVersion 0.4.0.0，应用清单 0.4.0.0。
- 当前会话已在旧验收构建中实际点击验证取消声明；v0.4.0 最终产物的完整交互验收未执行。
- 未执行：新项目至新会话的完整真机流程、最终版本 Codex 名称同步结果复核、macOS/Linux 验证、代码签名及安装包制作。

## Known Issues

- 新建项目时选择平台及设置默认平台尚未实现。目前新会话使用当前 agent 筛选，空筛选时按本机会话数选择。
- 修改项目显示名不会重命名磁盘目录。
- 手动输入路径截短问题的根因尚未充分确认；本版增加「浏览…」提供系统目录选择入口，不宣称已修复所有输入问题。
- Codex thread/name/set 为实验接口，可能随 CLI 版本变动。

## 产物与追溯

- 源码冻结提交：d430d08855f7e7e5bafa10c8f894910ff485c72a。
- 发布快照：releases/v0.4.0/；source/ 包含冻结提交中排除 releases/ 后的 692 个文件。
- 产物：AgentDeck-v0.4.0-win-x64.exe、AgentDeck-v0.4.0-win-x64.zip、AgentDeck-v0.4.0-source.zip。
- SHA256SUMS 记录三个产物的校验和；分发 ZIP 内单独提供 exe 校验和。
- Tag：v0.4.0，指向发布提交。二进制与 ZIP 不进入 Git History。
