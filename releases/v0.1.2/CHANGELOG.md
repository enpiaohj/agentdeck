# AgentDeck v0.1.2

发布日期：2026-10-17

修正版本，不改产品行为：把版本号在发布产物里的落点补齐，并更正 v0.1.1 审计出来的
几处不成立的说法。

## Fixed

### 版本号在 exe 里不再有旧值残留

- **`build/windows/wails.exe.manifest` 的 `assemblyIdentity version` 从未随版本号提升**。
  该文件是 `wails3 generate syso` 的输入之一，所以 v0.1.0 与 v0.1.1 的 exe 里都嵌着
  `version="0.1.0.0"`——它属于内嵌**应用清单**，资源管理器不显示，但确实在产物字节里。
  （v0.1.1 的 CHANGELOG 写过「不含 0.1.0 残留」，那句话当时不成立。）
- **`info.json` 补 `fixed.product_version`**：此前固定信息块的 dwProductVersion 是
  `0.0.0.0`，而字符串表里的 ProductVersion 是真实版本，同一个版本资源里两个产品版本
  互相矛盾。不写这项是 wails 上游模板的默认行为（`version/json.go` 允许写）。
- **`info.json` 补 `info.0409.FileVersion`**：此前版本资源字符串表里没有 `FileVersion`
  键，`VersionInfo.FileVersion` 取到空串，资源管理器只能回退到固定信息块显示「文件版本」。

本版产物的实测（对 `releases/v0.1.2/` 内的 exe）：资源管理器「文件版本 `0.1.2.0` /
产品版本 `0.1.2`」；ASCII 与 UTF-16 两路全字节扫描都找不到 `0.1.1` 或 `0.1.0`；
内嵌清单是 `assemblyIdentity ... name="AgentDeck.Desktop" version="0.1.2.0"`。

## Corrected

v0.1.1 的快照按不可变原则**未改动**，以下三处在本版更正（同一说法也留在提交
`19e64eb` 的信息里，提交信息不可改）。前两处来自发布当时的错误推断。

1. **`TestVarsAreRead` 的解释不成立**。v0.1.1 写它「只是恰好通过——副本里同名的变量
   把 found 置上了……约束被悄悄放宽」。实测不成立：快照副本是**同一提交树的逐字节
   复制**，每个文件在原路径同样存在，副本提供不了 live 树没有的读取者，因此它既不会
   让这条测试变红，也没有放宽任何约束。真正被副本打红的是**按文件逐个判定**的两条
   （`TestNoCommandBypassesProc`、`TestOnlyThisPackageReachesSQLite`）；`TestVarsAreRead`
   是**按名字是否存在判定**，副本影响不到它。`CLAUDE.md` 陷阱 13 已改写。
2. **「不含 0.1.0 残留」不成立**：见上，v0.1.1 的 exe 内嵌清单里确实有一个 `0.1.0`。
   当时能读出 `0.1.1` 的是版本资源（字符串 ProductVersion `0.1.1`、固定文件版本
   `0.1.1.0`），这两项一直是对的。
3. **Known Issues 的「『Token 统计』界面中置灰并标注」与界面不符**。界面里没有名为
   「Token 统计」的置灰项：Token 是实时值（会话信息面板「Token 消耗 约 …」）；
   实际置灰的是「生成新摘要」「交接到其他 Agent」两个按钮与项目/文件/工具调用三个页签。
   该行与 v0.1.0 的 CHANGELOG 逐字相同，属沿用未更正。

## Added

- **仓库根 `CHANGELOG.md`**：按 `github-repository.md` §5，正式发布项目的根目录应有
  变更日志。此前只有 `releases/vX.Y.Z/CHANGELOG.md`，根目录没有索引。
- **`CLAUDE.md` 陷阱 14**：版本号散在 `build/windows/` 两个文件的五处、改完必须重跑
  `wails3 generate syso`；并更正过期的测试基线（18 → 21 个包）。

## Known Issues

