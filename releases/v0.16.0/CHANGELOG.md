# AgentDeck v0.16.0

发布日期：2026-10-27

## Added

- 设置 → 终端新增「继续会话时最大化打开终端」，默认关闭。开启后继续会话打开的终端以最大化窗口显示：
  检测到 Windows Terminal 时用它的 `-M`，没有时给新建的控制台窗口 `-WindowStyle Maximized`。
- 设置 → 终端新增「继续会话时的打开方式」，可选「新窗口」（默认）与「新标签」。「新标签」在活动的
  Windows Terminal 窗口中开一个新标签；没有打开的终端窗口时新建一个。未检测到 Windows Terminal 时
  该选项不可选，后端也拒绝保存该取值。
- 两项取值都落在 `%APPDATA%\AgentDeck\app-config.json`（`resume_maximized` / `resume_open_mode`），
  与终端 shell 设置同一份配置、同一条写入路径（先写文件再更新内存，配置读不出来时拒绝写入）。

## Changed / Improved

- 设置页两项新增说明文字区分作用范围：只影响继续会话，新建会话仍按普通窗口打开。
- 最大化说明里写明「复用已有窗口开标签时不改变该窗口尺寸」——`-M` 只在新建窗口时生效，
  目标窗口多大由用户自己决定，程序不去改它。

## Known Issues

- 「新标签」依赖 Windows Terminal。未安装时回退路径只能开新窗口，「新标签」选项置灰；
  已保存的 `tab` 取值在没有 `wt.exe` 的环境里自动按新窗口处理。
- `-WindowStyle Maximized` 只在窗口由 conhost 托管时生效。若 Windows Terminal 是系统默认终端，
  `CREATE_NEW_CONSOLE` 建出的窗口会被转交给它托管，此时该参数不起作用（实测仍是普通窗口）。
- `-w 0` 指向「最近使用的窗口」，这是 Windows Terminal 的定义。没有自己新开窗口时，标签会开进
  用户当时正使用的那个终端窗口——这是选项语义本身，不是缺陷。
- 本轮未执行 macOS / Linux 构建与运行验证，也未做不同 DPI 的完整走查。

## Verification

- `gofmt -l cmd internal`：无输出；`go vet ./...`、`go build ./...`：通过。
- `go test ./... -count=1`：28 个测试包全部通过（`exit=0`）。
- `npm test`：108 项通过；`npm run build` 与 `sync-assets.ps1`：通过。
- `govulncheck@v1.1.4`：No vulnerabilities found；`npm audit`：0 vulnerabilities。
- Windows x64 生产构建使用 `-trimpath`；`go version -m` 的 `vcs.revision` 等于下述构建提交，
  `vcs.modified=false`。
- 真机实测（隔离演示实例 + 命名探针窗口）：开启「新标签」后点继续会话，探针窗口标签数 1 → 2，
  用户自己的终端窗口标签数不变（1）；关闭全部终端窗口后再继续会话，成功新建窗口。
- 真机实测最大化三态：`-M` 得 `-8,-8,2568,1400`（最大化），关闭时 `330,338,1459,973`，
  conhost 回退路径 `-WindowStyle Maximized` 同样得 `-8,-8,2568,1400`。
- 最终 EXE 校验固定与字符串版本资源、应用清单和前端内嵌版本均为 0.16.0。
- 前端工具链有 `EnvHttpProxyAgent` 实验性警告；Git 有 LF/CRLF 转换提示，均未屏蔽。

## 未执行

- 真实会话的完整交互（只验证终端起得来、标签落点与窗口尺寸正确，未在终端里继续实际对话）。
- macOS / Linux 构建与运行；不同 DPI 的完整界面走查。

## Git

- 构建与 source/ 来源提交：253f7639b774e0f8ed6e9c76dfffcfd4e4d3a74a。
- Tag：v0.16.0，指向单独的发布快照提交。
