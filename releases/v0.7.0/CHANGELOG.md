# AgentDeck v0.7.0

发布日期：2026-10-20

数据存储位置可整体迁移到其他磁盘（含迁移后对旧位置的显式清理），终端 PowerShell 可在
设置中选择，侧栏筛选改为默认单选、「全部会话」清除全部筛选，Hermes 会话里的飞书（Lark）
卡片正文可读；同时针对磁盘占用与检索路径做了永久会话库与全文索引的优化。

## Added

- **数据存储位置可迁移**（设置 → 数据与诊断 → 数据存储位置）：把 AgentDeck 自有数据整体
  迁到其他磁盘，缓解系统盘空间不足。迁移对三个数据库取在线一致快照（`VACUUM INTO`），
  其余数据文件逐个复制并校验 SHA-256，全部通过后才写入位置指针；目标目录必须为空，剩余
  空间按源数据总量 1.1 倍预检；中途失败只删除本次写出的副本，源目录一个字节不动。数据位置
  重启后生效，界面提供「立即重启」。旧位置一律保留，确认新位置可用后再用「清理旧位置数据」
  显式删除：预览清单绑定内容哈希，内容变化即整批拒绝；只删清单内的文件，不含
  `app-config.json`，不删非空目录，也不删用户自己的文件。位置指针保存在
  `%APPDATA%\AgentDeck\app-config.json`，该文件始终留在原地。
