# AgentDeck 项目规则

> 本文件定义 AgentDeck 项目专属规则，补充 `~/.claude/CLAUDE.md` 的全局规则。
> 冲突时以本文件为准（用户当前明确指令除外）。

## Repository Rule

| 项 | 值 |
| --- | --- |
| 产品名 | AgentDeck |
| 仓库名 | `agentdeck`（lowercase-kebab-case） |
| 本地目录 | 仓库根目录（本机路径不作约定） |
| 默认分支 | `main` |
| 可见性 | Public |
| 版本规范 | Semantic Versioning |
| Tag 规范 | `vMAJOR.MINOR.PATCH` |
| Commit 规范 | Conventional Commits |
| Artifact 策略 | 二进制产物上传 GitHub Releases，不入 Git History |

## 技术栈

| 项 | 选型 |
| --- | --- |
| 语言 | Go 1.26.8+ |
| 桌面框架 | Wails v3（已接入，React 18 + Fluent UI v9） |
| SQLite | `modernc.org/sqlite`（纯 Go，无 CGO） |
| JSON | `tidwall/gjson` + `encoding/json` |

**不使用 Tauri / Rust。** 原产品设计文档 §7.1 的 Tauri 选型已被替代，
原因是复用 magpie 的会话层实现。

## 代码来源约定

`internal/` 下多数包**移植自 magpie**（MIT），移植方式为一次性源码搬运。

- 修改移植代码时，在函数或文件顶部注明改动原因，便于日后与上游对比
- 不跟随上游更新；上游修复需人工评估后手动引入
- 新增功能**优先写在独立包**，避免与移植代码混杂
- `internal/provider/` 是本项目自写的最小替代，**不是** magpie 的 provider

移植范围与未移植项详见 `NOTICE.md`。

## 目录约定

```
cmd/          程序入口，一个子目录一个可执行文件
internal/     内部包，不对外暴露
tools/        开发与构建辅助脚本（Python / PowerShell，不属于产品代码）
testdata/     测试夹具
releases/     正式发布快照（vX.Y.Z/）
```

## 文档与设计资料

遵循全局规则：`YYYY-MM-DD-内容-v<文档版本>.<ext>`

产品设计、评审、侦察与实测报告**不随仓库分发**，只保留在本地工作区
（`.gitignore` 之外的本机目录）。仓库里只放对外可公开的说明：
`README.md`、`CHANGELOG.md`、`releases/vX.Y.Z/CHANGELOG.md`、`NOTICE.md`
与 `THIRD-PARTY-NOTICES.txt`。

提交前检查：新文档若含本机路径、真实会话标识、私有项目名或用户账号，
一律留在本地，不 `git add`。

## 测试与验证

- 静态检查：`go vet ./...`
- 编译：`go build ./...`
- 测试：`go test ./... -count=1`
- 格式化：`gofmt -l cmd internal` 应为空

当前基线：21 个包全部通过（`sessions` 约 14s，`settings` 约 14s，`proc` 约 12s，
全套约 50s）。
运行验证：`go run ./cmd/agentdeck -all` 应读出本机 600+ 条会话；
`-search "查询"`（可配 `-agent`/`-all`）对所选会话正文建 FTS5 索引并
全文检索，`-all -search` 实测索引 476 条会话正文。

测试套件移植自 magpie 上游（约 16,000 行）。三处按本项目情况调整过，
均在代码内注明原因：

| 文件 | 改动 | 原因 |
| --- | --- | --- |
| `sessions/price_test.go` | 移除 `TestPriceOfAfterProviderRename` | 依赖 magpie 的 provider 注册表与改名键迁移，AgentDeck 未移植 provider 体系 |
| `sessions/cursor_snapshot_test.go` | `TMPDIR` 改为 `isolateTemp()`，同时设 `TMP`/`TEMP` | 上游只设 `TMPDIR`，Windows 上 `os.TempDir()` 不读它，测试会污染系统临时目录并使断言失效 |
| `sqlitedb/sqlitedb_test.go` | Windows 上跳过 `TestSQLiteSurvivesMalformedEnv` | Windows 内核在 `fork/exec` 阶段拒绝无 `=` 的环境变量，该场景无法构造 |
| `agentenv/agentenv_test.go` | 新增 `upstreamReaders` 表（36 项） | 这些环境变量的唯一读取者都在未移植的包（`agent`/`gateway`/`library`/`provider`）里；变量名保留，因为 `Vars` 是沙箱清理清单 |

