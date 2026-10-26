# AgentDeck v0.15.3

发布日期：2026-10-26

## Fixed / Improved

- 搜索结果携带原文偏移，超长段落从命中附近加载，避免深处命中只跳到段首或未载入。
- 跨语法高亮节点、连续空白与代码行的关键词可正确高亮；代码块内部与正文依次滚动，让当前命中可见。
- 刷新正文后恢复阅读锚点与段内偏移；显式导航优先，旧的异步定位不再消费新的跳转意图。
- 阅读区与代码复制的成功、失败均有反馈。Markdown 导出后显示保存路径并可复制；取消目录选择不报错，失败时显示原因。

## 安装与升级

- Windows x64，需 Microsoft Edge WebView2 Runtime；绿色版 EXE 可直接运行，ZIP 另附变更与许可文件。
- v0.15.2 用户可在「设置 → 更新」升级。v0.15.0 / v0.15.1 需手动下载一次新版 EXE，保留原有数据与设置。
- 产物未做代码签名；运行前可按 SHA256SUMS 校验。

## Known Issues

- 会话内查找按原始 Markdown 文本的子串匹配；段内 8000 字符窗口位于中部时可能缺少围栏而影响排版。
- 本轮未执行真机导出对话框操作与复制失败场景；此前隔离 UI 已验证深处命中、跨代码节点 const value 查找与同段刷新，不能据此推定全部交互均已实测。
- macOS / Linux 构建与运行、不同 DPI 完整走查未执行。

## Verification

- gofmt -l cmd internal：无输出；go vet ./...、go build ./...：通过。
- go test ./... -count=1：28 个测试包通过，另 2 个包无测试文件。
- npm test：106 项通过；npm run build 与资源同步：通过。
- govulncheck@v1.1.4：No vulnerabilities found；npm audit：0 vulnerabilities。
- Windows x64 生产构建使用 -trimpath；来源为下述构建提交，vcs.modified=false。
- 前端工具链有 EnvHttpProxyAgent 实验性警告；Git 有 LF/CRLF 转换提示，未屏蔽。
- 最终 EXE 校验固定与字符串版本资源、应用清单和前端内嵌版本。

## Git

- 构建与 source/ 来源提交：201d55e93f1752a9f01ec5698d99d3961a47e5c9。
- Tag：v0.15.3，指向单独的发布快照提交。