- **迁移目标固定为所选目录下的 `AgentDeck\Data`**（`D:\` → `D:\AgentDeck\Data`，
  `D:\software` → `D:\software\AgentDeck\Data`）：路径可预期，数据不与所选目录里已有的
  其他 AgentDeck 文件混在一层。所选目录本身就叫 `AgentDeck` 时只追加 `Data`，已经选到
  数据根本身时原样使用；只比末级目录名、不做前缀匹配（`D:\AgentDeckData` →
  `D:\AgentDeckData\AgentDeck\Data`）。补全规则由 Go 侧给出并直接展示在界面上，前端不
  自己拼路径。选择目录后、确认之前即显示补全后的数据位置与该目录的现状（已存在且为空可
  直接使用、已存在且非空给出文件数与占用并拒绝、不存在则迁移时创建）。
- **终端 PowerShell 可选**（设置 → 新建与启动）：自动（优先 7+）／Windows PowerShell 5.1
  ／PowerShell 7+／自定义可执行文件，并显示各选项检测到的版本；自定义路径须存在且能自报
  PowerShell 版本。默认仍为 Windows PowerShell 5.1，与既有行为一致。恢复会话与新建会话
  走同一条命令拼装路径，一处改动同时覆盖。
- **「压缩历史库」**：把升级前留下的明文正文重压一遍并回收文件空间，可重复执行；进度显示在
  按钮下方。

## Changed

- **侧栏筛选默认单选**：「全部会话」清除全部筛选，智能体、项目、收藏、归档、标签、月份、
  排序与搜索关键词一并复位，回到完整列表。其余入口单击任一智能体、项目、收藏或归档会替换
  整个筛选；按住 Ctrl（macOS 上为 Cmd）单击才把该维度并入当前筛选或取消它，再次单击唯一
  生效项即回到全集。列表标题如实写出组合条件（如「收藏 · Codex」「归档 · 某项目」）。
- **列表头的「全选」与批量条里的「清空」合并为一个图标**：没有选中项时是全选（Ctrl+A），
  已选中时是清空（Esc），选中一项时也有清空入口。快捷键不变。
- **迁移进度可见**：每个文件动手前先报一次（「正在生成 history.db 的一致副本」「正在复制 /
  正在校验 X」），此后每秒刷新一次字节进度与已用时间。三个库的 `VACUUM INTO` 与整库
  `integrity_check` 是单条阻塞调用，中途没有回调可挂，百分比是由目标文件现有大小估算的真实
  字节数；文件大小暂时不变时靠已用时间说明还在推进。进度行画在迁移弹窗内（此前画在被模态
  遮住的设置页里），阶段切换不再把百分比打回 0，也不带转圈图标——「正在动」由弹窗主按钮的
  Spinner 表示。迁移成功后弹窗就地切换为「数据迁移完成」，直接给出「稍后」与「立即重启」。
- **全文索引只留一份分词结果**：`docs_fts` 改为 FTS5 contentless（`content=''` +
  `contentless_delete=1`），不再在 `docs` 之外另存一份分词后的正文。索引库改用 WAL、
  `synchronous=NORMAL`、64 MiB 页缓存、256 MiB mmap、`temp_store=MEMORY` 与
  `busy_timeout=5s`——索引是随时可重建的派生物，按缓存对待。
- **索引写入路径去掉两处 30 万行级的全表扫描**：`SaveStamps` / `Remove` 原用
  `key = ? OR key LIKE ?` 判断会话是否还在索引里，而 LIKE 的右操作数是参数、又没有索引
  可用，默认设置下用不上主键，每个会话一次全表扫描；改为按 key 区间
  （`key = ? OR (key >= 'k#' AND key < 'k#\uFFFF')`）走主键范围扫描。逐会话写入时先分词
  再开事务，分词不再占着写入锁。索引库新增 `PRAGMA user_version` 标记（当前为 2），分词
  契约或表形态变更时据此升级或整库重建。
- **永久会话库正文按 zstd 存放**（小于 4 KiB 不压），正文与元数据都没变时不再重写——同一
  会话被反复读取是最常见的调用形态，此前每次都要在 `synchronous=FULL` 下等一次 fsync。

## Improved

- **本机真实库的实测数字**（本机 DPI 200%，数据根 `D:\AgentDeck\Data`）：永久
  会话库 680 个会话 / 941 个版本，正文 3606.4 MB → 1143.3 MB，文件 3.61 GB → 1.15 GB，
  压缩耗时 41 秒（含 `VACUUM` 共 66 秒）。体积最大的三行压缩前后解压比对逐字节一致，
  `PRAGMA integrity_check` 为 `ok`。
- 全文索引：本机 `index.db` 1931.2 MB → 1225.2 MB。旧索引在首次打开时就地升级——只重建
  FTS 表并从 `docs` 重新分词灌入，不重读会话文件，`doc_sources` 增量游标原样保留（本机
  1 分 39 秒，一次而已）；升级进度显示在「全文索引」一节。
- 读取端按 `codec` 列透明解压，升级前的删除备份与数据备份（没有该列）照旧可校验、可合并、
  可撤销删除；压缩逐行解压比对后才写回，原始内容不变。

## Fixed

- **修正升级与压缩尾部「先截断 WAL 再 VACUUM」的顺序**。WAL 模式下 VACUUM 是把整理后的
  整库写进 WAL、再由检查点搬回主库并截短主库文件的；检查点排在 VACUUM 前面，VACUUM 的
  一整份输出就留在 WAL 文件里——磁盘上等于「压缩后的主库 + 一整份 VACUUM 输出」，要等
  进程干净退出才随 WAL 一起消失。本机升级后 `index.db`（1225.2 MB）旁边就挂着
  1.1 GB 的 `index.db-wal`，加上历史库压缩后留下的同类残留，共约 2.3 GB 空间没有真正还回去。
  两处顺序调换后，升级与压缩结束时 WAL 已被截短（实测 0 字节）。新增
  `TestUpgradeTightensTheWALItLeaves` 与 `TestCompactTightensTheWALItLeaves` 锁住这个顺序
  （把两行调回旧顺序，两条测试都会失败）。
- **修复 Hermes 会话中飞书（Lark）卡片正文读不出来**。Hermes 偶尔把卡片本身当消息内容写入
  `messages.content`——整个卡片（`header` + `body.elements`）或单个卡片元素。此前的提取只认
  `"\x00json:"` 前缀里的 `type:"text"` 分片，其余原样返回，于是回复显示成一串 `tag` /
  `lark_md` 的 JSON。现在按卡片形状展平成文本：标题、`div`/`markdown`/`note`、`column_set`
  的列、按钮与 `action`、`at` 提及、图片的替代文字都取其中的原文，`hr` 不留空行。展平是全有
  或全无——只要有一处元素不认识，整条就仍按原文显示；超过 64 KB 的内容不做识别（那是工具
  输出，不是卡片）。
- **修复 Hermes 会话里由工具调用发出的飞书卡片正文读不出来**。这类卡片不在消息内容里：行
  本身是智能体的说明文字，卡片只存在于调用参数中——`execute_code` 把载荷写成 Python 字面量
  （`card = {…}`，`True` / `False` / `None`），或 `terminal` 用 heredoc 把 JSON 交给
  `card_send.py`。此前这段正文只能在「过程记录」里当代码读。现在按形状在参数里认出卡片，把
  展平后的正文作为一条「飞书卡片」紧跟在该调用之后显示，正文因此可读、可检索、可导出；参数
  可能是一层 JSON 字符串（OpenAI 形状），两种编码都读。识别用 JSON 解码而不是数引号。仍全有
  或全无：不认识的元素、被上游截断（`...[truncated]`）而无法闭合的字面量都保持原样；Python
  单引号写法的卡片不识别（本机 107 条卡片字面量里 1 条）。
- **修复改回默认数据位置后旧位置清单残留默认目录、使「清理旧位置数据」整批被拒的问题**。
  清理同时删除索引的旁文件（`index.db-wal` / `-shm` / `-journal`），旧位置只剩 AgentDeck
  数据时连空目录一并收掉。

## Known Issues

- 迁移完成后、重启之前的写入仍落在旧位置。迁移期间界面进入忙态并在完成后立即引导重启，
  但这一窗口客观存在；如需在迁移后继续整理会话，先重启再操作。
- 数据位置变更只在重启后生效：本版不做运行中热切换。
- 索引库运行中会保留一段 WAL 文件（本机实测可达数百 MB），干净退出时随 WAL 一起释放；
  异常退出后下次打开由 SQLite 自行恢复。
- 压缩历史库会同时占用一份等大的临时空间用于 `VACUUM`；空间不足时正文已压好、库仍可用，
  只是文件暂时偏大，界面会如实报出错误。
- Python 单引号写法（`card = {'tag': …}`）的飞书卡片不识别，仍按原文显示代码。
- 项目显示名称只影响 AgentDeck 的项目列表；会话记录中的项目字段仍按目录名归组。
- 新建会话仍是打开终端请求启动 CLI，产生记录后需要刷新列表才能看到。

## Verification

- 静态检查与编译：`gofmt -l cmd internal` 无输出，`go vet ./...`、`go build ./...` 通过。
- Go 测试：`go test ./... -count=1` 通过（26 个包）。期间观察到两次偶发失败
  （`internal/sessions` 的 `TestDeleteActiveRefused`、`internal/settings` 的
  `TestSaveConcurrent`，都是并行负载下的文件锁 / 改名时序问题，落在本轮未改动的路径上）：
  单包重跑 3 次与随后两次全量均通过，未能稳定复现。
- 前端：`npm test` 39 项通过；`npm run build`（tsc + vite）通过。
- 版本资源：`wails3 generate syso` 重新生成 `cmd/agentdeck-desktop/resource_windows_amd64.syso`。
  产物 ProductVersion `0.7.0`、FileVersion `0.7.0.0`、嵌入清单 `0.7.0.0`；`0.6.0` 在
  exe 内已无残留。
- 构建溯源：`vcs.revision=734fc8571477a48da6cc8b85a6d48c04ee24ba18`、`vcs.modified=false`。
- 真实规模迁移实测：本机 4.6 GB 数据迁至 `D:\AgentDeckData`，重启后会话 389、收藏 8、
  项目声明 132 与迁移前一致，逐表键集合比对确认源数据一行不缺；清理按预览删除 5 个文件
  4.4 GiB 并收掉空目录，`app-config.json` 与默认位置的原始数据均未被改动。
- 真实规模压缩实测：在本机真实 `history.db`（3.61 GB）上跑一次压缩，结果见「Improved」，
  抽样三行逐字节一致、`integrity_check` 为 `ok`；索引库升级后 `docs_fts_content` 影子表
  消失、`MATCH` 命中与升级前一致、`doc_sources` 完整。
- 新增单测：`TestUpgradeTightensTheWALItLeaves`、`TestCompactTightensTheWALItLeaves`（两者
  都在旧顺序下失败）、`TestSaveCompressesLargeTranscripts`、`TestSaveKeepsSmallTranscriptsPlain`、
  `TestSaveSkipsUnchangedWrites`、`TestSaveRewritesWhenSessionMetadataChanges`、
  `TestCompactCompressesLegacyRows`、`TestOpenAddsCodecColumnToLegacyDatabase`、
  `TestOpenUpgradesContentlessIndexInPlace`、`TestContentlessIndexReplacesAndRemoves`、
  `TestSaveStampsIgnoresSessionsThatOnlyShareThePrefix`、`TestSearchStillUsesBM25AfterUpgrade`、
  `TestMigrateEmitsProgress`、`TestMigrationProgressHeartbeat`、`TestHermesTextFlattensFeishuCard`、
  `TestHermesCardText`、`TestHermesCallCardTextFromExecuteCode`、`TestHermesCallCardTextFromHeredoc`、
  `TestHermesCallCardText`、`TestHermesPartsAddsTheCardACallSent`、
  `TestHermesPartsKeepsCallsWithoutCards`。
- 终端实测：Windows PowerShell 5.1 与 PowerShell 7.6.7 下各起一次，含中文与含空格的工作目录。
- 界面走查：生产构建上截图核对「数据存储位置」一节（含补全规则的说明文字）、迁移弹窗内的
  进度态（阶段、文件计数、百分比、当前文件说明与已用时间五项同时出现并逐秒变化）与列表头部
  合并后的选择图标；飞书卡片在重建后的应用里按会话打开核对（会话「询问AI助手名称」首轮回复
  显示为两段正文，会话「主模型切换完成通知」末尾那张 `execute_code` 发出的卡片显示为一条
  「飞书卡片」）。
- 只读复核：未触及删除会话路径的不变量；新增的飞书卡片解析对上游只读；`go.mod` 与
  `frontend/package.json` 依赖未变，无新增第三方依赖（zstd 与 FTS5 都已在使用中）。

## 未执行

- **「压缩历史库」按钮与索引升级进度行的界面走查未执行**：本机这一轮截图抓不到 WebView2
  的内容区（屏幕 DC `BitBlt` 抓到的窗口内容区为空白，`PrintWindow` 同样为空），改为在服务层
  对本机真实库跑同一条代码路径并核对结果（见上）。按钮的点击链路未经人工点击确认。
- 迁移目标「不存在则创建」分支的确认弹窗未走查——原生目录选择器无法脚本化，且 WebView2 的
  运行库调用走 `chrome.webview.postMessage` 而非 `fetch`，替不到选择器请求（该分支的判定与
  文案由 `ResolveDestination` 单元测试覆盖，非空目录的拒绝弹窗已在真实选择下核对）。
- 「数据迁移完成」态与「稍后 / 立即重启」两个按钮未做界面走查（要让它们出现必须让一次真实
  迁移跑完并改写当前数据位置），改由两项单测与 `tsc` 覆盖。
- 侧栏「默认单选 / Ctrl+单击叠加」的手工点击走查未执行（自动化点击与正在操作窗口的用户冲突）。
- 删除备份与恢复前备份目录在本机为空，其真实数据的迁移未实测（单元测试覆盖）。

## Git

- 源码提交：`734fc85`（feat: 飞书卡片正文可读、永久库与全文索引优化，准备 v0.7.0）
- Tag：`v0.7.0`

## Upgrade

- 覆盖 exe 即可。数据库 schema 有向后兼容的增补：永久会话库新增 `codec` 列（旧行按明文读，
  写回时才压缩），索引库新增 `PRAGMA user_version` 标记（旧索引首次打开就地重建 FTS 表）。
  标注、项目声明、会话正文与界面偏好原样保留。
- 升级后想立刻收回磁盘空间：`index.db` 的旧形态在首次打开时自动升级并 `VACUUM`；永久会话库
  的旧明文正文在「设置 → 数据与诊断 → 压缩历史库」点一次即可（本机 3.61 GB → 1.15 GB）。
- 原 v0.6.0 及更早版本留下的数据位置不变；需要换盘时用「设置 → 数据与诊断 → 数据存储位置」。