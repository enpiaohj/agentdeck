# AgentDeck v0.6.0

发布日期：2026-10-19

新增 DeepSeek Harness（dsh）会话正文读取与新建入口，统一新建 / 重命名弹窗与空状态，
把顶栏提示条换成 Toast，并修正侧栏计数口径与原生文件对话框「取消被当成失败」的报错。

## Added

- **DeepSeek Harness（dsh）正文可读**：新增 `internal/sessions/transcript_dsh.go`，按事件写入
  顺序读取会话自身的 `session.v<N>.jsonl[.zstd]`（format 4 起为 zstd），用户输入、模型回复
  （思考 / 正文 / 工具调用）与工具结果分块呈现；fork 会话从其被复制的历史读起。此前 dsh
  不在 `HasTranscript` 名单里，正文、预览与 Markdown 导出都不可用。
- **dsh 加入可启动清单**：「新建会话」平台列表新增 `DeepSeek Harness CLI`（命令 `dsh`），
  `internal/app/resumectl.go` 的 `cliAgents` 与前端 `CLI_AGENTS` 同步。续会话命令形态没有可靠
  依据，恢复菜单对它保持置灰并标注「仅可查看历史」。
- **dsh 品牌标识**：展示名由 `dsh` 改为 `DeepSeek Harness`（此前无图标），图标改用
  Lobe Icons 的 `deepseek-color.svg`（MIT，pin 到提交 `8a1b2821…`），品牌色 `#4D6BFE`；
  来源与许可见 `frontend/public/agents/README.md`。
- **Toast 反馈**：右下角提示，最多三条、可关闭，支持「撤销」动作；顶栏常驻的导入提示条移除，
  导入结果改由 Toast 给出。
- **侧栏计数**：「全部会话 / 收藏 / 归档」三行显示计数（`SessionService.Counts`），口径与列表
  一致；智能体与项目计数不再包含归档会话。
- **导航宽度可调**：侧栏与列表之间的分隔条可拖拽，方向键微调，双击恢复默认宽度。
- **未检测到 CLI 的提示**：智能体选择器对不在 PATH 上的命令标注「未检测到 xxx」；仅提示，不阻止使用。
- **快捷键**：`Ctrl+N` 新建会话、`Ctrl+Shift+N` 新建项目，设置页快捷键表同步更新。
- 新建会话对话框可展开「将执行的命令」并一键复制；列出「最近使用」目录可直接点选。
- 新建项目对话框新增目录即时校验（已存在 / 将创建 / 被文件占用 / 已在项目列表）与「使用目录名」回填。

## Changed

- **弹窗统一**（`FormDialog`）：新建会话、新建项目与项目重命名共用同一外壳，具备忙态、就地错误、
  Enter 提交、关闭后焦点归还；新建会话入口统一由 App 持有（顶栏按钮 / `Ctrl+N` / 右键菜单 /
  项目行「＋」走同一路径）。
- **空状态统一**（`EmptyState`）：图标 + 结论 + 下一步。
- **项目重命名**：改名后提供「撤销」，会话列表与详情中的项目名跟随显示名称；项目行悬停或聚焦时
  出现「＋」，可直接在该项目新建会话，悬停提示补充归档条数、默认智能体与右键提示。
- **不可读正文的会话**：正文区给出说明并禁用「导出为 Markdown」，而不是点击后无反馈。
- README 发布状态行由 v0.2.2 更正为 v0.6.0，数据来源表补 DeepSeek Harness 的存储位置
  （`~/.dsh/sessions/<编码目录>/<会话>/session.v<N>.jsonl[.zstd]`）。

## Fixed

- **取消原生文件对话框被当成失败**：Windows 上 wails 对取消不返回空值，而是抛出
  `Invalid dialog call: Dialog.OpenFile failed, error getting selection: cancelled by user`，
  「浏览…」取消后会弹出这条英文报错。新增 `frontend/src/nativeDialogs.ts` 的 `pickOpenPath`，
  按文本识别取消并返回空串，8 处调用点（新建会话、新建项目、完整备份、从备份恢复、Claude 导入、
  信息面板导出、正文区导出、列表批量导出）全部改走该封装。
- **导出 Markdown 失败静默**：信息面板的导出失败此前被 `.catch(() => {})` 吞掉，现给出错误文本。
- **「全部会话」计数与列表不符**：此前按各智能体计数相加且包含归档，点进去行数对不上；现按
  `Counts` 口径统计，智能体计数同样不再把归档算进去。
- `usePreferences` 的 `update` / `reset` 改用 `useCallback`，依赖它的事件 effect 不再反复重建。

## Known Issues

- dsh 只接「新建会话」，不提供续会话恢复：其续会话命令形态没有可靠依据，不猜测着实现。
- 项目显示名称只影响 AgentDeck 的项目列表；会话记录中的项目字段仍按目录名归组。
- 新建会话仍是打开终端请求启动 CLI，产生记录后需要刷新列表才能看到。
- 深色模式沿用主题变量改动，本次未单独做逐项视觉走查。

## Verification

- 静态检查与编译：`go vet ./...`、`go build ./...` 通过；`gofmt -l cmd internal` 为空。
- Go 测试：`go test ./... -count=1` 全部通过（23 个包有测试，另 3 个包无测试文件）。
- 前端：`npm --prefix frontend test` 35 项全部通过；`npm --prefix frontend run build`（tsc + vite）通过。
- 版本资源：`wails3 generate syso` 重新生成 `cmd/agentdeck-desktop/resource_windows_amd64.syso`
  （版本号改动后必须重跑，`go build` 不会自行重生成）。产物 ProductVersion `0.6.0`、
  FileVersion `0.6.0.0`、固定信息块 `0.6.0.0`、嵌入清单 `0.6.0.0`。
- 构建溯源：`vcs.revision=34e6896f4e05f3d936214f7d02ad9eda44da3157`、`vcs.modified=false`。
- 只读复核（逐文件核查本次改动）：未触及删除会话路径（`internal/sessions/native_delete*.go`、
  `internal/app/delete_service.go` 无改动）；新增的 dsh 读取链路对 `~/.dsh` 只读
  （`scanAt` → `openLines`，无写入分支）；`go.mod` 与 `frontend/package.json` 依赖未变，无新增第三方依赖。
- 界面实测（Windows 11、本机 DPI 200%）：侧栏「全部会话 389 / 收藏 8 / 归档 0」与 `marks.db`
  （8 条收藏、0 条归档）一致；dsh 会话详情读出正文与分块统计（245 段，含工具调用 27、思考 19）；
  dsh 图标加载正常、侧栏显示「DeepSeek Harness 1」；新建会话平台列表出现
  「DeepSeek Harness CLI（未检测到 dsh）」。
- **未执行**：真实点击「浏览… → 取消」的人工走查（取消路径由 `frontend/tests/nativeDialogs.test.mjs`
  以生产环境原始错误文本锁定，未在 GUI 里手点）；真实 CLI 端到端启动（不代用户启动 agent 会话）；
  深色模式与 125% / 150% DPI 逐项视觉走查；Toast「撤销」的真实交互走查。

## Git

- 源码提交：`34e6896`（feat: DSH 会话正文与新建支持，统一弹窗与反馈，准备 v0.6.0）
- Tag：`v0.6.0`

## Upgrade

- 覆盖 exe 即可。数据库 schema 未变更（`marks.db`、`projects.db`、`history.db` 与全文索引库均不变），
  标注、项目声明与界面偏好原样保留。
- dsh 会话此前已在列表里，升级后正文可读；正文按需读取，无需迁移或重建索引。