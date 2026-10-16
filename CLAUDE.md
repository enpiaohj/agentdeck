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

## 测试与验证

- 静态检查：`go vet ./...`
- 编译：`go build ./...`
- 测试：`go test ./...`（移植的测试文件尚未引入）
- 格式化：`gofmt -l .` 应为空

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

5. **中文 FTS5 检索**：`unicode61` 与 `trigram` 对 2 字中文词均失效，
   需 bigram 预分词或自注册 tokenizer。

6. **控制台代码页是 cp949**（区域 zh-CN）。向 stdout 写中文的 Go 程序需先调
   `SetConsoleOutputCP(65001)`，Python 子进程需 `sys.stdout.reconfigure(encoding='utf-8')`。

## 安全约定

- 只读访问上游存储，绝不 `DELETE` / `UPDATE` / `INSERT` 上游数据库
- 会话内容视为不可信输入：恢复参数（session-id、cwd）必须白名单校验
- 不得在日志、诊断包、提交中泄露凭据文件内容
  （`~/.claude/.credentials.json`、`~/.codex/auth.json` 等）
- 扫描根目录须有明确排除清单
