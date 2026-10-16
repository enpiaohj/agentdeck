# AgentDeck 图标体系

界面里出现的每一个图标,要么是本工具自己画的,要么是对方产品的真实品牌图标。
不存在「看起来像」的仿制标志。

## 1. 三层图标

| 层 | 内容 | 代码 | 资源 |
| --- | --- | --- | --- |
| 应用标志 | AgentDeck 自己 | `AppMark.tsx` | `build/appicon.png` → `frontend/public/appicon-256.png` |
| Agent 标识 | 每个被管理的 Agent | `AgentMark.tsx` + `agents.ts` | `frontend/public/agents/<key>.png` |
| 界面图标 | 导航、动作、状态 | `@fluentui/react-icons` | 组件内联引用 |

界面图标沿用 Fluent 图标集:它是完整的 24×24 网格、1.5px 描边、圆头圆角体系,
自绘一套只会更差。需要自绘的场合只有两种——应用标志、以及 Fluent 里没有的
品牌标识。

## 2. 形状语言

- **方片**(应用标志、Agent 标识):圆角半径 = 边长 / 4。
- Agent 标识不带边框、不带描边、不叠阴影——品牌图标自己带底色,再加装饰会脏。
- 尺寸取 14 / 16 / 18 / 24 / 30 / 38:列表行 14、筛选器 14、侧栏与搜索 16~18、
  消息流头像 30、顶栏应用标志 38。
- 同一 Agent 在任何位置都是同一张图、同一个圆角比例,只有尺寸变化。

## 3. 应用图标(自己画的)

`tools/gen_appicon.ps1` 用 GDI+ 现画,不依赖任何素材:

- 蓝色渐变(azure → indigo)圆角方片,顶部一层高光、底部一层暗角;
- 中间一块白色工作台,三条分隔线切成四栏——就是本应用的四栏布局
  (导航 | 会话列表 | 消息流 | 会话信息);
- 内容栏中央一枚四角星,表示「有 Agent 正在工作」。

每个尺寸都单独绘制(不从大图缩放),所以 16px 那一档不会糊。输出:

```
build/appicon.png             1024px 母版
build/windows/icon.ico        256/128/64/48/32/24/16,PNG 压缩条目
frontend/public/appicon-256.png   界面顶栏标志(与 exe 图标同源)
```

exe 图标经 Wails 的资源文件嵌入:

```powershell
wails3 generate syso `
  -icon build/windows/icon.ico `
  -info build/windows/info.json `
  -manifest build/windows/wails.exe.manifest `
  -arch amd64 -out cmd/agentdeck-desktop/resource_windows_amd64.syso
```

`resource_windows_amd64.syso` 是普通构建输入(Go 会自动链接同目录下的 .syso),
换图标后必须重跑上面这条命令再 `go build`,否则 exe 还是旧图标。

## 4. Agent 图标(取真的)

`tools/gen_agenticons.ps1` 从本机已安装产品的图标资源里提取,缩放到 128px:

| key | 来源 |
| --- | --- |
| claude / claude-desktop | Claude Desktop(MSIX)`app/resources/ion-dist/images/claude_app_icon.png` |
| codex | Codex Desktop(MSIX)`app/resources/default_app/icon.png` |
| zcode | `C:\Program Files\ZCode\resources\icon_windows.png` |
| hermes | hermes-agent `apps/desktop/public/hermes.png`(品牌吉祥物) |
| workbuddy | `C:\Program Files\WorkBuddyAI\...\resources\icon.png` |
| qoder-cn | `C:\Program Files\Qoder\resources\application-icons\qoder-dark-windows.png` |

- Store/MSIX 路径带版本号,脚本用通配符在运行时解析,不写死版本。
- 每个 key 给一串候选路径,取第一个存在的;产物直接覆盖 `frontend/public/agents/`。
- 脚本只读源文件,不改动任何已安装产品。

### 没有官方图标时

`agents.ts` 里 `icon: null` → `AgentMark` 渲染识别色方片 + 首字母,并在代码里
写明原因。**不用相近的图形冒充品牌标志**,也不拿安装器自带的通用图标顶替
(OpenCode 就是这种情况:本机只有更新器安装包,没有产品图标资源)。

补上真图标的方法:把 PNG 放到 `frontend/public/agents/<key>.png`,在 `agents.ts`
里把该条目的 `icon` 从 `null` 改成路径即可。图标加载失败时组件会自动退回方片,
不会出现破图。

## 5. 新增一个 Agent

1. `frontend/src/icons/agents.ts` 的 `AGENT_LIST` 加一条(key / label / color);
2. 若本机装了对应产品:`tools/gen_agenticons.ps1` 的 `$sources` 里加候选路径,
   重跑脚本;
3. 否则留 `icon: null`,写清没有官方资源的原因。
