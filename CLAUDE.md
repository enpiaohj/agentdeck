# AgentDeck 项目规则

> 本文件定义 AgentDeck 项目专属规则，补充 `~/.claude/CLAUDE.md` 的全局规则。
> 冲突时以本文件为准（用户当前明确指令除外）。

## Repository Rule

| 项 | 值 |
| --- | --- |
| 产品名 | AgentDeck |
| 仓库名 | `agentdeck`（lowercase-kebab-case） |
| 本地目录 | `D:\Work\AgentDeck` |
| 默认分支 | `main` |
| 可见性 | Private |
| 版本规范 | Semantic Versioning |
| Tag 规范 | `vMAJOR.MINOR.PATCH` |
| Commit 规范 | Conventional Commits |
| Artifact 策略 | 二进制产物上传 GitHub Releases，不入 Git History |

## 技术栈

| 项 | 选型 |
| --- | --- |
| 语言 | Go 1.26.8+ |
| 桌面框架 | Wails v3（待接入） |
| SQLite | `modernc.org/sqlite`（纯 Go，无 CGO） |
| JSON | `tidwall/gjson` + `encoding/json` |

**不使用 Tauri / Rust。** 原产品设计文档 §7.1 的 Tauri 选型已被替代，
原因是复用 magpie 的会话层实现（见 `docs/` 下的可行性分析）。

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
tools/        开发与侦察脚本（Python，不属于产品代码）
docs/         设计文档、实测报告、决策记录
testdata/     测试夹具
releases/     正式发布快照（vX.Y.Z/）
```

## 文档命名

遵循全局规则：`YYYY-MM-DD-内容-v<文档版本>.<ext>`

已归档文档：

| 文件 | 内容 |
| --- | --- |
| `docs/2026-10-16-AgentDeck设计评审报告-v1.0.html` | 对 v1.0 产品设计文档的评审 |
| `docs/2026-10-16-Magpie复用可行性分析-v1.0.md` | 复用 magpie 的依赖闭包分析 |
| `docs/2026-10-16-四目标存储与恢复实测-v1.0.md` | 四个目标的存储真相实测 |
| `docs/2026-10-16-移植可行性实验报告-v1.0.md` | 移植实验的完整记录 |
| `docs/2026-10-16-Claude与Codex适配器实现规格-v1.0.md` | 适配器实现规格（⚠️ 类型草案是 Rust，存储真相有效） |
| `docs/2026-10-16-侦察结论复核与41条修正-v1.0.md` | 两族侦察结论的对抗性复核，41 条修正 |

前四份是 Phase 0 的决策依据，后两份是 Phase 1 的输入。**读取后两份时注意**：
它们是 Rust/Tauri 选型时期的产物，存储结构与数值已经复核过并可信，但
类型草案（§2.2.5 / §3.2.8 / §4）需按 Go 重写；§1.1 的存储根判定与本机
MSIX 实测不符，见前四份中的实测报告。

## 测试与验证

- 静态检查：`go vet ./...`
- 编译：`go build ./...`
- 测试：`go test ./... -count=1`
- 格式化：`gofmt -l cmd internal` 应为空

当前基线：18 个包全部通过（`sessions` 约 18s，`proc` 约 15s，全套约 50s）。
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

3. **Codex Desktop 与 CLI 共用 `~/.codex`**（与 Claude 的情况相反）。

4. **项目路径有三种表示**，不可从目录名反解，必须读会话文件内的 `cwd` 字段：
   - Claude：`D--Work-ExampleApp`（有损编码）
   - Codex：`\\?\D:\Work\ExampleApp`（扩展长度前缀）
   - ZCode：`D:\Work\ExampleProject`（普通路径）

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
    再复制 `frontend/dist`**，已固化在 `frontend/scripts/rebuild.ps1`。

11. **UI 自检截图必须满足两个条件**，否则看到的是假象：
    - **DPI 感知**（`SetProcessDPIAware()`）：否则 `GetWindowRect` 返回虚拟化
      坐标，截出来只有左上角一块，看着像「布局溢出」。
    - **抢到前台**（`SetForegroundWindow` 前先发一次 ALT 键解除系统限制，
      并轮询 `GetForegroundWindow` 确认）：否则抓到的是压在应用上的终端画面。
    - GDI 位图是 **BGRA**，不换通道蓝色会显示成橙色。WebView2 走
      DirectComposition，`PrintWindow` 抓不全，用屏幕 DC `BitBlt`。
    工具：`frontend/scripts/rebuild.ps1`（构建+重启+截图）、`uishot.py`、
    `crop.py`（放大局部比对）、`uiclick.py`（模拟点击）。

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

## 安全约定

- 只读访问上游存储，绝不 `DELETE` / `UPDATE` / `INSERT` 上游数据库
- 会话内容视为不可信输入：恢复参数（session-id、cwd）必须白名单校验
- 不得在日志、诊断包、提交中泄露凭据文件内容
  （`~/.claude/.credentials.json`、`~/.codex/auth.json` 等）
- 扫描根目录须有明确排除清单
