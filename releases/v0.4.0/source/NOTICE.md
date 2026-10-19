# NOTICE

本文件记录 AgentDeck 中包含的第三方代码及其许可。

## magpie

- **来源**：<https://github.com/yetone/magpie>
- **许可**：MIT License
- **版权**：Copyright (c) 2026 yetone
- **取用版本**：main 分支，commit `63be477`（2026-10-16）
- **取用方式**：源码移植（非依赖引用），模块路径已由 `github.com/yetone/magpie`
  改为 `github.com/enpiaohj/agentdeck`

### 移植范围

`internal/` 下以下包的全部或部分内容来自 magpie：

| 包 | 说明 |
| --- | --- |
| `internal/sessions` | 会话发现、解析、列表、恢复命令构造、子代理归属 |
| `internal/edit` | 配置文件编辑 |
| `internal/catalog` | 模型目录与定价数据 |
| `internal/proc` | 进程与可执行文件定位 |
| `internal/redact` | 敏感信息脱敏 |
| `internal/settings` | 设置读写 |
| `internal/appdir` | 各 Agent 的数据目录解析 |
| `internal/desktopdir` | Claude Desktop（含 MSIX 打包）目录发现 |
| `internal/agentenv` | Agent 环境变量 |
| `internal/dirent` | 目录项判断 |
| `internal/filelock` | 文件锁 |
| `internal/filememo` | 文件记忆 |
| `internal/fonts` | 字体相关 |
| `internal/lastgood` | 最后可用状态 |
| `internal/sqlitedb` | SQLite 辅助 |
| `internal/steady` | 稳定化辅助 |
| `internal/testenv` | 测试环境 |

### 未移植

- `internal/provider`（58,948 行）：magpie 的模型路由与定价体系。AgentDeck
  仅自写最小替代（`internal/provider/provider.go`，约 130 行），覆盖会话层
  实际用到的 7 个符号，外加移植中补上的 `SetModelPrice` 与 effort/window
  后缀剥离。
- `internal/gui`、`internal/gateway`、`internal/plugin`、`internal/update`、
  `internal/agent`、`internal/library`、`internal/backup`、`internal/middleware`
  等模型路由网关与 Agent 管理相关包。

### 测试

上游的 `*_test.go` 与 `testdata/` 一并移植（约 16,000 行测试 + 126 个夹具），
作为「移植后行为不变」的回归资产。当前 17 个包全部通过。

按本项目情况调整过 4 处，均在代码内注明原因，详见 `CLAUDE.md` 的
「测试与验证」一节。

## Go 依赖

见 `go.mod`。全部为纯 Go 实现，无 CGO 依赖。
