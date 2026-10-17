# AgentDeck v0.2.2

发布日期：2026-10-18。平台：Windows x64。此修复版本解决 Codex 会话无法删除的问题，并修正删除预览中「将删除文件」的统计口径。

## 修复与改进

- **修复 Codex 会话完全无法删除**。`~/.codex/state_5.sqlite` 自带 5 个维护 trigger，分别把 `created_at` / `updated_at` / `recency_at` 的秒级值同步到对应的 `_ms` 列；它们只响应 `INSERT` 与 `UPDATE OF <列>`，`DELETE` 时不会触发。原守卫一律拒绝任何 trigger，导致所有 Codex 会话在预览阶段即报「Codex 状态库含未知 trigger，拒绝删除」。现按 DDL 识别：触发事件必须是 `INSERT` 或 `UPDATE`、全文不含 `DELETE`、单个 `BEGIN…END`、监听与写入的表都在已知白名单内，全部满足才放行。
- **未知 schema 仍然 fail closed**。新增 trigger、表、列、外键关系，或 trigger 监听未知表、写入未知表、无 `WHEN` 子句、包含多条语句时，一律拒绝删除并在删除任何内容之前返回。DDL 全文继续参与预览与执行时的比对，「结构变了就要求重新预览」的语义不变。
- **删除预览区分「将删除文件」与「改写元数据」**。原先 `state_5.sqlite`、`session_index.jsonl`、`history.jsonl` 都被计入「涉及原始文件」及其总大小。这些文件实际只删行或原地重写，文件本身保留：选中一个 Codex 会话会显示「涉及原始文件 2 个、12.27 MB」，其中绝大部分是整个状态库的体积，与真实删除量不符。现在只有真正会被 unlink 的路径计入「将删除文件」与总大小，其余单列为「改写元数据」。执行前的重新校验同时比对这两份清单。
- **修复删除确认弹窗打开时说明文字被滚走**。弹窗首个复选框会自动获得焦点，把标题下方的两段说明滚出视口，标题下只剩残句。焦点改为落在滚动容器上（`preventScroll`），打开即可完整阅读说明。
- 新增被动截图与点击脚本 `frontend/scripts/uishot-passive.py`、`uiclick-passive.py`：原有 `uishot.py` 会点击标题栏以收起输入法浮条，而该点击会连带关闭正在检查的弹窗。

## 升级

关闭旧程序，运行 `AgentDeck-v0.2.2-win-x64.exe`。沿用现有本地数据目录，无数据库迁移或数据清理；建议升级前在「设置」中创建完整备份。

本次只修正删除路径的守卫与预览统计，不改变会话发现、索引、保留库、导入与标注的任何行为。已有保留版本、收藏、标签、备注、归档与备份格式均不变。程序需要 Windows WebView2 Runtime。

## 验证

- `gofmt -l cmd internal` 输出为空；`go vet ./...`、`go build ./...` 通过。
- `go test ./... -count=1` 全部 23 个包通过（`sessions` 约 22s，`settings` 约 22s）。
- 新增 6 条针对性用例：真实 5 个维护 trigger 必须放行且删除、撤销均成功；`DELETE` 型、无 `WHEN`、写入未知表、监听未知表、多语句 5 类 trigger 必须继续拒绝且不触碰源文件。
- 新增预览口径断言：Codex 批量删除时 `native_files` 为 2、`metadata_files` 为 3（状态库 + `session_index.jsonl` + `history.jsonl`）。
- 对本机真实 `state_5.sqlite` 做只读探测：守卫放行，`threads` 表正确取到目标行；对真实库的临时副本执行完整「准备 → 备份 → 删除 → 撤销」流程通过，确认 `state_5.sqlite` 文件本身保留、仅删除对应行。
- 真机界面确认：删除预览弹窗可正常打开，显示「将删除文件 1 个 / 改写元数据 3 个 / 44.95 MB」，弹窗顶部说明完整显示。
- 正式构建使用 `go build -tags production -ldflags "-H windowsgui -s -w"`；FileVersion 为 `0.2.2.0`，ProductVersion 为 `0.2.2`，嵌入 manifest 为 `0.2.2.0`，运行中窗口标题栏显示 `v0.2.2`。
- `go version -m` 确认 `GOOS=windows`、`GOARCH=amd64`、`vcs.modified=false`，`vcs.revision` 为下方冻结提交。
- 快照 678 个文件逐项比对 Git blob SHA，与冻结提交逐字节一致；不包含嵌套 `releases/`、依赖安装目录、构建输出、缓存或临时数据。敏感文件扫描（`.env` / `*.pem` / `*.key` / 凭据 / `auth.json` / 证书）无命中。
- 工具链：Go windows/amd64，Wails 3.0.0-beta.30，Node 22.23.1。

## 已知限制与未执行项

- 维护 trigger 的识别基于 DDL 文本结构，不是语义解析。若上游 Codex 改变 trigger 写法（例如改用 `INSTEAD OF` 或拆分多语句），会被判定为未知并拒绝删除——这是有意的保守取舍，届时需人工评估后再放行。
- 其他现有限制见 [v0.2.1 更新说明](../v0.2.1/CHANGELOG.md)。
- 本版本仍为未签名的独立 EXE，无安装包。
- 未执行：对真实会话点击「备份并删除」的完整破坏性验证（仅在真实库的临时副本上完成等价流程）、原生桌面自动化运行、不同 DPI 的逐项走查、macOS / Linux 构建与运行、代码签名。
- 本机未配置 Git 远端，Push 与 GitHub Release 未执行。

## 产物与追溯

- 产物：`AgentDeck-v0.2.2-win-x64.exe`，17,946,112 字节。
- SHA256：`43e311588a565a9f204d764542ccf1dec47902a2a37864e7b20f825d3d5bf7ab`，同时见 `SHA256SUMS`。
- 源码冻结提交：`33f6ef856973468db9059f9d0f1619a1ff3f05ad`；二进制与 `source/` 均来自该提交。
- 发布提交：`release: AgentDeck v0.2.2`；Tag：`v0.2.2`，指向包含本快照的发布提交。发布提交仅增加快照与说明，不改变构建源码。
- 本地不可变快照：`releases/v0.2.2/`。