未执行的验证必须如实标注，不得写成「通过」。

## 已知技术陷阱

以下为实测发现，改相关代码时注意：

1. **`sessions.List(0)` 不是「全部」**，是「最新的 200 条」（`Limit = 200`）。
   要全部用 `sessions.All`（5000）。诊断会话缺失时先排除此项。

2. **Claude Desktop 是 MSIX 包**，会话在包内 `LocalCache\Roaming\Claude`
   而非 `%APPDATA%\Claude`。`desktopdir` 包同时返回包内与包外路径，两者都要查。
   **Desktop 3.x 起又搬了一次**：整个 userData 移到
   `%LOCALAPPDATA%\Claude-Data\claude_<publisher>`（本机 2026-10-20 迁移），
   旧包目录只剩一个 `.user-data-left.<package>` 标记文件，Desktop 自己的日志
   （`%LOCALAPPDATA%\Claude\logs\main.log`）也写的是 `Claude-Data` 路径。
   `desktopdir.Dirs.Shared` 就是这个目录；新增读取路径时必须让
   `Everywhere()` 带上它，否则 Claude Desktop 会话会一条都读不到（实施前实测
   CLI 返回 0 条）。
   **普通聊天 / 项目会话不落 jsonl**：本机只有内嵌 claude.ai 页面的 IndexedDB
   缓存（`<userData>\IndexedDB\https_claude.ai_0.indexeddb.{leveldb,blob}`），
   打开过的会话整棵消息树（`hub_transcript`）与会话列表查询缓存在里面。值是
   snappy 包装 + V8 序列化，中文是 **UTF-16LE**——按 UTF-8 字节搜标题会得出
   「本机没有」的错误结论（本项目就这样误判过一次）。读取在
   `internal/claudecache`：只读扫 `.log/.ldb` 与 blob 文件，不用 LevelDB 库打开
   （比较器是 `idb_cmp1`，且打开会改写文件），副本写到导入目录旁的
   `claude-desktop-cache/`。没打开过的会话本机没有正文，读不到。

3. **Codex Desktop 与 CLI 共用 `~/.codex`**（与 Claude 的情况相反）。

4. **项目路径有三种表示**，不可从目录名反解，必须读会话文件内的 `cwd` 字段：
   - Claude：`D--Work-ExampleApp`（有损编码）
   - Codex：`\\?\D:\Work\ExampleApp`（扩展长度前缀）
   - ZCode：`D:\Work\ExampleApp`（普通路径）

5. **中文 FTS5 检索**：`unicode61` 与 `trigram` 对 2 字中文词均失效。
   已按 **bigram 预分词** 落地于 `internal/search`：索引侧与查询侧共用
   `Segment`（CJK 相邻二字成词、词内短语保相邻、词间 AND），2 字中文
   词可命中。改分词逻辑必须两侧同步，且分词契约变更 = 整库重建。

6. **控制台代码页是 cp949**（区域 zh-CN）。向 stdout 写中文的 Go 程序需先调
   `SetConsoleOutputCP(65001)`，Python 子进程需 `sys.stdout.reconfigure(encoding='utf-8')`。

7. **Windows 内核拒绝无 `=` 的环境变量**。`os/exec` 传这类条目会直接返回
   "The parameter is incorrect"，子进程起不来。因此 `sqlitedb` 的 `wellFormed`
   防护在 Windows 上无法实测，只能靠 macOS/Linux 验证。

