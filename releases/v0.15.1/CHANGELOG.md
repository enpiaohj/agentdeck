# AgentDeck v0.15.1

发布日期：2026-10-23

v0.15.0 把在线更新改成「下载后在应用内装完」，本版修的是那条链路上几处
不够保守的地方。没有新功能，行为变化都集中在更新流程与它的失败路径上。

## Fixed

**更新流程不再认领用户自己的备份文件**

原地替换时旧文件要先改名留作回滚副本。此前这个名字是 `<程序名>.old`——而
`.old` 恰恰是用户自己做备份时最常用的后缀。同目录出现 `AgentDeck.exe.old`
的常见原因就是用户手动备份过，更新流程把它当成自己的回滚副本，善后阶段
直接删掉，等于删了用户的文件。

回滚副本改用 `.agentdeck-backup`。带上产品名之后，同目录里出现它只可能是
本程序的更新流程留下的，善后可以放心清。

**启动善后清掉 side-by-side 的临时文件残留**

side-by-side 落地时，新文件先写成 `<新版本名>.new` 再改名到位。若在写完
staging、改名之前崩溃，这个残留文件与当前程序名不同名，善后阶段那行
`remove(current + ".new")` 清不到它，会一直留在程序目录里。

现在按发布产物命名扫一遍同目录（`AgentDeck-vX.Y.Z-win-x64.exe.new`）并清掉。
只认这一种命名、只删 `.new` 这一种后缀，用户放在同目录的普通 `.new` 文件
不受影响。

**快捷方式改指向不再静默失效**

`CoInitializeEx` 返回 `S_FALSE` 表示「线程已初始化过 COM，本次没有增加引用
计数」。此前这个分支与成功初始化一样返回 `ole.CoUninitialize`，调用方一执行
就减掉了别人的计数——同一线程调两次就会把计数减穿，之后该线程上的 COM 调用
全部失败，表现是「更新成功了但快捷方式还是指向旧文件」，且不报错。

`S_FALSE` 与 `RPC_E_CHANGED_MODE` 现在都返回空操作。判定逻辑抽成纯函数
`comOutcome`，可以直接对四种返回码做断言——这两个分支依赖线程既有的 COM
状态，在测试进程里很难稳定构造。

**启动善后移到取得单实例所有权之后**

另一个实例正在运行时，本进程马上会退出。此前善后在这一步之前执行，等于替
一个自己都不继续跑的进程去删旧文件、改启动项。与 WebView2 数据目录迁移是
同一个道理。

**落地失败后拉起的原版本不会再直接消失**

落地失败时 `Finish` 已经写了失败标记，新实例启动会读到并提示。但那次重启
此前没有带 `--update-failed`，也就没有「等旧进程退出」的宽限期，新实例抢不到
单实例锁会立刻退出——用户看到的是「重启后程序没了」，连提示都读不到。

`--update-failed` 现在与 `--after-update` 一样进入等待分支，并使用 30 秒宽限。

**不支持应用内更新时不再显示无效按钮**

通过 `go run` 或测试二进制启动时 `can_install` 为 false，此前仍显示「下载到
本地」，点了必然报错。现在改为「到发布页下载」，并说明当前运行方式无法在
应用内安装。

**标记写不成时补失败标记**

side-by-side 落地写完新文件后要写 `.replaces` 标记，内容是被取代的旧文件名。
标记写不成时此前只是「尽力而为」，后果是旧文件永远留着——清理逻辑没有标记
就不敢删。现在写不成会补 `MarkFailed`，下次启动至少提示一次。

## Verification

- Build：`go build ./...`、`go build -trimpath -tags production -ldflags "-H windowsgui -s -w"` 通过
- Tests：`go test ./... -count=1` 27 个包全部通过；前端 `npm test` 87 项通过
- Static：`gofmt -l cmd internal` 为空、`go vet ./...` 无输出
- 前端构建：`npm run build` + `sync-assets.ps1` 通过，包内 `product_version` 与 `info.json` 一致
- 版本号：VERSIONINFO `0.15.1` / `0.15.1.0`、清单 `assemblyIdentity 0.15.1.0`、前端包内嵌 `0.15.1`，三处一致
- 进程级端到端：在隔离用户目录与独立单实例锁下，用真实 exe 走完「检查 → 下载 →
  校验 → 退出时落地 → 新实例善后删旧文件 → 拉起新版本」全程；落地文件与更新包
  SHA-256 一致，`.replaces` 标记已清，无 `.new` / `.agentdeck-backup` 残留
- 单元测试覆盖新增行为：`TestInstallInPlaceKeepsUserOldBackup`（用户的 `.old` 在
  落地与善后后都原样保留）、`TestCleanupRemovesStaleSideBySideStaging`（残留被清、
  用户的 `notes.new` 不受影响）、`TestComOutcome`（四种返回码的 counted/ok 判定，
  已确认在注入原缺陷时会失败）

## Known Issues

无。

## Git

- Commit：`04c2d81`
- Tag：`v0.15.1`
