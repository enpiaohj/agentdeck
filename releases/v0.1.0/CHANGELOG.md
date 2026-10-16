# AgentDeck v0.1.0

发布日期：2026-10-17

首个发布版本。把本机散落在各家 Agent 里的历史会话收进一个工作台:统一发现、
中文全文检索、逐条查看正文、按来源筛选、随手标注,并在需要时回到原始工具继续。

## Added

### 会话层(移植自 magpie,MIT)

- 覆盖本机 10 类来源:`claude`、`claude-desktop`、`codex`、`qoder`、`qoder-cn`、
  `cursor`、`opencode`、`zcode`、`kimi`、`workbuddy`、`hermes`、`pi`/`omp`
  (本机实测读出 662 条会话)
- 各来源的存储差异按实测分别处理:Claude Desktop 是 MSIX 包,会话在包内
  `LocalCache\Roaming\Claude`;Codex Desktop 与 CLI 共用 `~/.codex`;
  ZCode 是 OpenCode schema 但 message id 存在列里;WorkBuddy 是 OpenAI Agents
  SDK 的 items 文件;Hermes 是 `state.db` 里的一行
- 只读访问上游存储:全程不 `DELETE` / `UPDATE` / `INSERT` 上游数据库

### 中文全文检索

- SQLite FTS5 + bigram 预分词,索引侧与查询侧共用同一分词(`internal/search`)
- 2 字中文词可命中(`unicode61` 与 `trigram` 对中文 2 字词均失效,故自建分词)
- CLI:`agentdeck -search "查询"`(可配 `-agent` / `-all`)

### 桌面壳(Wails v3 + React 18 + TypeScript + Vite + Fluent UI v9)

- 四栏布局:导航 ｜ 会话列表 ｜ 消息流 ｜ 会话信息,列表按日分组、卡片式行
- Ctrl+K 全局搜索:索引进度、结果列表、命中片段高亮、点击定位到具体消息段
- 会话详情:按角色/类型分色的消息段、思考过程折叠、工具调用与输出、复制、Markdown 导出
- 会话信息面板:创建/更新时间、消息数量、Token 消耗(约数)、项目、工作目录、源路径、恢复能力
- 筛选:按 Agent、按项目、收藏、归档
- 本地标注库(`marks.db`,用户数据目录,schema 用 `PRAGMA user_version`):
  收藏、标签、备注、归档 —— 归档只过滤隐藏列表,不触碰任何源文件,可撤销
- 项目聚合视图:路径归一化(剥离 `\\?\` 前缀、`filepath.Clean`、比较时大小写折叠),
  盘符 → 项目两级
- 恢复动作:复制恢复命令 / 在终端打开(`wt.exe -d <cwd>` 回退 `powershell`),
  不可恢复的会话在列表与详情都标注「仅可查看历史」
- 诊断与备份恢复:索引状态、`marks.db` 备份/恢复、上次非正常退出后的自动恢复提示
- 图标体系:自绘应用标志(四栏工作台 + 四角星,每个尺寸单独绘制)+ 各 Agent
  真实品牌图标(从本机已安装产品中提取),见 `frontend/src/icons/README.md`

## Changed

- 桌面产物改为 GUI 子系统构建(`-H windowsgui`),双击启动不带控制台窗口
- 检索索引默认落盘 `%LOCALAPPDATA%\AgentDeck`
- 列表表头显示 Agent 中文名,不再显示原始 key

## Fixed

- **四类 Agent 的会话可见但正文打不开**(claude-desktop 22 条、hermes 99 条、
  workbuddy 17 条、zcode 44 条,共 182 条):
  - ZCode:message 的 id 存在同名列而非文档内,而 part 按 `message_id` 查,
    读不到 id 时每次 `parts("")` 都静默返回 0 段;
  - Hermes:会话是 `state.db` 里的一行,路径记作 `<db>#<sid>`,补专门读法,
    正文与列表共用同一套可见性判断(active / 压缩 / display_metadata);
  - WorkBuddy:用户原话被包在 `<system-reminder>…<user_query>` 里,只取该段;
  - Claude Desktop:Desktop 的 Code/Cowork 会话就是 Claude Code 的 jsonl,
    走默认分支并补测试固定。
- 检索层 6 项复查确认的缺陷
- 搜索命中定位到具体消息段(TC-03)

## Known Issues

- **OpenCode 没有官方品牌图标**:本机只装了它的更新器安装包,没有产品图标资源,
  界面退化为识别色方片 + 首字母。不拿相近图形冒充品牌标志,也不拿安装器通用图标顶替。
- **Codex 有 3 条会话本身没有可读正文**(其文件内就没有可解析的消息),与本次改动无关。
- 「交接」「生成摘要」「Token 统计」是 v1.1 规划,界面中置灰并标注。
- 深色模式未逐项走查。
- 未做代码签名,未提供安装包(单个裸 exe)。
- 本机尚未配置远端仓库,没有 GitHub Release;分发包即本快照内的 exe。

## Verification

- Build:`go build ./...` 通过;前端 `npm run build` 通过(vite 2249 modules,
  `dist/assets/index-5V4tl8eB.js` 443.82 kB → gzip 133.73 kB);
  发布构建 `go build -tags production -ldflags "-H windowsgui -s -w" -o bin/agentdeck-desktop.exe ./cmd/agentdeck-desktop`
- Tests:`go vet ./...` 干净;`go test ./... -count=1` 21 个含测试的包全部通过;
  `gofmt -l cmd internal` 输出为空
- 运行验证:发布产物启动后读出本机 662 条会话;`claude`、`claude-desktop`、
  `codex`、`hermes`、`workbuddy`、`zcode`、`opencode`、`qoder-cn` 的会话正文
  在界面中实测可读
- 产物校验:SHA256 `A81EB37293CE75EEE460A5B9E9C843408920B05E7FE7A5E634EFDB8FBE82A82F`;
  嵌入版本资源 `0.1.0` 与图标已实测从 exe 中取出
- Platform:Windows 11 Pro 10.0.26200 / amd64
- Toolchain:Go 1.27.0 windows/amd64、Node v22.23.1
- 未执行:macOS / Linux 构建与运行验证(本机为 Windows,跨平台未验证);
  深色模式与 125%/150% DPI 的逐项走查

## Git

- 源码冻结提交:`21fd524`(发布产物的构建来源)
- 发布提交:`release: AgentDeck v0.1.0`
- Tag:`v0.1.0`
