# AgentDeck v0.1.1

发布日期：2026-10-17

补丁版本：应用图标重新设计，并修复 v0.1.0 发布时才引入的一处测试回归。

## Changed

### 应用图标重新设计（换掉了四根白条的旧图）

底色不变（azure → indigo 渐变圆角方片 + 顶部高光 + 底部暗角），中间的白图形
换成会话列表本身：

- 正在工作的那一行是一张完整白色卡片，右端一枚四角星（「有 Agent 正在工作」）；
- 下面两行更淡、长度不一的会话行。

选它的理由是 16~32px 这一档（任务栏、资源管理器、Alt-Tab 都用这一档）：这里
只容得下一个白色主体。新图形只有一个实心白块加一个亮点，32px 仍读得出「列表 +
某一行在工作」；旧图（四根白条）在这一档会读成表格/窗口图标。

同样画过并放弃的方案，以及三版对比图（256 / 64 / 32 / 16 真实像素）见
`docs/2026-10-17-AgentDeck应用图标方案-v1.0.png`。生成器
`tools/gen_appicon.ps1` 改为 `-Design rows|deck|panes` 三选一（默认 `rows`），
新增 `-Sheet <目录>` 输出各档单图与小尺寸最近邻放大图；切换与重新嵌入 exe 图标的
步骤写在 `frontend/src/icons/README.md`。

## Fixed

- **仓库级约束测试扫到发布快照而失败**（自 v0.1.0 发布起，`go test ./...` 是红的）：
  `releases/vX.Y.Z/source/` 是每个已发布版本的源码冻结副本，三条从仓库根递归扫
  源码的测试扫到了副本里的历史——
  - `TestNoCommandBypassesProc` 报副本里的 `proc.go` 用 `os/exec` 起命令；
  - `TestOnlyThisPackageReachesSQLite` 报副本里的 `sqlitedb.go` 直接 import `modernc`;
  - `TestVarsAreRead` 只是「恰好」通过：副本里同名的变量把 `found` 置上了，等于一个
    已无读取者的变量名只要在旧快照里出现过就算「还在被读」，约束被悄悄放宽。

  三处都把 `releases/` 加进各自的跳过清单。**此回归由 v0.1.0 的发布提交引入**
  （快照入库前全绿），v0.1.1 修掉。

- 本版源码快照导出时排除 `releases/`：v0.1.0 的快照是干净的（当时 `releases/`
  还不存在），但按 v0.1.0 的做法导出 v0.1.1 会把上一版的快照整个套进来
  （1216 个文件 / 11 MB），违反「快照必须排除 `releases/` 以防递归嵌套」。

## Improved

- UI 自检脚本（`frontend/scripts/uishot.py`、`uiclick.py`）改为按进程名通配
  `agentdeck*` 找窗口：发布产物改名成 `AgentDeck-v0.1.1-win-x64.exe` 后进程名不再
  是 `agentdeck-desktop`（Windows 的进程名取自 exe 文件名），原来写死的名字取不到
  窗口。只影响开发脚本，不影响产品二进制。

## Known Issues

- **OpenCode 没有官方品牌图标**：本机只装了它的更新器安装包，没有产品图标资源，
  界面退化为识别色方片 + 首字母。
- **Codex 有 3 条会话本身没有可读正文**（其文件内就没有可解析的消息）。
- 「交接」「生成摘要」「Token 统计」是 v1.1 规划，界面中置灰并标注。
- 深色模式未逐项走查。
- 未做代码签名，未提供安装包（单个裸 exe）。
- 界面内不显示版本号，只有 exe 的文件版本（本版 0.1.1）。
- 本机尚未配置远端仓库，没有 GitHub Release；分发包即本快照内的 exe。

## Verification

- Build:`go build ./...` 通过;前端 `npm run build` 通过(vite 2249 modules,
  `dist/assets/index-5V4tl8eB.js` 443.82 kB → gzip 133.73 kB);
  发布构建 `go build -tags production -ldflags "-H windowsgui -s -w" -o bin/agentdeck-desktop.exe ./cmd/agentdeck-desktop`
- Tests:`go vet ./...` 干净;`go test ./... -count=1` 21 个含测试的包全部通过;
  `gofmt -l cmd internal` 输出为空
- 运行验证:发布产物启动后读出本机 662 条会话,四栏界面与正文渲染正常
- 产物校验:SHA256 `497AA51165701AD81E27C7A12E4A053EF4CE59A727F2AAFE48BFBEE0B5A4D0E8`;
  从 exe 中实测读出嵌入版本 `0.1.1`(不含 `0.1.0` 残留)与新产品图标
- Platform:Windows 11 Pro 10.0.26200 / amd64
- Toolchain:Go 1.27.0 windows/amd64、Node v22.23.1、wails3 v3.0.0-beta.30
- 未执行:macOS / Linux 构建与运行验证(本机为 Windows);深色模式与
  125%/150% DPI 的逐项走查

## Git

- 源码冻结提交：`3e73714`（本快照导出自该提交）
- 发布产物的构建来源：`31ffd60`；两者之间只差一个提交，且只动了 `CLAUDE.md`
  （增补「发布快照的两个坑」一节），不参与编译，二进制不受影响
- 发布提交：`release: AgentDeck v0.1.1`
- Tag：`v0.1.1`
