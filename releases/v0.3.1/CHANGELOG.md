# AgentDeck v0.3.1

发布日期：2026-10-19。平台：Windows x64。补丁版本，只修「继续会话」在终端里打不开的问题，并恢复恢复出来的会话的颜色。

## 修复

- **「继续会话」报「系统找不到指定的文件」（0x80070002）**。恢复命令是一条复合语句：`Set-Location -LiteralPath '<cwd>'; <agent> resume <id>`。此前它被当作 `-Command` 的值交给 `wt.exe`，而 wt 会重新解析自己的命令行、按空格拆参数——分号之后的那半句被当成另一条参数丢掉，终端开在了正确的工作目录，却去执行一个不存在的文件名 `codex resume <id>`，于是报 `启动 "" codex resume <id>"" 时，系统找不到指定的文件`。Claude Code 用的是同一条复合语句，因此两个 agent 一起失效。
  改为把整条脚本编码成 **UTF-16LE base64** 交给 `powershell -EncodedCommand`。编码后的参数不含空格、分号与引号，任何会重新解析命令行的中间层都拆不开。
- **恢复出来的会话没有颜色**。AgentDeck 是 `-H windowsgui` 程序，自身没有控制台；以 `CREATE_NEW_CONSOLE` 直接启动 powershell 时，`exec.Cmd` 会把子进程的标准句柄接成 NUL，实测子进程 `[Console]::IsOutputRedirected = true`、node 的 `isTTY = false`。agent CLI 判断「要不要上色」正是看这个，于是输出退化为无色。改为显式补 `FORCE_COLOR=1`（`resumeEnv`）：用户自己设了 `FORCE_COLOR` 则不覆盖。

## 变更

- **目录不存在时仍然恢复会话**。此前 cwd 不存在会让整条 wt 路径作废；现在只是不再传 `-d`，恢复命令照常执行，终端落在默认目录。
- `frontend/package-lock.json` 的版本号此前一直停在 `0.2.1`（v0.3.0 那次只提了 `package.json`），本次一并同步到 `0.3.1`。

## 未采用的方案（实测记录）

- **`cmd.exe /c start`**：子进程确实拿到真 TTY（`IsOutputRedirected = false`），但实测在本机建出的控制台窗口不可见（只有一个 0×0 的 `PseudoConsoleWindow`），用户什么都看不到，弃用。
- **给 `-Command` 补引号**：要在中间层的转义规则上再叠一层，而中间层的转义规则本身没有文档保证，不可靠。

## 升级

关闭旧程序，运行 `AgentDeck-v0.3.1-win-x64.exe`。沿用现有本地数据目录，无数据库迁移、无数据清理；本次不触碰任何上游存储。

## 验证

- `gofmt -l cmd internal` 输出为空；`go vet ./...` 无输出；`go build ./...` 退出码 0。
- `go test ./... -count=1` 全部 21 个包通过（`sessions` 约 16s，`settings` 约 15.5s，`proc` 约 12s）。
- 前端：`npx tsc --noEmit` 退出码 0；`npm test` 30 / 30 通过。
- 新增 Go 用例 5 条（`internal/app/app_test.go`）：恢复命令必须以 `-EncodedCommand` 交付且脚本原文不出现在任何参数里；根路径与不存在的目录各一条；`-EncodedCommand` 的 UTF-16LE 往返（含中文路径与 PowerShell 单引号转义）；`FORCE_COLOR` 只在调用方未设时补、已设则不覆盖。
- **真机验证**（Windows 11 Pro 26200，DPI 200%，WebView2）：
  - Codex：选中 `ExampleApp-Main` 点「继续会话」，新终端标题 `ExampleApp-Main | ExampleApp`，`codex resume 01a13d4d-…` 完整执行，会话正文正常渲染且**有颜色**（绿色的「已通过！」、蓝色项目符号、青色提示行），cwd 为 `D:\Work\ExampleApp`。
  - Claude Code：选中 `CalendarFlow-Main` 点「继续会话」，新终端标题 `CalendarFlow-Main`，`claude --resume 5f2f20f4-…` 完整执行，正文有颜色，cwd 为 `D:\Work\CalendarFlow`。
  - 从资源管理器启动（环境与用户双击一致）复验 Codex 一路，结果同上。