8. **Go 在 Windows 上 `os.TempDir()` 读 `TMP`/`TEMP`，不读 `TMPDIR`**。
   测试要隔离临时目录必须三个变量一起设（见 `sessions` 包的 `isolateTemp`）。

9. **移植的源码在磁盘上是 CRLF**，而 `.gitattributes` 声明 `*.go text eol=lf`。
   换行符转换会一次改动 200+ 个文件，gofmt 也会整文件报差异。批量转换用
   二进制方式做，不要用 `sed`。

10. **WebView2 会缓存 `index.html`，且 `assets/assets` 是只增目录**：改前端后
    直接 `go build` 可能打包出「新 exe 跑旧页面」的假象（旧 js 仍在目录里可达）。
    新增/删除前端文件后必须**先清空 `cmd/agentdeck-desktop/assets/assets`
    再复制 `frontend/dist`**，已固化在 `frontend/scripts/sync-assets.ps1`，
    `Taskfile.yml` 与 `frontend/scripts/rebuild.ps1` 共用此入口。
    **整个 `cmd/agentdeck-desktop/assets/` 都会被 `//go:embed all:assets` 打进 exe**，
    只清 `assets/assets` 不够：从 `frontend/public` 删掉的文件会留在 `assets/agents`
    里继续被打包。v0.9.0 发布产物因此仍内嵌 7 张从安装包提取的品牌 PNG（与其
    CHANGELOG 所述相反，按字节比对确认）。现在脚本清空除 `.gitkeep` 外的全部生成
    内容，复制后校验与 `frontend/dist` 文件清单完全一致，不一致直接失败。

11. **UI 自检截图必须满足两个条件**，否则看到的是假象：
    - **DPI 感知**（`SetProcessDPIAware()`）：否则 `GetWindowRect` 返回虚拟化
      坐标，截出来只有左上角一块，看着像「布局溢出」。
    - **抢到前台**（`SetForegroundWindow` 前先发一次 ALT 键解除系统限制，
      并轮询 `GetForegroundWindow` 确认）：否则抓到的是压在应用上的终端画面。
    - GDI 位图是 **BGRA**，不换通道蓝色会显示成橙色。WebView2 走
      DirectComposition，`PrintWindow` 抓不全，用屏幕 DC `BitBlt`。
    工具：`frontend/scripts/rebuild.ps1`（构建+重启+截图）、`uishot.py`、
    `crop.py`（放大局部比对）、`uiclick.py`（模拟点击）。
    **检查弹窗必须用 `uishot-passive.py` / `uiclick-passive.py`**：`uishot.py`
    会点标题栏以收起输入法浮条，而那个点击会把打开的弹窗一起关掉。
    本机 DPI 是 192（200%），窗口 2560×1448 物理像素；截图按物理像素存，
    点击坐标要用 `截图坐标 × 1.28` 换算后再加到 `GetWindowRect` 的 left/top 上。

12. **`go` / `gofmt` 不在默认 PATH**，工具链在 `C:\Program Files\Go\bin`。
    PowerShell 里先 `$env:Path += ";C:\Program Files\Go\bin"` 再调用。

13. **`releases/vX.Y.Z/source/` 是源码冻结副本，不是当前源码树**，两处必须当心：
    - **任何从仓库根递归扫源码的检查都要跳过 `releases/`**，否则会扫到副本里的
      历史：v0.1.0 发布提交一入库，`go test ./...` 就变红
      （`TestNoCommandBypassesProc`、`TestOnlyThisPackageReachesSQLite` 报的是副本里的
      代码）。这两条是**按文件逐个判定**的，副本里每个文件都要重新过一遍判定，
      所以副本必然把它们打红。`TestVarsAreRead` 是**按名字是否存在判定**的，副本
      影响不到它：副本是同一提交树的逐字节复制，每个文件在原路径同样存在，副本
      提供不了 live 树没有的读取者（`go test ./internal/agentenv -run
      TestVarsAreRead` 在跳过 `releases/` 的当前代码上照样 PASS）。v0.1.1 的
      CHANGELOG 与提交 `19e64eb` 把这条也写成「副本让它恰好通过」，那是错的推断，
      已发布快照按不可变原则未改。新增这类全仓扫描测试时，跳过清单里要有 `releases`。
    - **导出快照时必须排除 `releases/`**，否则把上一版快照整个套进来：
      `git archive HEAD | tar -x -C <dst> --exclude=releases`
      （v0.1.1 用不带 `--exclude` 的写法导出是 1216 个文件 / 11 MB，排除后
      608 个 / 5.4 MB）。

