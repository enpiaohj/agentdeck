# AgentDeck v0.13.0

发布日期：2026-10-22。平台：Windows x64。版本资源：ProductVersion `0.13.0`，FileVersion / 应用清单 `0.13.0.0`。

本版新增会话、智能体、项目的置顶；修复两个影响日常使用的问题：升级后界面偏好回到默认值，
以及右键菜单偶尔出现滚动条。

## 新增

- **置顶**：
  - 会话：在右键菜单（支持多选；所选会话全部已置顶时变为「取消置顶」）或详情栏「更多操作」中置顶。
    置顶的会话集中在列表最上方的「置顶」分组，其余仍按日期分组；行内以图钉标出。
  - 智能体与项目：在侧栏的右键菜单中置顶，排在各自分区的最前面，同样以图钉标出；项目路径
    大小写不敏感。
  - 置顶只影响显示顺序，不改动任何会话数据；保存在界面偏好（`pinnedSessions`、`pinnedAgents`、
    `pinnedProjects`）中，随「完整数据备份」一起备份。删除会话时同步移除其置顶。每类最多保留
    200 条，异常数据在解析时被丢弃。
  - 纯计算放在 `frontend/src/pins.ts`，新增 5 项前端测试。

## 修复

- **升级后界面偏好丢失**：界面偏好（主题、字号、栏宽、删除前备份、置顶等）存放在 WebView2 的
  localStorage 中。Wails 默认把 WebView2 数据目录放在 `%APPDATA%\<exe 文件名>\`，而发布产物的
  文件名带版本号，每次升级都换一个新目录，偏好回到默认值（开发机上已积累 20 多个这样的目录，
  每个约 70 MB）。现在固定使用 `%LOCALAPPDATA%\AgentDeckWebView`：
  - 放在数据根之外，数据位置迁移与「清理旧位置数据」都不会触碰它。
  - 首次启动时，从 `%APPDATA%` 下最近修改过的旧版本目录复制 localStorage：只读旧目录，先复制到
    临时目录再改名，遇到子目录或链接拒绝，失败不留半份；新目录已有偏好时不覆盖。旧目录原样保留，
    可自行删除。
  - 迁移在取得单实例所有权之后进行，此时不会有旧版本正在写它。
  - 新增 3 个测试：取最近目录、永不覆盖已有偏好、拒绝非普通文件。
- **右键菜单出现滚动条**：菜单使用 `autoSize: 'height'`，点击点上下两侧都放不下完整菜单时会被截短
  成带滚动条的窄菜单（靠上或靠下的会话都可能遇到）。现在加上 `shiftToCoverTarget`，菜单竖直
  平移、盖住点击点（与系统右键菜单一致），只有窗口本身放不下时才滚动。

## 依赖维护

- `github.com/tidwall/gjson` 1.19.0 → 1.20.0。
- 同批评估的 `github.com/pelletier/go-toml/v2` 2.4.3 未升级：它改变了 TOML 解析错误的提示文字与
  列号，`TestTOMLParseErrorLocation` 四个子用例失败，留待单独评估。
- 新增 `tools/dismiss_snapshot_alerts.py`：只驳回 `releases/` 快照清单上重复报告的 Dependabot
  告警（默认预览，`--apply` 执行）。
- README 新增「安全与依赖维护」一节，SECURITY.md 补充依赖与构建的处理方式，验证命令加入
  `govulncheck` 与 `npm audit`。

## 安装与升级

1. 下载 `AgentDeck-v0.13.0-win-x64.exe`，或包含程序与许可说明的 `AgentDeck-v0.13.0-win-x64.zip`；
   用 `SHA256SUMS` 校验下载文件。
2. 退出旧程序后，将新 EXE 放到固定位置并启动。已有数据继续使用；首次启动会自动迁入上一版的
   界面偏好，升级前可在「备份与恢复」创建完整备份。
3. **从 v0.12.1 及更早版本升级后**，`%APPDATA%` 下会留着各旧版本的 WebView2 目录
   （`AgentDeck-vX.Y.Z-win-x64.exe\`，每个约 70 MB），确认新版本偏好正常后可自行删除。
4. 若移动了 EXE 且启用了开机启动，在新程序中关闭并重新启用该选项以更新路径。

## 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...`、`go build ./...` 通过（Go 1.27.3）。
- `go test ./... -count=1`：27 个包全部通过。`govulncheck`：No vulnerabilities found。
- 前端 `npm test`：79 项通过（本版新增置顶 5 项）；
  `tsc --noEmit`、`npm run build` 通过；`sync-assets.ps1` 通过，前端包内嵌
  `product_version:"0.13.0"`；`npm audit` 0 项。
- 前端命令输出 Node.js `EnvHttpProxyAgent` 实验性警告，未隐藏，未阻止构建。
- 重新生成 Windows syso，正式 EXE 的 ProductVersion 为 `0.13.0`、FileVersion 为 `0.13.0.0`、应用清单
  版本 `0.13.0.0`；EXE 内不含本机绝对路径与已移除的品牌图。
- 正式构建：`go version -m` 记录 `go1.27.3`、`klauspost/compress v1.18.7`、`gjson v1.20.0`、
  `vcs.revision=ef8faa8610b980db8f1315e132552a6ca992d9ab`、`vcs.modified=false`、`CGO_ENABLED=0`、
  `GOOS=windows`、`GOARCH=amd64`；使用 `-trimpath`、`production` 标签、Windows GUI 子系统。
- **界面实测**（隔离演示用户目录 + 虚构会话，窗口 1280×800，未读取真实会话；测试用 exe 临时更换了
  单实例互斥锁名，该改动没有提交，发布产物使用原名称）：
  - 右键菜单：靠上、居中、靠下三个位置，菜单均完整显示，没有滚动条；加入「置顶」一项后仍然如此。
  - 会话置顶：右键「置顶」后，列表最上方出现「置顶 1」分组，行内显示图钉；详情栏按钮在「置顶」与
    「取消置顶」之间切换，取消后该会话回到按日期分组的位置。
  - 项目置顶：右键「置顶」后该项目排到项目分区最前并显示图钉。
  - 智能体置顶：右键菜单出现「置顶」，置顶后显示图钉。
  - 重启应用后，三类置顶全部保留。
  - 偏好迁移：首次启动后 `%LOCALAPPDATA%\AgentDeckWebView\EBWebView\Default\Local Storage\leveldb`
    出现，且包含从旧目录复制来的 LevelDB 文件。
- 未操作开机启动注册项。

## 已知问题与未执行

- 未做代码签名，Windows SmartScreen 可能显示未知发布者；SHA-256 校验仅保证与发布清单一致。
- 界面实测在隔离演示数据上进行，未在真实用户数据上验证置顶；偏好迁移只验证了「目录出现且含数据」，
  未对比迁移前后的具体偏好取值。
- 置顶的会话如果被上游清理且不再出现在列表中，其 key 会留在偏好里（每类上限 200 条），不影响显示。
- 界面实测只在 1280×800 一种窗口尺寸下进行；未执行多 DPI 完整走查、macOS / Linux 构建与运行验证。
- `internal/filememo` 的 `TestReadWhileReplaced` 在 Windows 上曾偶发失败，本轮完整测试通过，后续仍应
  保留失败证据并调查。

## Git

- 构建提交：`ef8faa8610b980db8f1315e132552a6ca992d9ab`（`build: 准备 v0.13.0 的版本号、资源与文档`）
- 功能提交：`dbb724f`（置顶与右键菜单修复）、`9b9971b`（固定 WebView2 数据目录）
- Tag：`v0.13.0`（指向本快照所在的发布提交）
