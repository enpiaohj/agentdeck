# NOTICE

本文件记录 AgentDeck 中包含的第三方代码、数据与品牌资源及其许可。
发行包内同时附带 `LICENSE` 与本文件；完整的依赖许可文本见
`THIRD-PARTY-NOTICES.txt`。

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
作为「移植后行为不变」的回归资产，随各个包一起运行。

按本项目情况调整过的测试在代码内注明原因，详见 `CLAUDE.md` 的
「测试与验证」一节。

## Lobe Icons

- **来源**：<https://github.com/lobehub/lobe-icons>
- **许可**：MIT License
- **版权**：Copyright (c) 2023 LobeHub
- **取用版本**：commit `8a1b2821bb768c484fe8da20c5e1838b6bf551a9`
- **取用方式**：复制 `packages/static-svg/icons/` 下的 SVG 到
  `frontend/public/agents/`，保留原始内容；许可证全文见
  `frontend/public/agents/LOBE-ICONS-LICENSE`

这些 SVG 是开源图标库提供的品牌资源。品牌名称与标识的商标归各自权利人所有；
本项目仅作指示性使用，不表示任何形式的背书或授权代言。仓库不包含从第三方
产品安装包中提取的位图。

## Go 依赖

`go.mod` 中实际链接进桌面程序的模块全部为宽松许可（MIT、BSD-3-Clause、
Apache-2.0、ISC），无 CGO 依赖。逐项许可文本见 `THIRD-PARTY-NOTICES.txt`。

其中 `gopkg.in/yaml.v3` 附带 Canonical 的 NOTICE（Copyright 2011-2016
Canonical Ltd.）；`modernc.org/*` 为 BSD-3-Clause，`golang.org/x/*` 为
BSD-3-Clause，`github.com/klauspost/compress` 为 BSD-3-Clause。

## 前端依赖

界面使用 React、Fluent UI（`@fluentui/react-components`）、Vite 等，均为
MIT、ISC、Apache-2.0、BSD-3-Clause 或 0BSD 许可，无 copyleft 依赖。
随应用打包的依赖许可文本见 `THIRD-PARTY-NOTICES.txt`。

## 数据

`internal/catalog/testdata/models.dev-*.json` 是
<https://models.dev/api.json> 的公开数据集快照，仅作为测试夹具使用，
不随安装包分发，也不用于联网请求。

## 商标声明

Claude、Claude Code、Claude Desktop、OpenAI、Codex、Gemini、Cursor、Kimi、
Qoder、ZCode、WorkBuddy、Hermes 等名称与标识归各自权利人所有。AgentDeck
与这些项目没有隶属或赞助关系，相关名称仅用于说明兼容的会话来源。
