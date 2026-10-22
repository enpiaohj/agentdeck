import { Button, Input, Dialog, DialogSurface, DialogBody, DialogTitle, DialogContent, DialogActions } from '@fluentui/react-components'
import { ArrowLeftRegular, ArrowResetRegular, ArrowSyncRegular, SearchRegular, PaintBrushRegular, TextFontRegular, DatabaseRegular, KeyboardRegular, CheckmarkCircleRegular, SettingsRegular, WindowNewRegular, DismissRegular, ArrowDownloadRegular, PulseRegular } from '@fluentui/react-icons'
import { useId, useRef, useState, type ReactNode } from 'react'
import { usePreferences } from '../hooks/usePreferences'
import { matchesSetting, settingSearchKeywords } from '../settingsSearch'
import { DiagnosticsView, type SettingsDataSection } from './DiagnosticsView'
import { ReadingPreview } from './MessagePane'
import { AgentSelect } from './AgentSelect'
import { TerminalShellControl } from './TerminalShellControl'
import { DesktopSettingsControl, matchesDesktopSettings } from './DesktopSettingsControl'
import { UpdateControl, matchesUpdateSettings } from './UpdateControl'
import { SettingRow } from './SettingRow'

const categories = [
  { id: 'general', label: '常规', description: '设置应用启动、窗口关闭与新建会话的默认选项。', icon: <SettingsRegular /> },
  { id: 'appearance', label: '外观', description: '设置界面主题、导航侧栏与详情栏布局。', icon: <PaintBrushRegular /> },
  { id: 'sessions', label: '会话', description: '设置会话排序、正文显示与默认阅读方式。', icon: <TextFontRegular /> },
  { id: 'terminal', label: '终端', description: '配置新建和继续会话时使用的 PowerShell 环境。', icon: <WindowNewRegular /> },
  { id: 'data', label: '数据管理', description: '查看存储占用、迁移数据位置与管理本地会话库。', icon: <DatabaseRegular /> },
  { id: 'backup', label: '备份与恢复', description: '创建完整数据备份，或从备份文件恢复数据。', icon: <ArrowDownloadRegular /> },
  { id: 'diagnostics', label: '诊断', description: '检查智能体状态与搜索索引。', icon: <PulseRegular /> },
  { id: 'updates', label: '更新', description: '检查新版本，下载并校验到本地。', icon: <ArrowSyncRegular /> },
  { id: 'shortcuts', label: '快捷键', description: '查看导航、会话管理与阅读操作的键盘快捷键。', icon: <KeyboardRegular /> },
] as const

type SettingGroup = { id: string; category: string; title: string; rows: { key: string; title: string; description: string; control: (id: string) => ReactNode }[] }

