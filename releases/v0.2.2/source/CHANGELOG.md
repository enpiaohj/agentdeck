# CHANGELOG

AgentDeck 的版本索引。

- 每个版本的**完整正文**在 `releases/vX.Y.Z/CHANGELOG.md`；本文件只记该版做了什么、
  快照在哪、产物是哪个，不重复正文。
- 版本遵循 Semantic Versioning，Tag 为 `vMAJOR.MINOR.PATCH`，与 `releases/vX.Y.Z/`
  一一对应。
- `releases/` 下是**不可变**发布快照：已发布版本不改写、不覆盖，发现缺陷出下一个
  PATCH（v0.1.2 正是这样修 v0.1.1 的元数据）。已发布快照里写错的话，不回头改它，
  在新版本的 CHANGELOG 里更正并说明。
- 发布产物（exe）不入 Git History（`.gitignore` 的 `releases/**/*.exe`），按规则
  应上传 GitHub Releases；本机尚未配置远端仓库，所以目前没有 GitHub Release，
  分发包就是快照内的 exe。

## v0.2.1 — 2026-10-17

- 修复 Codex 子 Agent 会话被平铺为独立会话的问题：按已确认的根会话关系归组，默认收起子会话，支持展开、键盘导航和搜索定位；主会话计数与侧栏统计不重复计入子会话。
- 子会话继续保留原始记录及独立标注；父会话缺失、被筛选或身份不明确时保持可见，不按项目或标题误合并不同主会话。
- 主会话活动时间包含子会话更新，启动优先选中主会话；修复搜索返回列表时子会话选中状态丢失。
- 完整变更、升级说明及验证记录见 [本版 CHANGELOG](releases/v0.2.1/CHANGELOG.md)。

快照：`releases/v0.2.1/`　产物：`AgentDeck-v0.2.1-win-x64.exe`

## v0.2.0 — 2026-10-17

新增永久会话库、完整数据备份恢复、Claude 聊天历史导入和桌面式多选，重新整理四栏工作台与设置界面。

- 已读取会话永久保存，变化保留版本；原记录不可用时仍可阅读、检索和导出。
- 完整 ZIP 备份覆盖全部已保存版本、标注、导入库与界面偏好，恢复前自动备份并保留现有历史。
- 列表支持 Ctrl / Shift 多选、右键操作与批量导出；全文搜索内联显示，正文可定位最新问答并连续翻页。
- 思考与工具输出默认折叠，完善 Markdown 阅读、分组设置、主题和字号偏好。
- 更新应用与 Agent 图标、窗口标题版本显示，并修复标题栏双击最大化 / 还原。
- 修复资源同步、测试隔离、刷新代次与 Windows 短暂文件缺失重试。
- 完整变更、限制、升级说明及验证记录见 [本版 CHANGELOG](releases/v0.2.0/CHANGELOG.md)。

快照：`releases/v0.2.0/`　产物：`AgentDeck-v0.2.0-win-x64.exe`

## v0.1.2 — 2026-10-17

修正版（无新功能）：把版本号在 exe 里剩下的落点补齐，补上仓库根 CHANGELOG。

- **修正 v0.1.1 遗留的版本残留**：`build/windows/wails.exe.manifest` 的
  `assemblyIdentity version` 还写着 `0.1.0`，v0.1.1 只提了 `info.json`，于是
  重生成的 syso 把旧清单嵌进 exe——exe 里始终躺着一个 `0.1.0`。
- **补 `fixed.product_version`**：v0.1.0/v0.1.1 的固定信息块里 dwProductVersion
  是 `0.0.0.0`，与字符串 `ProductVersion` 矛盾（wails 上游模板同样不写这项）。
- **补字符串表 `FileVersion` 键**：此前版本资源里没有这个键，资源管理器只能回退
  到固定信息块来显示「文件版本」。
- **更正 v0.1.1 CHANGELOG 的三处说法**（原文不可改，在此更正，详见本版 CHANGELOG）：
  `TestVarsAreRead` 的解释、「不含 0.1.0 残留」、「Token 统计」置灰项。
- 新增仓库根 `CHANGELOG.md`（本文件）；`CLAUDE.md` 增补「版本号散在两个文件的
  五处」陷阱，并更正过期的测试基线（18 → 21 个包）。

快照：`releases/v0.1.2/`　产物：`AgentDeck-v0.1.2-win-x64.exe`

## v0.1.1 — 2026-10-17

补丁版本：换应用图标，修 v0.1.0 发布时才引入的测试回归。

- 应用图标重新设计为「会话列表：工作行 + 四角星」（16~32px 档更可读），
  生成器 `tools/gen_appicon.ps1` 参数化（`-Design rows|deck|panes`）
- **修**：仓库级约束测试扫到 `releases/` 快照副本而变红，三条测试各自跳过它
- **修**：快照导出排除 `releases/`，否则把上一版快照整个套进来（1216 → 608 个文件）
- UI 自检脚本改为按进程名通配 `agentdeck*` 找窗口

快照：`releases/v0.1.1/`　产物：`AgentDeck-v0.1.1-win-x64.exe`

## v0.1.0 — 2026-10-17

首个发布版本：把本机散在各家 Agent 里的历史会话收进一个工作台。

- 会话层（移植自 magpie，MIT）：覆盖本机 12 类来源的会话发现与只读访问
- 中文全文检索：FTS5 + bigram 预分词，2 字中文词可命中
- 桌面壳（Wails v3 + React 18 + Fluent UI v9）：四栏布局、Ctrl+K 全局搜索、
  正文分色与搜索定位、筛选与项目聚合、本地标注库（收藏/标签/备注/归档）、
  复制或在终端执行恢复命令、诊断与标注库备份恢复

快照：`releases/v0.1.0/`　产物：`AgentDeck-v0.1.0-win-x64.exe`
