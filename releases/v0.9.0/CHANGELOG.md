# AgentDeck v0.9.0

发布日期：2026-10-21。平台：Windows x64。

首个公开发布的版本。这一版没有新增功能，全部改动都是为了满足公开仓库的前提：
移除内部设计资料与第三方品牌位图，补齐第三方许可声明，构建不再内嵌构建机路径，
并重写 Git 历史清除本机标识。

## 移除

- **产品设计资料不再随仓库分发**：产品设计评审、适配器实现规格、magpie 复用可行性分析、
  侦察结论复核、四目标存储与恢复实测、移植可行性实验报告、应用图标方案，共 7 份文档
  从当前树与全部历史提交中移除，只保留在开发机本地。
- **侦察与实验脚本**：`tools/recon/` 下 9 个 Python 脚本与 `tools/gen_agenticons.ps1`
  一并移除；这些脚本记录的是内部实测口径，且硬编码了本机路径。
- **7 个从第三方产品安装包提取的品牌位图**：`claude.png`、`claude-desktop.png`、
  `codex.png`、`hermes.png`、`qoder-cn.png`、`workbuddy.png`、`zcode.png` 及其来源记录
  `codex-source.json`。其中前三个在 `agents.ts` 里已被 SVG 取代，属死资源。

## 变更

- **Codex 图标改用 Lobe Icons 的 SVG**（MIT），与其余 11 个品牌图标同源；
  ZCode、Hermes、WorkBuddy 没有可再分发的开源图标，回退到 Fluent 终端图标并保留
  可访问名称。图标库许可与自绘边界写在 `frontend/public/agents/README.md`。
- **`LICENSE` 恢复为标准 MIT 文本**：此前在 MIT 正文后追加了 `---` 与第三方归属段，
  导致 GitHub 把仓库许可识别为 `Other / NOASSERTION`。第三方归属统一放进 `NOTICE.md`。
- **构建加入 `-trimpath`**（`Taskfile.yml`、`frontend/scripts/rebuild.ps1`、README 的
  分步构建示例）：v0.8.0 的 exe 里实测有 403 处构建机绝对路径，现在为 0。
- **`NOTICE.md` 重写**：补充 Lobe Icons、Go 依赖的实际链接口径、前端依赖、models.dev
  测试数据来源与商标声明；修正了测试包数量等过时表述。
- 测试、脚本与配置中的本机路径、真实会话标识与私有项目名统一替换为示例值。
- `frontend/package.json` 补 `license` 字段。

## 新增

- **`THIRD-PARTY-NOTICES.txt`**：21 个实际链接进桌面程序的 Go 模块与 191 个前端生产
  依赖的许可全文，由 `tools/gen_third_party_notices.py` 生成（只读本地模块缓存与
  `node_modules`，不联网）。
- **分发 ZIP 随附许可文件**：除 EXE、CHANGELOG 与内层 SHA256SUMS 外，新增 `LICENSE`、
  `NOTICE.md`、`THIRD-PARTY-NOTICES.txt`，满足 MIT / BSD-3-Clause / Apache-2.0 / ISC
  的随附声明义务。
- README 新增「下载与安装」与「隐私与网络访问」两节：写明产物校验方式、未签名提示、
  不读取账号凭据、以及仅向 `models.dev` 与 `aihubmix.com` 发起只读 GET。

## 历史重写

- 当前仓库是重写后的新历史：78 个提交 + 1 个新提交，14 个 tag 全部指向重写后的对象，
  提交与 tag 的作者/打标签者统一为本项目的 GitHub noreply 地址。
- 移除了历史中全部 `docs/` 与上述位图；构建机绝对路径、本机用户名、内部项目名、
  真实会话 UUID 在历史 diff 中已零命中。
- 移除了 70 个提交里标注 AI 协作的 trailer。
- v0.1.0–v0.8.0 的 GitHub Release 与资产不再提供，公开仓库从 v0.9.0 开始。

## 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...`、`go build ./...` 通过。
- `go test ./... -count=1` 通过：25 个含测试的包全部通过，2 个包无测试文件。
- `npm test` 49 项通过；`npx tsc --noEmit` 无报错；`npm run build` 通过。
- 历史扫描：`git log -p --all` 对本机用户名、构建机路径、内部项目名、两个真实会话 UUID、
  作者个人邮箱均零命中；AI 协作 trailer 零命中；`docs/`、品牌 PNG、`tools/recon/`
  在全部历史路径中零命中。
- 产物核对：`go version -m` 的 `-trimpath=true`、`vcs.revision=d3ba0ab`、
  `vcs.modified=false`；exe 内本机用户名、构建机路径与用户目录前缀均为 0 次；
  ProductVersion `0.9.0`、FileVersion `0.9.0.0`、嵌入清单 `0.9.0.0`。
- 运行验证：本机构建产物启动后窗口正常，会话列表、正文、侧栏智能体与项目区渲染正常，
  图标兜底生效（ZCode / Hermes / WorkBuddy 显示通用图标）。

## 未执行

- **未做代码签名**：产物仍为未签名 exe，Windows SmartScreen 会提示未知发布者。
- **未做真实 Windows 开机登录、托盘点击、多 DPI 走查**（同 v0.8.0）。
- **未对真实用户数据执行备份恢复或迁移**。
- **未在 Windows 之外的平台构建或运行验证**。
- **未做商标检索或法务级评估**；品牌名称仅作指示性使用。
- 历史重写前的完整仓库保存在开发机 `_Backup/` 的 Git bundle 中，未随仓库分发。

## 产物与追溯

- `AgentDeck-v0.9.0-win-x64.exe`（Windows x64）
- `AgentDeck-v0.9.0-win-x64.zip`（EXE、CHANGELOG、LICENSE、NOTICE、第三方许可、内层校验值）
- `AgentDeck-v0.9.0-source.zip`（完整源码快照）
- `SHA256SUMS`（上述三份产物的 SHA-256）
- 源码冻结提交与 Tag：见本文件末尾的 Git 小节与仓库 tag `v0.9.0`。

## 许可

MIT。包含移植自 [magpie](https://github.com/yetone/magpie) 的代码
（MIT, Copyright (c) 2026 yetone）与 [Lobe Icons](https://github.com/lobehub/lobe-icons)
的图标（MIT, Copyright (c) 2023 LobeHub）。
