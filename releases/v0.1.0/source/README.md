# AgentDeck

跨 Agent 的历史会话检索与恢复工作台。

统一索引 Claude Code、Claude Desktop、Codex 等多个 AI Coding Agent 的历史会话，
做「找到当时的工作 → 看清做到哪一步 → 在正确工具和目录里继续」。

## 定位

AgentDeck 是**历史检索与导航层**，不是又一个 Agent 客户端。它不重新实现
执行内核、不绕过登录限制、不修改上游数据库、不上传会话。

与 [magpie](https://github.com/yetone/magpie) 互补：magpie 解决「模型怎么选」，
AgentDeck 解决「当时做了什么、怎么接着做」。

## 当前状态

**Phase 0 — 会话读取基线**（可编译、可运行）

已实现：

- 会话发现与索引（Claude Code / Claude Desktop / Codex / OpenCode / ZCode 等）
- 会话列表、查找、原生标题
- 恢复命令构造
- 子代理归属（`role` / `parent_session` / `root_session`）
- Token 与成本统计（含逐模型明细）
- 会话删除 / 回收站 / 恢复

待实现：

- 全文检索（含中文分词）
- 项目维度聚合
- 收藏 / 标签 / 备注
- 桌面 UI

## 架构

```
cmd/agentdeck/       程序入口
internal/sessions/   会话发现、解析、列表、恢复（核心）
internal/catalog/    模型目录与定价数据
internal/redact/     脱敏
internal/settings/   设置
internal/desktopdir/ Claude Desktop 目录发现（含 MSIX 打包路径）
internal/provider/   最小 provider 替代（只读打开 + 基础定价）
tools/recon/         侦察与验证脚本
docs/                设计文档与实测报告
```

会话层不依赖任何桌面框架，可独立编译与测试。

## 构建

要求 Go 1.26.8 或更高。

```sh
go build ./...
go run ./cmd/agentdeck
```

## 数据来源

AgentDeck 只读访问各 Agent 的本地存储，不写入：

| Agent | 存储位置 |
| --- | --- |
| Claude Code CLI | `~/.claude/projects/<编码目录>/*.jsonl` |
| Claude Desktop | `%LOCALAPPDATA%\Packages\Claude_<pub>\LocalCache\Roaming\Claude\local-agent-mode-sessions\...\.claude\projects\` |
| Codex | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` + `state_5.sqlite` |
| OpenCode | `~/.local/share/opencode/opencode.db` |
| ZCode | `~/.zcode/cli/db/db.sqlite` |

## 已知限制

- Claude Code 默认 `cleanupPeriodDays=30`，主会话转录会被上游自动清理；
  AgentDeck 尚未做持久化保留。
- Claude Desktop 会话无原生恢复入口。
- 移植自 magpie 的代码为一次性搬运，不跟随上游更新。

## 许可

MIT。包含移植自 [magpie](https://github.com/yetone/magpie) 的代码
（MIT, Copyright (c) 2026 yetone），详见 [NOTICE.md](NOTICE.md)。