- 正式构建使用 `go build -tags production -ldflags "-H windowsgui -s -w"`。`(Get-Item exe).VersionInfo`：FileVersion `0.3.1.0`、ProductVersion `0.3.1`、FileDescription「跨 Agent 历史会话工作台」、ProductName / CompanyName `AgentDeck`；内嵌应用清单 `assemblyIdentity ... version="0.3.1.0"`；exe 内 `0.3.0` 残留为 0 处。
- `go version -m` 确认 `GOOS=windows`、`GOARCH=amd64`、`CGO_ENABLED=0`、`vcs=git`、`vcs.modified=false`、`vcs.revision=5bcc2f855ff1eaa6b24c7948a6e636668e7b3045`。
- 快照 685 个文件由 `git archive HEAD | tar -x -C <dst> --exclude=releases` 导出；逐文件 `git hash-object` 与 HEAD（排除 `releases/`）比对，685 / 685 哈希完全相同，不含嵌套 `releases/`。
- 工具链：Go 1.27.0 windows/amd64，Wails 3.0.0-beta.30，Node 22.23.1，npm 10.9.8。

## 已知限制与未执行项

- **恢复出的终端是否「管理员」取决于本机的 UAC 设置**，不是 AgentDeck 申请的。本机 `EnableLUA = 0`（UAC 关闭），所有进程都以高完整性运行，因此终端标题带「管理员: 」前缀。AgentDeck 的清单是 `requestedExecutionLevel level="asInvoker"`，代码里没有 `runas` / `ShellExecute` / `IsUserAnAdmin`。
- 未执行：`wt.exe` 不可用时的 powershell 回退路径的**真机点击验证**（本机 `wt.exe` 始终可用，无法在不改变用户环境的前提下走到该分支；该分支只由 `TestTerminalCommandRootPath` 与 `TestTerminalCommandKeepsScriptUnsplittable` 覆盖）。
- 未执行：macOS / Linux 构建与运行；代码签名与安装包。
- 本机未配置 Git 远端，Push 与 GitHub Release 未执行。

## 产物与追溯

| 产物 | 大小 | 内容 |
| --- | --- | --- |
| `AgentDeck-v0.3.1-win-x64.exe` | 17,981,440 字节 | 程序本体，未签名单文件 |
| `AgentDeck-v0.3.1-win-x64.zip` | 7,672,003 字节 | 分发用：解压得到 `AgentDeck-v0.3.1-win-x64/`，内含 exe、本 CHANGELOG、`SHA256SUMS` |
| `AgentDeck-v0.3.1-source.zip` | 5,193,304 字节 | 归档与重新构建用：解压得到 `AgentDeck-v0.3.1-source/`，即 `source/` 的 685 个文件 |

- SHA256（同时见 `SHA256SUMS`，`sha256sum -c` 三项全部 OK）：
  - `AgentDeck-v0.3.1-win-x64.exe`：`b61ac5cd56177ce90fa3bf0d0f762167d98798f8bb41eec2486b295af94d248c`
  - `AgentDeck-v0.3.1-win-x64.zip`：`4e02c4bb8d17412e216a644de6ea2714747a63f6f68e7e34bc242cdfc876284d`
  - `AgentDeck-v0.3.1-source.zip`：`6a64a9e29c78b2c9854a27ca37b21ce4df1dbf03233d5bb0d8d5e742bf823125`
- 两个 zip 的条目名以 UTF-8 写入（显式指定 `UTF8Encoding`，不用 `Compress-Archive`——它按本地代码页 cp949 写条目名，中文文件名解压后会乱码）。解压复核：源码包 685 个文件、分发包 3 个文件，包内 exe 的 SHA256 与上表一致。
- 源码冻结提交：`5bcc2f855ff1eaa6b24c7948a6e636668e7b3045`；二进制、`source/` 与 `source.zip` 均来自该提交。
- 发布提交：`release: AgentDeck v0.3.1`；Tag：`v0.3.1`，指向包含本快照的发布提交。
- 本地不可变快照：`releases/v0.3.1/`。三个产物均被 `.gitignore` 的 `releases/**/*.{exe,zip}` 排除，不入 Git History。

> **归档补充说明（2026-10-19）**：初版发布提交 `dd0c96d` 漏了分发用的 zip，仅打包了 exe。经用户明确要求，在同一发布快照内补入 `AgentDeck-v0.3.1-win-x64.zip` 与 `AgentDeck-v0.3.1-source.zip`，并同步更新本 CHANGELOG 与 `SHA256SUMS`。
>
> 补充的**只有归档产物与元数据**，源码、二进制、`source/` 均未改动：exe 的 SHA256 仍是 `b61ac5cd…`，`source/` 仍是冻结提交 `5bcc2f8` 的 685 个文件。Tag `v0.3.1` 保持指向 `dd0c96d` 不变——移动已发布的 Tag 违反不可变原则，因此本节的元数据补充作为其后一个提交记录，Tag 与快照的对应关系不受影响（zip 本就不入 Git）。
