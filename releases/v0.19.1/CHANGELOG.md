# AgentDeck v0.19.1

发布日期：2026-11-13

## Fixed

- 导出的 Markdown 不再写入 NUL 等控制字符（保留换行、回车、制表符）。工具输出（如 PDF 文字提取）里常带 NUL，很多编辑器见到它会把整个文件当二进制或猜错编码，文件后半段就显示成乱码。标题、工具名、正文都会清理。
- 导出的 Markdown 文件开头加上 UTF-8 字节序标记（BOM）。一些 Windows 编辑器和查看器没有它会按 ANSI / GBK 猜编码，文件一长后半段就成了乱码。重新导出时既认得带 BOM 的导出文件，也认得旧版本导出的没有 BOM 的文件，都是原地更新，不会另存一份；更新后的文件带 BOM。
- PDF 文字提取常带出的 Wingdings / Symbol 私有区项目符号（U+F06C、U+F06E、U+F075、U+F076、U+F0A7、U+F0B7、U+F0D8、U+F0FC、U+F0E0）换成标准符号（● ■ ◆ ❖ ▪ • ➢ ✓ →）；不认识的私有区字符保持原样，不擅自删除。

## Improved

- 导出有明确反馈：单个导出时按钮显示进度并禁用，弹出“正在导出 Markdown”，完成后提示完整路径并带“复制路径”按钮，失败弹出错误；批量导出显示“正在导出 n / 总数”，完成后写明导出目录，全部失败时不再显示“已导出 0 个”，部分失败时写明成功与失败数量。

## Known Issues

- 原始会话里已经变成 `�` 的内容（例如控制台代码页造成的工具输出）无法恢复，导出如实保留。
- 与 v0.19.0 相同：恢复误删会话时账号名称要与当初导入时一致；推荐的项目名只是推测；“在 ChatGPT 中打开”的网址格式未逐项验证；浏览器检测与启动仅支持 Windows。
- 与 v0.17.0 相同：导出里图片文件名多为通用名、阅读区图片暂不支持点击放大、旧版导出的 Markdown 不被识别为同一会话、含大量图片的备份旧版（4 GiB 上限）无法恢复。
- 未在 macOS / Linux 上构建运行。

## 升级与回退

- 不改数据库结构与配置字段，首次启动无需迁移。
- 升级：在应用内“检查更新”，或用新 exe 手动替换。
- 回退：换回上一版 exe 即可，数据与配置不受影响。已导出的 Markdown 文件不会被改动；需要清理旧导出里的控制字符时，重新导出同一会话即可（会更新原文件）。

## Verification

- `gofmt -l cmd internal`：无输出；`go vet ./...`、`go build ./...`：通过。
- `go test ./... -count=1`：全部通过。新增 `TestWriteExportMarkdownDropsControlCharacters` 与 `TestExportMarkdownStartsWithBOMAndReexportReusesFile`（后者断言导出以 BOM 开头且只出现一次、重新导出复用原文件、无 BOM 的旧导出同样被更新）。前者：断言导出里没有 NUL、其他控制字符与已映射的私有区字符，保留换行与制表符、未知私有区字符，输出为合法 UTF-8，干净文本原样返回。开发中 `TestMoveCodexProvider` 与 `TestDroid` 出现过一次 Windows `rename … Access is denied` 的偶发失败，单独重跑通过，与本版改动无关。
- 前端：`tsc --noEmit` 通过；`npm test` 110 项通过；`npm run build` 与 `sync-assets.ps1`：通过。
- `govulncheck@v1.1.4`：No vulnerabilities found（退出码 0）；`npm audit`：0 vulnerabilities。
- 对一份真实的受影响导出文件做了检查：确认文件是合法 UTF-8，NUL 与私有区字符都位于“工具结果”折叠块内、约在文件 53% 之后。
- Windows x64 生产构建使用 `-trimpath`，工具链 go1.27.4；`vcs.revision` 等于下述构建提交，`vcs.modified=false`。
- 最终 EXE 的固定与字符串版本资源、应用清单、前端内嵌版本均为 0.19.1。

## 未执行

- 用常见的 Windows 编辑器（记事本等）实际打开带 BOM 的导出文件确认编码识别（只在测试里断言了文件头字节）。
- 导出进度提示、成功提示与失败提示在界面里的实际显示与点击（只做了类型检查与构建）。
- 用修复后的程序对那份受影响的原始会话重新导出，核对 NUL 已消失（原始会话文件不在本机）。
- 发布产物 exe 本身的启动验证：它与正在运行的实例共用单实例锁。
- 从 v0.19.0 走一遍完整的应用内更新流程。
- macOS / Linux 构建与运行；不同 DPI 的完整界面走查。

## Git

- 构建与 source/ 来源提交：1198ccc68b2347d61558c7bc11bbe86514f5b013。
- Tag：v0.19.1，指向单独的发布快照提交。