- **全套测试存在偶发失败（本版验证期间新发现，与本次改动无关）**：改动落地后
  `go test ./... -count=1` 跑了约 16 次，其中 3 次各命中一个**不同的**偶发失败
  （`TestIncremental`、`TestWSLOmpProfiles`（均在 `internal/sessions`）、
  `TestSaveConcurrent`（`internal/settings`）各一次），其余运行 21 个包全绿。
  单跑对应的包（`internal/sessions` 3 次、`internal/settings` 的并发测试 6 次）全部通过，
  说明它们只在整机高并发（21 个包并行 + 应用在跑）时发作，属时序/负载敏感。
  初步线索：`internal/steady` 自述「Windows 上被改名覆盖的文件有两毫秒左右是不存在的」，
  读侧重试预算（50×10ms / 15×1ms）在整机高并发时可能被耗尽，而 `TestSaveConcurrent`
  断言读错误为 0，比实现承诺的语义更严；`TestIncremental` 与 `TestWSLOmpProfiles`
  则与包级共享状态和后台 WSL 扫描 goroutine（`wslSettle`）相关。未定位到具体行，
  列为下一版第一件事。
- **OpenCode 没有官方品牌图标**：本机只装了它的更新器安装包，没有产品图标资源，
  界面退化为识别色方片 + 首字母。
- **Codex 有 3 条会话本身没有可读正文**（其文件内就没有可解析的消息；v0.1.2 期间
  实测全量 452 条 codex 会话中恰为 3 条）。
- 「交接」「生成摘要」是 v1.1 规划，界面中置灰并标注。
- 深色模式未逐项走查。未做代码签名，未提供安装包（单个裸 exe）。
- 界面内不显示版本号，以 exe 的文件版本为准（本版 `0.1.2.0` / 产品版本 `0.1.2`）。
- 本机尚未配置远端仓库，没有 GitHub Release；分发包即本快照内的 exe。

## Verification

- Build：`go build ./...` 通过；前端 `npm run build` 通过（vite 2249 modules，
  `dist/assets/index-5V4tl8eB.js` 443.82 kB → gzip 133.73 kB，本版未改前端，bundle 名未变）；
  发布构建 `go build -tags production -ldflags "-H windowsgui -s -w" -o bin/agentdeck-desktop.exe ./cmd/agentdeck-desktop`
- Tests：`go vet ./...` 干净；`gofmt -l cmd internal` 输出为空；`go test ./... -count=1`
  约 16 次中 13 次 21 个包全绿，3 次命中上述偶发失败（详见 Known Issues）
- 运行验证：用**发布产物本身**（`AgentDeck-v0.1.2-win-x64.exe`）启动，读出本机 664 条
  会话，四栏界面、正文渲染、会话信息面板均正常；应用内品牌标识为新图标
- 产物校验：SHA256 `07ED18F436F937DD9A09B30F290ED576342D01D505D3E8DED9F5AE1A73B1BBAF`；
  `go version -m` 读出 `vcs.revision=a72e8669a4c3...`、`vcs.modified=false`、
  `-tags=production`、`-ldflags="-H windowsgui -s -w"`
- 快照校验：`releases/v0.1.2/source/` 与冻结提交逐字节相同（`diff -r` 无输出，609 个文件）；
  无嵌套 `releases/`、无 `node_modules`/`dist`/`_probe`/`bin`
- Platform：Windows 11 Pro 10.0.26200 / amd64
- Toolchain：Go 1.27.0 windows/amd64、Node v22.23.1、wails3 v3.0.0-beta.30
- 未执行：macOS / Linux 构建与运行验证（本机为 Windows）；深色模式与 125%/150% DPI
  的逐项走查；Push 与 GitHub Release（本机无远端）

## Git

- 源码冻结提交：`a72e866`（本快照导出自该提交，产物也构建自该提交，两者同源）
- 版本提升与文档提交：`ed379f9`（版本号 0.1.2）、`a72e866`（根 CHANGELOG + CLAUDE.md）
- 发布提交：`release: AgentDeck v0.1.2`
- Tag：`v0.1.2`