14. **版本号散在 `build/windows/` 两个文件的五处，只改一处会各说各话**：
    - `info.json`：`fixed.file_version`、`fixed.product_version`（不写这项，
      固定信息块的 dwProductVersion 就是 0.0.0.0，与字符串 ProductVersion 矛盾）、
      `info.0409.ProductVersion`、`info.0409.FileVersion`（不写这项，字符串表里
      就没有 `FileVersion` 键，资源管理器改读固定块来显示「文件版本」）。
    - `wails.exe.manifest`：`assemblyIdentity version`，写的是 exe 内嵌**应用清单**
      的版本，与 VERSIONINFO 无关，资源管理器不显示它，但 `grep -a` 能从 exe 里
      刮出旧版本号。v0.1.1 就是只提了 `info.json`，清单里残留 `0.1.0`。
    - 两个文件都改完**必须重跑** `wails3 generate syso`（见 `tools/gen_appicon.ps1`
      顶部注释里的完整命令），否则 syso 里还是旧值；`go build` 不会自己重生成。
    - **`info.json` 还会被前端打包**：`frontend/src/App.tsx` 直接 `import` 它来显示
      标题栏版本，Vite 把值内联进 bundle。改完 info.json 必须重跑 `npm run build`
      再 `sync-assets.ps1`，否则 exe 属性是新版本、标题栏还是旧版本——v0.9.0 发布
      产物就是这样：属性 0.9.0，标题栏 0.8.0（前端包在提版本号之前构建）。
      `sync-assets.ps1` 现在会比较 bundle 内的 `product_version` 与 info.json，
      不一致直接拒绝打包。设置页「当前版本」走的是另一条链（exe → VERSIONINFO）。
    - 验证：`(Get-Item exe).VersionInfo` 看版本资源、资源管理器「详细信息」看用户
      实际看到的两个版本号、`grep -a -o -E 'assemblyIdentity[^>]*' exe` 看清单、
      `grep -a -o 'product_version:"[^"]*"' exe` 看前端包内嵌值。

15. **Codex `state_5.sqlite` 自带 5 个维护 trigger**（`created_at_ms` /
    `updated_at_ms` / `recency_at` 的 `AFTER INSERT` 与 `AFTER UPDATE OF`），
    只响应 INSERT / UPDATE，**DELETE 时不会触发**。删除守卫按 DDL 识别它们
    （单个 `BEGIN…END`、写回已知表）；新增 trigger、表、列、外键仍 fail closed。
    改这段守卫时注意 Go 的 `regexp` **不支持反向引用**，trigger 头要拆成分组解析；
    写入表扫描只能看 `BEGIN` 之后——头部的 `UPDATE OF col` 不是写入。

16. **`wails3 generate bindings` 必须从 `cmd/agentdeck-desktop` 运行**，参数是
    `-d ../../frontend/bindings -ts`（见 `Taskfile.yml`）。在仓库根跑会报
    "no Go files"；加 `-clean` 更会把已有 bindings 清空且不重新生成，
    需要用 `git checkout -- frontend/bindings` 恢复。

