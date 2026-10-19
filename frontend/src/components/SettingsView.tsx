import { Button, Input } from '@fluentui/react-components'
import { ArrowLeftRegular, ArrowResetRegular, SearchRegular, PaintBrushRegular, TextFontRegular, DatabaseRegular, KeyboardRegular, CheckmarkCircleRegular, SettingsRegular } from '@fluentui/react-icons'
import { useId, useState, type ReactNode } from 'react'
import { usePreferences } from '../hooks/usePreferences'
import { DiagnosticsView } from './DiagnosticsView'
import { ReadingPreview } from './MessagePane'
import { AgentSelect } from './AgentSelect'
import { TerminalShellControl } from './TerminalShellControl'

const categories = [
  {id:'appearance',label:'外观',icon:<PaintBrushRegular />},
  {id:'reading',label:'会话阅读',icon:<TextFontRegular />},
  {id:'launch',label:'新建与启动',icon:<SettingsRegular />},
  {id:'data',label:'数据与诊断',icon:<DatabaseRegular />},
  {id:'shortcuts',label:'快捷键',icon:<KeyboardRegular />},
] as const

export function SettingsView({ revision, onBack }: { revision: number; onBack: () => void }) {
  const {preferences:p, update, reset, error} = usePreferences()
  const [category,setCategory] = useState<string>('appearance')
  const [query,setQuery] = useState('')
  const baseId = useId()
  const q = query.trim().toLocaleLowerCase()
  const matches = (section: string, text: string) => q ? text.toLocaleLowerCase().includes(q) : section === category
  const groups: {id:string;title:string;description:string;rows:{key:string;title:string;description:string;control:(id:string)=>ReactNode}[]}[] = [
    {id:'launch',title:'新建与启动',description:'设置新建项目和会话时默认使用的智能体，以及启动终端所用的 PowerShell。',rows:[
      {key:'defaultAgent',title:'默认智能体',description:'默认使用 Claude Code CLI。项目已设置默认智能体时优先使用项目设置；从智能体右键菜单新建时优先使用所选智能体。新建会话时可临时更改。',control:id=><AgentSelect id={id} value={p.defaultAgent} onChange={agent=>update({defaultAgent:agent})} />},
      {key:'terminalShell',title:'终端 PowerShell',description:'恢复或新建会话时启动终端所用的 PowerShell。默认使用 Windows PowerShell 5.1；PowerShell 7+ 已安装时可改用它，也可指定其他可执行文件。新开终端立即生效。',control:id=><TerminalShellControl id={id} />},
    ]},
    {id:'appearance',title:'外观',description:'设置工作台主题、正文字号与阅读宽度。',rows:[
      {key:'theme',title:'颜色主题',description:'跟随系统，或固定使用浅色 / 深色。',control:id=><select id={id} value={p.theme} onChange={e=>update({theme:e.target.value as typeof p.theme})}><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select>},
      {key:'fontSize',title:'正文字号',description:'应用于会话正文；列表和工具栏字号保持不变。',control:id=><select id={id} value={p.fontSize} onChange={e=>update({fontSize:Number(e.target.value)})}>{[13,14,15,16,18].map(n=><option key={n} value={n}>{n} px</option>)}</select>},
      {key:'width',title:'阅读宽度',description:'适中宽度用于常规阅读；宽版适用于较宽屏幕和代码内容。',control:id=><select id={id} value={p.readingWidth} onChange={e=>update({readingWidth:e.target.value as typeof p.readingWidth})}><option value="balanced">适中</option><option value="wide">宽版</option></select>},
    ]},
    {id:'reading',title:'会话阅读',description:'设置会话初始位置、正文行距和过程记录的显示方式。',rows:[
      {key:'start',title:'默认阅读位置',description:'首次打开或没有已记阅读位置时应用；新的搜索跳转优先定位匹配段。',control:id=><select id={id} value={p.readingStart} onChange={e=>update({readingStart:e.target.value as typeof p.readingStart})}><option value="latest">最新一轮问答</option><option value="start">会话开头</option></select>},
      {key:'spacing',title:'正文行距',description:'舒适行距适合长文，紧凑行距方便快速浏览。',control:id=><select id={id} value={p.spacing} onChange={e=>update({spacing:e.target.value as typeof p.spacing})}><option value="comfortable">舒适</option><option value="compact">紧凑</option></select>},
      {key:'fold',title:'默认折叠过程记录',description:'默认收起思考过程、工具调用、工具结果、上下文与结构化数据。',control:id=><input id={id} type="checkbox" role="switch" checked={p.foldProcesses} onChange={e=>update({foldProcesses:e.target.checked})} />},
    ]},
  ]
  const shown = groups.map(group=>({...group,rows:group.rows.filter(row=>matches(group.id,`${group.title} ${row.title} ${row.description}`))})).filter(group=>group.rows.length)
  const data = matches('data','数据与诊断 永久会话库 持久保存 索引 重建 完整数据备份与恢复 完整备份 ZIP Agent 能力 数据存储位置 迁移 磁盘 空间 清理旧位置')
  const shortcuts = matches('shortcuts','快捷键 键盘 搜索 Ctrl 选择 Shift 阅读 设置')
  return <section className="ad-settings" aria-label="应用设置">
    <div className="ad-settings-layout">
      <aside className="ad-settings-rail">
        <Button className="ad-settings-back" size="small" appearance="subtle" icon={<ArrowLeftRegular />} onClick={onBack}>返回会话</Button>
        <div className="ad-settings-rail-title"><SettingsRegular /><span>应用设置</span></div>
        <nav className="ad-settings-nav" aria-label="设置分类">
          {categories.map(item=><button key={item.id} aria-pressed={!q&&category===item.id} onClick={()=>{setCategory(item.id);setQuery('')}}>{item.icon}{item.label}</button>)}
        </nav>
        <p className="ad-settings-local">本机配置<span>界面与启动偏好仅保存在本机</span></p>
      </aside>
      <div className="ad-settings-main">
        <div className="ad-settings-content">
          <header className="ad-settings-header">
            <div><h1>设置</h1><p>调整外观、阅读习惯与本机数据。</p></div>
            <span className={`ad-settings-save${error?' ad-settings-save-error':''}`} role="status">{!error ? <CheckmarkCircleRegular /> : null}{error || '更改自动保存'}</span>
          </header>
          <div className="ad-settings-toolbar">
            <Input value={query} onChange={(_,data)=>setQuery(data.value)} contentBefore={<SearchRegular />} placeholder="搜索设置…" aria-label="搜索设置" />
            <Button size="small" appearance="subtle" icon={<ArrowResetRegular />} onClick={reset} title="恢复界面偏好和应用默认智能体，不影响项目设置、会话、收藏与备注">恢复默认设置</Button>
          </div>
          {shown.map(group=><section className="ad-settings-section" key={group.id}>
            <h2>{group.title}</h2><p className="ad-settings-description">{group.description}</p>
            <div className={group.id==='appearance'?'ad-settings-appearance-grid':undefined}>
              <div className="ad-setting-card">{group.rows.map(row=><div className="ad-setting-row" key={row.key}>
              <div><label htmlFor={`${baseId}-${row.key}`}>{row.title}</label><p>{row.description}</p></div>{row.control(`${baseId}-${row.key}`)}
              </div>)}</div>
              {group.id==='appearance' || group.id==='reading' ? <aside className="ad-settings-preview" aria-label="阅读预览"><div className="ad-preview-toolbar"><span>阅读预览</span><span>实时更新</span></div><ReadingPreview /></aside> : null}
            </div>
          </section>)}
          {data ? <DiagnosticsView revision={revision} embedded /> : null}
          {shortcuts ? <section className="ad-settings-section"><h2>快捷键</h2><p className="ad-settings-description">列表选择遵循常见桌面应用的操作习惯。</p><dl className="ad-shortcuts">{[
            ['搜索所有会话','Ctrl + K'],['新建会话','Ctrl + N'],['新建项目','Ctrl + Shift + N'],['打开设置','Ctrl + ,'],['调整导航宽度','拖拽分隔条 / 方向键'],['打开会话操作菜单','Shift + F10'],['查找当前会话','Ctrl + F'],['下一 / 上一命中段','Enter / Shift + Enter'],['全选筛选后当前展开项','Ctrl + A'],['追加 / 取消选择','Ctrl + 单击'],['连续选择','Shift + 单击'],['追加连续范围','Ctrl + Shift + 单击'],['清空选择','Esc'],['移动焦点 / 扩展选择','方向键 / Shift + 方向键'],['保存备注','Ctrl + Enter'],
          ].map(([label,key])=><div key={label}><dt>{label}</dt><dd><kbd>{key}</kbd></dd></div>)}</dl></section> : null}
          {!shown.length&&!data&&!shortcuts ? <p className="ad-settings-description">未找到匹配的设置，请调整搜索关键词。</p> : null}
        </div>
      </div>
    </div>
  </section>
}
