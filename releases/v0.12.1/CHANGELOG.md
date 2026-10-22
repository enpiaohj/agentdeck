# AgentDeck v0.12.1

发布日期：2026-10-22。平台：Windows x64。版本资源：ProductVersion `0.12.1`，FileVersion / 应用清单 `0.12.1.0`。

安全补丁。功能与 v0.12.0 相同，建议所有用户升级。

## 安全修复

- **`github.com/klauspost/compress` 1.18.3 → 1.18.7**：修复 s2 包的整数溢出与越界写（GitHub
  Dependabot 高危告警）。s2 会编译进程序（`snappy` 包基于它实现），用于解析 Claude Desktop 本机
  缓存中的 snappy 数据，属于不可信输入。
- **构建工具链固定为 Go 1.27.3**：`go.mod` 新增 `toolchain go1.27.3`。govulncheck 在 Go 1.27.0 上
  报告 10 项会被实际调用的标准库漏洞，均在 1.27.2 / 1.27.3 中修复：
  - GO-2026-6809 `crypto/x509`：不再信任已泄露的中间证书
  - GO-2026-6607 `crypto/tls`：拒绝格式错误的 ECH 外层扩展引用
  - GO-2026-6603 / 6610 / 6611 / 6617 `net/http` HTTP/2：Trailer 内存耗尽、畸形帧头、窗口更新
    CPU 消耗、HPACK 编码器竞争
  - GO-2026-6605 / 6609 / 6613 `net/http`：CONNECT 后的连接错位、Range 头大小限制
  - GO-2026-6608 `mime/multipart`：MIME 头解析的内存限制绕过

  在线更新查询 GitHub 与下载更新包的 HTTPS 请求会走到 `crypto/x509`、`crypto/tls` 与 `net/http`
  客户端路径。升级后 govulncheck 报告「No vulnerabilities found」。
- **开发依赖 vite 5.4.21 → 6.4.5**（esbuild 0.21.5 → 0.25.12）：修复 Dependabot 报告的 9 项告警
  （开发服务器的路径穿越、跨源读取、`launch-editor` 命令参数与 UNC 路径处理等）。这些只影响本地
  开发服务器 `npm run dev`，不进入发布的程序；`@vitejs/plugin-react` 4.7.0 已支持 vite 6，`npm audit`
  报告 0 项。

## 关于 GitHub 上的其余告警

`releases/` 下每个已发布版本的源码快照都带一份 `go.mod` 与 `package-lock.json`，GitHub 会把它们当作
独立项目扫描，同样的问题会按快照数量重复报告。快照是不可变的发布记录，不参与构建；这些告警在
本版发布后统一驳回，针对快照目录的 Dependabot PR 关闭、不合并。

## 安装与升级

1. 下载 `AgentDeck-v0.12.1-win-x64.exe`，或包含程序与许可说明的 `AgentDeck-v0.12.1-win-x64.zip`；
   用 `SHA256SUMS` 校验下载文件。
2. 退出旧程序后，将新 EXE 放到固定位置并启动。已有数据与偏好继续使用。
3. 若移动了 EXE 且启用了开机启动，在新程序中关闭并重新启用该选项以更新路径。
4. 从源码构建：`go.mod` 固定 `toolchain go1.27.3`，本机 Go 版本较低时会自动下载该工具链（需要
   网络，或预先安装）。

## 验证

- `gofmt -l cmd internal` 无输出；`go vet ./...`、`go build ./...` 通过（Go 1.27.3）。
- `go test ./... -count=1`：27 个包全部通过（`sessions` 约 17s、`settings` 约 17s、`proc` 约 13s、
  `search` 约 11s）。
- `govulncheck`（v1.1.4）：升级前报告 10 项会被调用的标准库漏洞；升级后「No vulnerabilities found」。
- 前端 `npm test`：74 项通过；`tsc --noEmit`、`npm run build`（TypeScript + vite 6.4.5）通过；
  `sync-assets.ps1` 通过，前端包内嵌 `product_version:"0.12.1"`；`npm audit` 0 项。
- 前端命令输出 Node.js `EnvHttpProxyAgent` 实验性警告，未隐藏，未阻止构建。
- 重新生成 Windows syso，正式 EXE 的 ProductVersion 为 `0.12.1`、FileVersion 为 `0.12.1.0`、应用清单
  版本 `0.12.1.0`；EXE 内不含本机绝对路径与已移除的品牌图。
- 正式构建：`go version -m` 记录 `go1.27.3`、`klauspost/compress v1.18.7`、
  `vcs.revision=e277e09c306a4616b18a3fa5f86ebfd672637297`、`vcs.modified=false`、`CGO_ENABLED=0`、
  `GOOS=windows`、`GOARCH=amd64`；使用 `-trimpath`、`production` 标签、Windows GUI 子系统。
- **界面冒烟**（隔离演示用户目录 + 虚构会话，窗口 1280×800）：启动后标题栏为 v0.12.1，会话列表、
  阅读区与详情栏正常渲染（vite 6 打包产物）；删除对话框正常显示；设置搜索「清理旧版本」能找到
  本地会话库一节（v0.12.0 修复项，本版完成界面复测）。未读取真实会话，未操作开机启动注册项。

## 已知问题与未执行

- 未做代码签名，Windows SmartScreen 可能显示未知发布者；SHA-256 校验仅保证与发布清单一致。
- 本版只做了界面冒烟，未重复 v0.12.0 的完整界面实测；功能代码与 v0.12.0 相同，差异只在依赖、
  工具链与前端打包器。
- Claude Desktop 缓存解析（使用 s2 / snappy 的路径）只由现有单元测试覆盖，未用真实 Desktop 缓存
  做运行验证。
- 未执行 macOS / Linux 构建与运行验证、多 DPI 完整走查。

## Git

- 构建提交：`e277e09c306a4616b18a3fa5f86ebfd672637297`（`build: 准备 v0.12.1 的版本号、资源与文档`）
- 安全修复提交：`4e5a148`（`fix: 升级 compress 与 vite，构建工具链固定为 Go 1.27.3`）
- Tag：`v0.12.1`（指向本快照所在的发布提交）