17. **恢复命令交给终端时必须编码，不能靠补引号**。恢复命令是
    `<Set-Location ...>; <agent> resume <id>` 这样的复合语句。交给 `wt.exe`
    或 `cmd /c start` 的 `-Command` 时，中间层会重新解析命令行、按空格拆参数，
    分号后的半句被当成另一条参数丢掉，终端开在正确目录却报
    「系统找不到指定的文件」（`启动 "" codex resume <id>"" 时`，0x80070002）。
    现在一律编码成 UTF-16LE base64 交给 `powershell -EncodedCommand`
    （`internal/app/resumectl_windows.go` 的 `encodedCommand`）：编码后不含
    空格、分号与引号，谁也拆不开。`cmd /c start` 那条路虽然子进程
    `IsOutputRedirected=false`（真 TTY），但**窗口在 WT 托管的机器上标题是
    「管理员: 」且用户看不到**，实测弃用。

18. **GUI 父进程直接 `CREATE_NEW_CONSOLE` 起 powershell，子进程 stdout 不是终端**。
    AgentDeck 是 `-H windowsgui`、自身没有控制台，`exec.Cmd` 会把子进程的
    标准句柄接成 NUL：实测 `[Console]::IsOutputRedirected=true`，node 的
    `isTTY` 为 false，agent CLI 因此不上色。修法是显式补 `FORCE_COLOR=1`
    （`resumeEnv`，用户自己设了则不覆盖）——claude.exe 与 codex.exe 都读它。
    走 `wt.exe` 时 stdout 是真 TTY（`IsOutputRedirected=false`），补不补都行，
    统一补上使两条路一致。

19. **Go 侧不需要也不允许再存一份版本号**。`internal/app/version_windows.go`
    在运行时从 exe 的 VERSIONINFO 读回 `build/windows/info.json` 经 syso 写进去的
    ProductVersion（`internal/update` 用它判断是否有新版本）。因此版本号仍然只有
    #14 那些位置，新增 Go 常量会立刻制造第二个真相。
    `go run` 与测试二进制读不到时返回空串，`App.Version()` 报 `dev`，
    更新检查据此不提示「有新版本」（开发构建没有可比较的版本号）。

20. **在线更新的边界**：`internal/update` 只做只读的 release 查询与下载，
    校验 `SHA256SUMS` 后解压到数据目录 `updates/vX.Y.Z/`，**绝不自动替换正在运行的
    exe，也绝不启动下载到的程序**。GitHub 匿名接口限 60 次/小时，所以启动检查
    受「自动检查开关 + 同版本每天一次」（`updates/check.json`）双重约束；
    手动「检查更新」不受限。测试通过 `Config.UpdateAPIURL` 指向本机服务。

## 安全约定

- **默认只读访问上游存储**：发现、读取、索引、导出全链路不写入上游。
- **唯一例外是手动删除会话**（`internal/sessions/native_delete*.go` +
  `internal/app/delete_service.go`）：仅对用户显式选择、预览确认、备份成功的
  对象写入；涉及 Codex `state_5.sqlite` 时在单个事务里 `DELETE` 相关行。
  改动这条路径必须保持以下不变量，测试里都有对应用例：
  - 只对 `codex` / `claude` / `claude-desktop` 的本机原生或保留会话开放，
    WSL / 网关 / 导入来源一律拒绝
  - 预览返回 token 绑定选择与内容哈希；执行前重新校验，源文件变了就整批拒绝
  - 先写备份（原始文件 + 保留版本 + 数据库行快照）再删；备份失败不删
  - 绝不 `RemoveAll` 目录，只删确认过的文件与索引条目
  - 未知数据库 schema 直接拒绝，不猜测着改。唯一例外是 Codex 库自带的维护型
    trigger：仅当 DDL 明确是 `AFTER/BEFORE INSERT` 或 `UPDATE`（不响应 DELETE）、
    单个 `BEGIN…END`、且写入目标都在已知表内时才放行；任何一条不满足仍 fail closed
- 会话内容视为不可信输入：恢复参数（session-id、cwd）必须白名单校验
- 不得在日志、诊断包、提交中泄露凭据文件内容
  （`~/.claude/.credentials.json`、`~/.codex/auth.json` 等）
- 扫描根目录须有明确排除清单