export function SettingsView({ revision, onBack }: { revision: number; onBack: () => void }) {
  const { preferences: p, update, reset, error } = usePreferences()
  const [category, setCategory] = useState<string>('general')
  const [query, setQuery] = useState('')
  const [resetOpen, setResetOpen] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const baseId = useId()
  const q = query.trim()
  const active = categories.find(item => item.id === category) ?? categories[0]
  const matches = (section: string, ...text: string[]) => q ? matchesSetting(q, ...text) : section === category
  const groups: SettingGroup[] = [
    { id: 'defaults', category: 'general', title: '新建会话', rows: [
      { key: 'defaultAgent', title: '默认智能体', description: '新建会话时优先使用项目配置，否则使用此选项。', control: id => <AgentSelect id={id} value={p.defaultAgent} onChange={agent => update({ defaultAgent: agent })} /> },
      { key: 'recentProjects', title: '最近使用的目录', description: p.recentProjects.length ? `已记录 ${p.recentProjects.length} 个目录，供新建会话时快速选择。` : '使用项目目录后，会在新建会话时提供快捷选择。', control: id => <Button id={id} size="small" disabled={!p.recentProjects.length} onClick={() => update({ recentProjects: [] })}>清除目录记录</Button> },
    ] },
    { id: 'theme', category: 'appearance', title: '界面主题', rows: [
      { key: 'theme', title: '颜色主题', description: '跟随系统外观，或始终使用浅色、深色主题。', control: id => <select id={id} value={p.theme} onChange={e => update({ theme: e.target.value as typeof p.theme })}><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select> },
    ] },
    { id: 'layout', category: 'appearance', title: '界面布局', rows: [
      { key: 'navCollapsed', title: '侧栏默认状态', description: '展开时显示名称，收起时仅显示图标。窄窗口下自动收起。', control: id => <select id={id} value={p.navCollapsed ? 'collapsed' : 'expanded'} onChange={e => update({ navCollapsed: e.target.value === 'collapsed' })}><option value="expanded">展开</option><option value="collapsed">收起</option></select> },
      { key: 'navWidth', title: '侧栏宽度', description: p.navWidth ? `当前为 ${p.navWidth} px。可拖动分隔条调整，或恢复自动宽度。` : '随窗口宽度自动调整。拖动分隔条可自定义宽度。', control: id => <Button id={id} size="small" disabled={!p.navWidth} onClick={() => update({ navWidth: 0 })}>恢复自动宽度</Button> },
      { key: 'listWidth', title: '会话列表宽度', description: p.listWidth ? `当前为 ${p.listWidth} px。可拖动列表右侧的分隔条调整，或恢复自动宽度。` : '随窗口宽度自动调整。拖动列表右侧的分隔条可自定义宽度。', control: id => <Button id={id} size="small" disabled={!p.listWidth} onClick={() => update({ listWidth: 0 })}>恢复自动宽度</Button> },
      { key: 'infoWidth', title: '会话详情栏宽度', description: p.infoWidth ? `当前为 ${p.infoWidth} px。可拖动详情栏左侧的分隔条调整，或恢复自动宽度。` : '随窗口宽度自动调整。拖动详情栏左侧的分隔条可自定义宽度。', control: id => <Button id={id} size="small" disabled={!p.infoWidth} onClick={() => update({ infoWidth: 0 })}>恢复自动宽度</Button> },
      { key: 'infoPanel', title: '会话详情栏', description: '选中会话时，按此状态显示标签、备注与管理操作。', control: id => <select id={id} value={p.infoPanel} onChange={e => update({ infoPanel: e.target.value as typeof p.infoPanel })}><option value="open">默认展开</option><option value="closed">默认收起</option></select> },
      { key: 'hideEmptyProjects', title: '隐藏空项目', description: '隐藏没有会话的项目，正在筛选的项目除外。项目本身仍会保留。', control: id => <input id={id} aria-describedby={`${id}-description`} type="checkbox" role="switch" checked={p.hideEmptyProjects} onChange={e => update({ hideEmptyProjects: e.target.checked })} /> },
    ] },
    { id: 'list', category: 'sessions', title: '会话列表', rows: [
      { key: 'listOrder', title: '默认排序', description: '重新进入全部会话或启动应用时使用此排序。', control: id => <select id={id} value={p.listOrder} onChange={e => update({ listOrder: e.target.value as typeof p.listOrder })}><option value="latest">最近活动优先</option><option value="oldest">最早活动优先</option></select> },
    ] },
    { id: 'typography', category: 'sessions', title: '正文显示', rows: [
      { key: 'fontSize', title: '正文字号', description: '调整会话正文的文字大小。', control: id => <select id={id} value={p.fontSize} onChange={e => update({ fontSize: Number(e.target.value) })}>{[13,14,15,16,18].map(n => <option key={n} value={n}>{n} px</option>)}</select> },
      { key: 'spacing', title: '正文行距', description: '舒适适合长文阅读，紧凑便于快速浏览。', control: id => <select id={id} value={p.spacing} onChange={e => update({ spacing: e.target.value as typeof p.spacing })}><option value="comfortable">舒适</option><option value="compact">紧凑</option></select> },
      { key: 'width', title: '阅读宽度', description: '自适应随窗口调整；适中与宽版使用固定的阅读宽度。', control: id => <select id={id} value={p.readingWidth} onChange={e => update({ readingWidth: e.target.value as typeof p.readingWidth })}><option value="balanced">适中</option><option value="wide">宽版</option><option value="adaptive">自适应</option></select> },
    ] },
    { id: 'reading', category: 'sessions', title: '阅读行为', rows: [
      { key: 'start', title: '默认阅读位置', description: '没有已保存的阅读位置时，打开会话后定位到此处。', control: id => <select id={id} value={p.readingStart} onChange={e => update({ readingStart: e.target.value as typeof p.readingStart })}><option value="latest">最新一轮问答</option><option value="start">会话开头</option></select> },
      { key: 'fold', title: '默认折叠过程记录', description: '收起思考、工具调用与结构化输出；需要时可逐项展开。', control: id => <input id={id} aria-describedby={`${id}-description`} type="checkbox" role="switch" checked={p.foldProcesses} onChange={e => update({ foldProcesses: e.target.checked })} /> },
    ] },
    { id: 'delete', category: 'sessions', title: '删除会话', rows: [
      { key: 'deleteBackup', title: '删除前创建恢复备份', description: p.deleteBackup ? '删除会话前自动备份所选会话，可在本次运行期间撤销。备份文件保留在本地磁盘。' : '已停用。删除会话时不创建备份，删除后无法恢复。', control: id => <input id={id} aria-describedby={`${id}-description`} type="checkbox" role="switch" checked={p.deleteBackup} onChange={e => update({ deleteBackup: e.target.checked })} /> },
    ] },
    { id: 'shell', category: 'terminal', title: '终端环境', rows: [
      { key: 'terminalShell', title: '终端 PowerShell', description: '新建或继续会话时使用的终端。更改后对新打开的终端生效。', control: id => <TerminalShellControl id={id} /> },
    ] },
  ]
  const shown = groups.map(group => ({ ...group, rows: group.rows.filter(row => matches(group.category, categories.find(item => item.id === group.category)?.label ?? '', group.title, row.title, row.description, settingSearchKeywords[row.key] ?? '')) })).filter(group => group.rows.length)
  const desktop = q ? matchesDesktopSettings(q) : category === 'general'
  const dataSections: SettingsDataSection[] = []
  if (matches('data', '数据管理 存储位置 数据目录 迁移 磁盘 空间 清理旧位置')) dataSections.push('storage')
  if (matches('data', '数据管理 本地会话库 永久会话库 历史库 持久保存 压缩 版本 统计 清理旧版本 旧版本 回收空间')) dataSections.push('library')
  if (matches('backup', '备份与恢复 完整数据备份 ZIP 文件 导入历史 收藏 标签 备注 偏好设置')) dataSections.push('backup')
  if (matches('diagnostics', '诊断 智能体检测 能力 状态 发现 原生恢复 会话来源')) dataSections.push('agents')
  if (matches('diagnostics', '诊断 搜索索引 全文索引 重建 检索')) dataSections.push('index')
  const shortcuts = matches('shortcuts', '快捷键 键盘 搜索 Ctrl 选择 Shift 阅读 设置 F2 重命名 Delete 删除 F5 刷新')
  const updates = q ? matchesUpdateSettings(q) : category === 'updates'
  const empty = !shown.length && !desktop && !dataSections.length && !shortcuts && !updates
  const chooseCategory = (id: string) => { setCategory(id); setQuery(''); contentRef.current?.scrollTo({ top: 0 }) }
  return <section className="ad-settings" aria-label="应用设置">
    <div className="ad-settings-layout">
      <aside className="ad-settings-rail">
        <Button className="ad-settings-back" size="small" appearance="subtle" icon={<ArrowLeftRegular />} onClick={onBack}>返回会话</Button>
        <div className="ad-settings-rail-title">设置</div>
        <nav className="ad-settings-nav" aria-label="设置分类">
          {categories.map(item => <button key={item.id} aria-current={!q && category === item.id ? 'page' : undefined} onClick={() => chooseCategory(item.id)}>{item.icon}{item.label}</button>)}
        </nav>
        <p className="ad-settings-local">本机设置<span>偏好设置仅应用于当前设备</span></p>
      </aside>
      <div className="ad-settings-main" ref={contentRef}>
        <div className="ad-settings-content">
          <div className="ad-settings-toolbar">
            <Input value={query} onChange={(_, value) => { setQuery(value.value); contentRef.current?.scrollTo({ top: 0 }) }} contentBefore={<SearchRegular />} contentAfter={q ? <Button appearance="transparent" size="small" icon={<DismissRegular />} aria-label="清除设置搜索" onClick={() => setQuery('')} /> : undefined} placeholder="搜索设置" aria-label="搜索设置" />
            <span className={`ad-settings-save${error ? ' ad-settings-save-error' : ''}`} role={error ? 'alert' : undefined}>{error ? error : <><CheckmarkCircleRegular />自动保存</>}</span>
          </div>
          <header className="ad-settings-header"><h1>{q ? '搜索结果' : active.label}</h1><p>{q ? `查找与“${q}”相关的设置` : active.description}</p></header>
          {desktop ? <section className="ad-settings-section" aria-labelledby={`${baseId}-desktop-heading`}><h2 id={`${baseId}-desktop-heading`}>{q ? '常规 · 启动与退出' : '启动与退出'}</h2><DesktopSettingsControl id={`${baseId}-desktop`} query={q} /></section> : null}
          {shown.map(group => <section className="ad-settings-section" key={group.id} aria-labelledby={`${baseId}-${group.id}-heading`}>
            <h2 id={`${baseId}-${group.id}-heading`}>{q ? `${categories.find(item => item.id === group.category)?.label} · ${group.title}` : group.title}</h2>
            <div className="ad-setting-card">{group.rows.map(row => <SettingRow key={row.key} id={`${baseId}-${row.key}`} title={row.title} description={row.description}>{row.control(`${baseId}-${row.key}`)}</SettingRow>)}</div>
          </section>)}
          {!q && ['appearance', 'sessions'].includes(category) ? <aside className="ad-settings-preview" aria-label="阅读预览"><div className="ad-preview-toolbar"><span>阅读预览</span><span>实时更新</span></div><ReadingPreview /></aside> : null}
          {dataSections.length ? <DiagnosticsView key={dataSections.join(':')} revision={revision} embedded sections={dataSections} /> : null}
          {updates ? <section className="ad-settings-section" aria-labelledby={`${baseId}-update-heading`}>
            <h2 id={`${baseId}-update-heading`}>{q ? '更新 · 版本' : '版本与更新'}</h2>
            <UpdateControl id={`${baseId}-update`} />
          </section> : null}
          {shortcuts ? <section className="ad-settings-section"><h2>快捷键列表</h2><dl className="ad-shortcuts">{[
            ['搜索所有会话','Ctrl + K'],['刷新会话列表','F5'],['新建会话','Ctrl + N'],['新建项目','Ctrl + Shift + N'],['打开设置','Ctrl + ,'],['调整栏宽','拖拽分隔条 / 方向键'],['打开操作菜单','Shift + F10 / 菜单键'],['重命名所选会话','F2'],['删除所选会话','Delete'],['查找当前会话','Ctrl + F'],['下一 / 上一处匹配','Enter / Shift + Enter'],['全选当前列表','Ctrl + A'],['多选 / 连选智能体（侧栏）','Ctrl + 单击 / Shift + 单击'],['追加 / 取消选择','Ctrl + 单击'],['连续选择','Shift + 单击'],['追加连续范围','Ctrl + Shift + 单击'],['清空选择','Esc'],['移动焦点 / 扩展选择','方向键 / Shift + 方向键'],['保存备注','Ctrl + Enter'],
          ].map(([label, key]) => <div key={label}><dt>{label}</dt><dd><kbd>{key}</kbd></dd></div>)}</dl><p className="ad-settings-note">全选仅作用于筛选后当前展开的会话。删除操作会先显示确认窗口。</p></section> : null}
          {empty ? <div className="ad-settings-empty"><SearchRegular /><h2>未找到匹配的设置</h2><p>尝试使用“主题”“托盘”或“备份”等关键词。</p><Button size="small" onClick={() => setQuery('')}>清除搜索</Button></div> : null}
          {!q && ['general', 'appearance', 'sessions'].includes(category) ? <footer className="ad-settings-footer"><div><strong>偏好设置</strong><p>恢复外观、布局、阅读与默认智能体的初始设置。</p></div><Button appearance="subtle" size="small" icon={<ArrowResetRegular />} onClick={() => setResetOpen(true)}>恢复偏好设置…</Button></footer> : null}
        </div>
      </div>
    </div>
    <Dialog open={resetOpen} onOpenChange={(_, value) => setResetOpen(value.open)}><DialogSurface><DialogBody><DialogTitle>恢复偏好设置？</DialogTitle><DialogContent>颜色主题、界面布局、会话设置、默认智能体和最近目录记录将恢复为初始状态。开机启动、托盘行为、终端配置及会话数据保持原样。</DialogContent><DialogActions><Button onClick={() => setResetOpen(false)}>取消</Button><Button appearance="primary" onClick={() => { reset(); setResetOpen(false) }}>恢复默认</Button></DialogActions></DialogBody></DialogSurface></Dialog>
  </section>
}
