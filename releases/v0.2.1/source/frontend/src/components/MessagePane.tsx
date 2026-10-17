// 主区:会话消息流。对齐概念图:大标题 + chips 行 + 头像消息流(用户圆头像 /
// Agent 品牌方块)+ Markdown 围栏代码块(文件名头 + 行号 + 复制)+ 底部操作条。
import {
  makeStyles,
  mergeClasses,
  shorthands,
  tokens,
  Text,
  Button,
  Spinner,
  Tooltip,
} from '@fluentui/react-components'
import {
  PersonRegular,
  CopyRegular,
  CheckmarkRegular,
  ArrowClockwiseRegular,
  ArrowDownloadRegular,
  CommentMultipleRegular,
  OpenRegular,
  PanelRightRegular,
  AddRegular,
  ChevronRightRegular,
  WrenchRegular,
  ArrowUpRegular,
  ArrowDownRegular,
  ChevronLeftRegular,
} from '@fluentui/react-icons'
import { createContext, useCallback, useContext, useEffect, useId, useMemo, useRef, useState } from 'react'
import { Browser, Dialogs } from '@wailsio/runtime'
import type { SessionView, TranscriptView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import { agentMeta, fmtTime } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { highlight } from '../highlight'
import { useTheme } from '../hooks/useTheme'
import { foldedContent, isLongCode, markdownCodeText, type FoldedContent } from '../transcriptPresentation'
import type { ComponentProps, ReactNode } from 'react'
import Markdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { usePreferences } from '../hooks/usePreferences'

const useStyles = makeStyles({
  wrap: {
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    minHeight: 0,
    backgroundColor: tokens.colorNeutralBackground1,
  },
  // ---- 头部:工作区路径 + 标题 + chips ----
  head: {
    padding: '16px 24px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
  },
  headRow: { display: 'flex', alignItems: 'flex-start', gap: '12px' },
  title: { flex: 1, minWidth: 0, fontSize: '18px', lineHeight: 1.5, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden', letterSpacing: '-0.025em', overflowWrap: 'anywhere' },
  headActions: { display: 'flex', gap: '4px', flexShrink: 0 },
  chips: { display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0, color: tokens.colorNeutralForeground3, overflow: 'hidden' },
  chipAdd: {
    flexShrink: 0,
    whiteSpace: 'nowrap',
    display: 'inline-flex',
    alignItems: 'center',
    gap: '3px',
    backgroundColor: 'transparent',
    cursor: 'pointer',
    fontSize: '12px',
    padding: '1px 6px',
    ...shorthands.borderRadius('6px'),
    ...shorthands.border('1px', 'dashed', tokens.colorNeutralStroke1),
    color: tokens.colorNeutralForeground3,
  },
  // ---- 消息流 ----
  body: { flex: 1, overflowY: 'auto', padding: '24px clamp(16px, 2vw, 32px) 8px', minHeight: 0, backgroundColor: tokens.colorNeutralBackground1 },
  msg: { display: 'flex', gap: '12px', marginBottom: '24px', paddingBottom: '20px', borderBottom: `1px solid ${tokens.colorNeutralStroke3}`, maxWidth: 'var(--ad-reader-width, 820px)', marginLeft: 'auto', marginRight: 'auto' },
  processMsg: { display: 'flex', alignItems: 'flex-start', gap: '7px', marginBottom: '8px', maxWidth: 'var(--ad-reader-width, 820px)', marginLeft: 'auto', marginRight: 'auto' },
  avatarCol: { flexShrink: 0, paddingTop: '2px' },
  userAvatar: {
    width: '26px',
    height: '26px',
    ...shorthands.borderRadius('50%'),
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: tokens.colorNeutralBackground4,
    color: tokens.colorNeutralForeground2,
  },
  msgBody: { flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: '8px', padding: '0', backgroundColor: 'transparent' },
  userMessage: { backgroundColor: tokens.colorNeutralBackground2, padding: '14px 16px', ...shorthands.borderRadius('10px'), ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3) },
  msgHead: { display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' },
  roleName: { fontWeight: 600, fontSize: '12px' },
  msgTime: { fontSize: '11px', color: tokens.colorNeutralForeground3 },
  msgCopy: {
    marginLeft: 'auto',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    padding: '5px',
    ...shorthands.borderRadius('4px'),
    color: tokens.colorNeutralForeground3,
    opacity: 0,
    ':focus-visible': { opacity: 1 },
  },
  text: { fontSize: '14px', lineHeight: 1.7, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' },
  // ---- 代码块 ----
  codeBlock: {
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3),
    ...shorthands.borderRadius('8px'),
    overflow: 'hidden',
    backgroundColor: tokens.colorNeutralBackground2,
  },
  codeHead: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '6px 12px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    backgroundColor: tokens.colorNeutralBackground2,
    fontSize: '12px',
    color: tokens.colorNeutralForeground2,
  },
  // 语言徽标(概念图:代码块标题栏左侧的小色块)
  langBadge: {
    fontSize: '10.5px',
    fontWeight: 700,
    letterSpacing: '0.02em',
    padding: '1px 5px',
    ...shorthands.borderRadius('4px'),
    backgroundColor: tokens.colorBrandBackground2,
    color: tokens.colorBrandForeground2,
    flexShrink: 0,
  },
  codeName: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  codeCopy: {
    marginLeft: 'auto',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    fontSize: '12px',
    color: tokens.colorNeutralForeground3,
    padding: '2px 4px',
    ...shorthands.borderRadius('4px'),
  },
  codeBody: { display: 'flex', overflowX: 'auto', padding: '8px 0' },
  gutter: {
    fontFamily: 'Cascadia Code, Consolas, monospace',
    fontSize: '12.5px',
    lineHeight: 1.65,
    color: tokens.colorNeutralForeground4,
    textAlign: 'right',
    padding: '0 10px 0 12px',
    userSelect: 'none',
    flexShrink: 0,
  },
  codeText: {
    fontFamily: 'Cascadia Code, Consolas, monospace',
    fontSize: '12.5px',
    lineHeight: 1.65,
    whiteSpace: 'pre',
    paddingRight: '14px',
    color: tokens.colorNeutralForeground1,
  },
  fold: { backgroundColor: tokens.colorNeutralBackground2, border: `1px solid ${tokens.colorNeutralStroke3}`, borderRadius: '6px', overflow: 'hidden' },
  foldSummary: { display: 'flex', alignItems: 'center', gap: '7px', padding: '8px 10px', cursor: 'pointer', listStyleType: 'none', ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover } },
  foldLabel: { fontSize: '12px', fontWeight: 600, color: tokens.colorNeutralForeground2, maxWidth: '65%', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  foldPreview: { flex: 1, minWidth: 0, fontSize: '11px', color: tokens.colorNeutralForeground3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  foldCount: { fontSize: '11px', color: tokens.colorNeutralForeground3, whiteSpace: 'nowrap', flexShrink: 0 },
  foldBody: { padding: '0 10px 10px' },
  codeToggle: { border: 'none', background: 'transparent', cursor: 'pointer', color: tokens.colorBrandForeground1, fontSize: '11px', whiteSpace: 'nowrap', padding: '2px 4px', flexShrink: 0 },
  hit: { backgroundColor: tokens.colorBrandBackground2, ...shorthands.borderRadius('8px'), padding: '8px 10px', margin: '-8px -10px' },
  // ---- 底部操作条(概念图:居中一排胶囊按钮)----
  bottom: {
    padding: '7px 16px',
    minHeight: '44px',
    backgroundColor: tokens.colorNeutralBackground1,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-start',
    gap: '8px',
    flexWrap: 'wrap',
    borderTop: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  grayed: { opacity: 0.5 },
  empty: {
    padding: '96px 40px 40px',
    textAlign: 'center',
    color: tokens.colorNeutralForeground3,
  },
  emptyIcon: {
    display: 'inline-flex',
    width: '56px',
    height: '56px',
    ...shorthands.borderRadius('16px'),
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: tokens.colorNeutralBackground3,
    color: tokens.colorNeutralForeground4,
  },
  errRow: { display: 'flex', alignItems: 'flex-start', gap: '8px' },
})

type Props = {
  session: SessionView | null
  sourceVersion: number
  jumpToken: number
  infoOpen: boolean
  onToggleInfo: () => void
  jumpPart: number // 搜索命中定位到 transcript 第 n 段(-1 = 不定位)
  onOpenInfo: () => void
}

export function MessagePane({ session, sourceVersion, jumpPart, jumpToken, infoOpen, onToggleInfo, onOpenInfo }: Props) {
  const styles = useStyles()
  if (!session) {
    // 注意:此前用 `flex:1` 的 flex 容器做居中,该容器内的子元素在本项目
    // 的 WebView2 里完全不绘制(容器背景可见、文本不可见)。改用 block 布局。
    return (
      <div className={styles.wrap}>
        <div className={styles.empty}>
          <span className={styles.emptyIcon}><CommentMultipleRegular fontSize={26} /></span>
          <Text size={400} weight="semibold" style={{ display: 'block', marginTop: 10 }}>选择一个会话查看消息</Text>
          <Text size={200} style={{ display: 'block', marginTop: 6 }}>左侧会话列表按时间分组;Ctrl+K 可全文检索消息内容</Text>
        </div>
      </div>
    )
  }
  return <Loaded key={session.key} session={session} sourceVersion={sourceVersion} jumpPart={jumpPart} jumpToken={jumpToken} infoOpen={infoOpen} onToggleInfo={onToggleInfo} onOpenInfo={onOpenInfo} />
}

function Loaded({ session, sourceVersion, jumpPart, jumpToken, infoOpen, onToggleInfo, onOpenInfo }: { session: SessionView; sourceVersion: number; jumpPart: number; jumpToken: number; infoOpen: boolean; onToggleInfo: () => void; onOpenInfo: () => void }) {
  const styles = useStyles()
  const theme = useTheme()
  const m = agentMeta(session.agent)
  const { preferences } = usePreferences()
  const [destination, setDestination] = useState(() => ({ at:jumpPart>=0?jumpPart:preferences.readingStart==='latest'?-1:0, before:false, focus:jumpPart>=0?jumpPart:null as number|null, revision:0 }))
  const lastJump = useRef(`${jumpPart}:${jumpToken}`)
  const request = useRef(0)
  const [tr, setTr] = useState<TranscriptView | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState('')
  const [actionError, setActionError] = useState('')
  const [actionBusy, setActionBusy] = useState(false)
  const hitRef = useRef<HTMLElement | null>(null)
  const latestRef = useRef<HTMLElement | null>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const hitPart = destination.focus ?? -1
  let latestIndex = -1
  tr?.parts.forEach((part,index)=>{if((part.role==='assistant'||part.role==='user')&&part.kind==='text')latestIndex=index})

  useEffect(()=>{
    const value=`${jumpPart}:${jumpToken}`
    if(lastJump.current===value)return
    lastJump.current=value
    if(jumpPart>=0)setDestination(previous=>({at:jumpPart,before:false,focus:jumpPart,revision:previous.revision+1}))
  },[jumpPart,jumpToken])

  const navigate = (at:number,before=false) => setDestination(previous=>({at,before,focus:null,revision:previous.revision+1}))

  const runAction = async (action: () => Promise<void>) => {
    setActionBusy(true); setActionError('')
    try { await action() } catch (error) { setActionError(String(error).slice(0, 180)) }
    finally { setActionBusy(false) }
  }

  const load = useCallback(() => {
    const version=++request.current
    setLoading(true)
    setErr('')
    sessionService
      .TranscriptWindow(session.key,destination.at,destination.before)
      .then(value=>{if(version===request.current)setTr(value)})
      .catch((e) => {if(version===request.current)setErr(String(e).slice(0, 300))})
      .finally(() => {if(version===request.current)setLoading(false)})
  }, [session.key, sourceVersion, destination.at, destination.before, destination.revision])

  useEffect(() => { load();return()=>{request.current++} }, [load])

  useEffect(() => {
    if (!tr || loading || err) return
    const frame = requestAnimationFrame(() => {
      if(hitPart>=0)hitRef.current?.scrollIntoView({block:'center'})
      else if(destination.at<0&&latestRef.current)latestRef.current.scrollIntoView({block:'end'})
      else if(bodyRef.current)bodyRef.current.scrollTop=destination.at<0?bodyRef.current.scrollHeight:0
    })
    return () => cancelAnimationFrame(frame)
  }, [hitPart, destination.at, destination.revision, tr, loading, err])

  const copy = (text: string, tag: string) => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(tag)
      setTimeout(() => setCopied(''), 1500)
    }).catch(() => {})
  }

  const exportMd = () => {
    Dialogs.OpenFile({
        CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true,
        Title: '选择 Markdown 导出目录', ButtonText: '导出到此目录',
      }).then((dir) => {
        if (!dir || typeof dir !== 'string') return
        sessionService.ExportMarkdown(session.key, dir).then(
          (p) => copy(String(p), 'export'),
          (reason) => setActionError(`导出失败：${String(reason)}`),
        )
      }).catch(reason => setActionError(`导出失败：${String(reason)}`))
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.head}>
        <div className={styles.headRow}>
          <Text title={session.title || '未命名会话'} className={styles.title} weight="semibold">{session.title || '未命名会话'}</Text>
          <div className={styles.headActions}>
            <Tooltip content={copied === 'title' ? '已复制标题' : '复制标题'} relationship="label">
              <Button
                appearance="subtle" size="small"
                icon={copied === 'title' ? <CheckmarkRegular /> : <CopyRegular />}
                onClick={() => copy(session.title, 'title')}
              />
            </Tooltip>
            <Tooltip content="导出为 Markdown" relationship="label">
              <Button size="small" appearance="subtle" icon={<ArrowDownloadRegular />} onClick={exportMd} />
            </Tooltip>
            <Tooltip content={infoOpen ? '收起会话详情' : '展开会话详情'} relationship="label">
              <Button size="small" appearance="subtle" icon={<PanelRightRegular />} aria-expanded={infoOpen} aria-controls="session-details" onClick={onToggleInfo} />
            </Tooltip>
          </div>
        </div>
        <div className={styles.chips}>
          <span className="ad-reader-agent"><AgentMark agent={session.agent} size={14} />{m.label}</span>
          {session.project ? <span className="ad-reader-context" title={session.project}>{session.project}</span> : null}
          {session.account ? <span className="ad-reader-context" title={`账号：${session.account}`}>{session.account}</span> : null}
          <span className="ad-reader-count">{session.messages} 条消息</span>
          <button className={styles.chipAdd} onClick={onOpenInfo} title="在会话信息中管理标签"><AddRegular fontSize={12} />{session.tags.length ? `${session.tags.length} 个标签` : '标签'}</button>
        </div>
      </div>

      <nav className="ad-reader-nav" aria-label="会话阅读导航">
        <Button size="small" appearance="subtle" icon={<ArrowUpRegular />} onClick={()=>navigate(0)} disabled={loading}>会话开头</Button>
        <Tooltip content="更早的内容" relationship="label"><Button size="small" appearance="subtle" icon={<ChevronLeftRegular />} aria-label="阅读更早内容" disabled={loading||!tr?.has_before} onClick={()=>navigate(tr?.start??0,true)} /></Tooltip>
        <span role="status" className="ad-reader-range">{loading?'正在载入…':tr?.total?`${tr.start+1}–${tr.start+tr.parts.length} / ${tr.total} 段`:'暂无正文'}</span>
        <Tooltip content="较新的内容" relationship="label"><Button size="small" appearance="subtle" icon={<ChevronRightRegular />} aria-label="阅读较新内容" disabled={loading||!tr?.has_after} onClick={()=>navigate((tr?.start??0)+(tr?.parts.length??0))} /></Tooltip>
        <Button size="small" appearance={destination.at<0?'secondary':'subtle'} icon={<ArrowDownRegular />} onClick={()=>navigate(-1)} disabled={loading}>最新消息</Button>
      </nav>
      <div ref={bodyRef} className={`ad-reading-body ${styles.body}`}>
        {tr?.storage_warning ? <Text role="alert" size={200} block style={{ marginBottom: 12, color: tokens.colorPaletteRedForeground1 }}>{tr.storage_warning}</Text> : null}
        {tr?.retained ? <Text size={200} block style={{ marginBottom: 12, color: tokens.colorNeutralForeground3 }}>正在阅读 AgentDeck 永久保留的本地副本，上游原记录暂不可用。</Text> : null}
        {tr && !loading && hitPart >= 0 && (hitPart < tr.start || hitPart >= tr.start+tr.parts.length || tr.parts[hitPart-tr.start]?.cut) ? <Text size={200} block style={{ marginBottom: 12, color: tokens.colorNeutralForeground3 }}>本段包含预览范围之外的内容，可导出完整 Markdown 查看。</Text> : null}
        {tr?.cut ? <Text size={200} block style={{ marginBottom: 12, color: tokens.colorNeutralForeground3 }}>会话预览达到读取上限，部分内容未载入。</Text> : null}
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 24 }}><Spinner size="tiny" /></div>
        ) : err ? (
          <div className={styles.errRow}>
            <Text size={300} style={{ color: tokens.colorPaletteRedForeground1, overflowWrap: 'anywhere', minWidth: 0 }}>
              {err}
            </Text>
            <Button size="small" icon={<ArrowClockwiseRegular />} onClick={load} style={{ flexShrink: 0 }}>重试</Button>
          </div>
        ) : tr ? (
          tr.parts.map((p, i) => {
            const partIndex=(tr.start??0)+i
            const hit = partIndex === hitPart
            const setHitRef = (el: HTMLElement | null) => { if (hit) hitRef.current = el; if(i===latestIndex)latestRef.current=el }
            const isUser = p.role === 'user'
            const folded = foldedContent(p)
            const time = (p as { at?: string }).at
            return (
              <div key={partIndex} data-part-index={partIndex} ref={setHitRef} className={mergeClasses('ad-message', folded ? styles.processMsg : styles.msg, hit && styles.hit)}>
                  {!folded ? <div className={styles.avatarCol}>
                    {isUser ? (
                      <span className={styles.userAvatar}><PersonRegular fontSize={15} /></span>
                    ) : p.role === 'tool' ? (
                      <span className={styles.userAvatar}><WrenchRegular fontSize={15} /></span>
                    ) : (
                      <AgentMark agent={session.agent} size={26} />
                    )}
                  </div> : null}
                  <div className={mergeClasses(styles.msgBody, !folded && isUser && styles.userMessage)}>
                    {!folded ? <div className={styles.msgHead}>
                      <span className={styles.roleName}>{isUser ? '你' : p.role === 'tool' ? '工具' : m.label}</span>
                      {time ? <span className={styles.msgTime}>{fmtTime(time)}</span> : null}
                      {p.model && !isUser ? <span className={styles.msgTime}>{p.model}</span> : null}
                      <button className={styles.msgCopy} onClick={() => copy(p.text, String(i))} title="复制此段">
                        {copied === String(i) ? <CheckmarkRegular fontSize={13} /> : <CopyRegular fontSize={13} />}
                      </button>
                    </div> : null}
                    {folded ? (
                      <ProcessContent content={folded} reveal={hit} revealToken={destination.revision} cut={p.cut} styles={styles} meta={[p.role === 'tool' ? '工具' : m.label, time ? fmtTime(time) : '', p.model].filter(Boolean).join(' · ')}>
                        {folded.monospace ? <CodeArea raw={p.text} styles={styles} theme={theme} head="" scroll foldLong={false} /> : renderRichText(p.text, styles, theme, hit, destination.revision)}
                      </ProcessContent>
                    ) : renderRichText(p.text, styles, theme, hit, destination.revision)}
                    {p.omitted_before ? <Text size={200} style={{color:tokens.colorNeutralForeground3}}>本段前 {p.omitted_before} 个字符未载入，当前展示末尾内容。</Text> : null}
                    {!folded && p.cut ? <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>此段还有 {p.cut} 个字符未载入</Text> : null}
                  </div>
                  {folded ? <button className={styles.msgCopy} style={{ marginTop: 9 }} onClick={() => copy(p.text, String(i))} title="复制原文" aria-label={`复制${folded.label}原文`}>
                    {copied === String(i) ? <CheckmarkRegular fontSize={13} /> : <CopyRegular fontSize={13} />}
                  </button> : null}
              </div>
            )
          })
        ) : null}
      </div>

      {actionError ? <div role="alert" style={{ padding: '8px 24px', color: tokens.colorPaletteRedForeground1, fontSize: 12 }}>{actionError}</div> : null}
      <div className={`ad-message-actions ${styles.bottom}`}>
        {session.resume && !tr?.retained ? (
          <>
            <Tooltip content={`在 ${m.label} 终端中继续此会话`} relationship="label">
              <Button size="small" appearance="primary" icon={<OpenRegular />} disabled={actionBusy}
                onClick={() => void runAction(() => resumeService.OpenInTerminal(session.key))}>{actionBusy ? '正在打开…' : '继续会话'}</Button>
            </Tooltip>
            <Button size="small" appearance="subtle" icon={<CopyRegular />} disabled={actionBusy}
              onClick={() => void runAction(() => resumeService.CopyToClipboard(session.key))}>复制恢复命令</Button>
          </>
        ) : (
          <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>此记录支持阅读、检索与导出</Text>
        )}
        <Button size="small" appearance="subtle" icon={<ArrowDownloadRegular />} onClick={exportMd}>导出 Markdown</Button>
      </div>
    </div>
  )
}

// Markdown/GFM 排版，代码块保留行号、高亮、复制和按需展开。

type Styles = ReturnType<typeof useStyles>

function ProcessContent({ content, reveal, revealToken, cut, styles, meta, children }: { content: FoldedContent; reveal: boolean; revealToken: number; cut?: number; styles: Styles; meta: string; children: ReactNode }) {
  const {preferences}=usePreferences()
  const [expanded, setExpanded] = useState(reveal||!preferences.foldProcesses)
  useEffect(()=>setExpanded(reveal||!preferences.foldProcesses),[preferences.foldProcesses])
  useEffect(() => { if (reveal) setExpanded(true) }, [reveal, revealToken])
  return <details className={mergeClasses('ad-part-fold', styles.fold)} open={expanded} onToggle={event => setExpanded(event.currentTarget.open)}>
    <summary className={styles.foldSummary} title={`${content.label} · ${meta}`}>
      <ChevronRightRegular className="ad-fold-chevron" />
      <span className={styles.foldLabel}>{content.label}</span>
      <span className={styles.foldPreview}>{content.preview}</span>
      <span className={styles.foldCount}>{content.lines} 行{cut ? ' · 已截断' : ''}</span>
    </summary>
    {expanded ? <div className={styles.foldBody}>{children}{cut ? <Text size={200} block>此段还有 {cut} 个字符未载入</Text> : null}</div> : null}
  </details>
}

type MarkdownSettings = { styles: Styles; theme: 'light' | 'dark'; reveal: boolean; revealToken: number; openLink: (url: string) => void }
const MarkdownContext = createContext<MarkdownSettings | null>(null)
const markdownPlugins = [remarkGfm]

function MarkdownCode({ className, children }: ComponentProps<'code'>) {
  const context = useContext(MarkdownContext)
  const raw = String(children ?? '')
  return context && (className?.startsWith('language-') || raw.endsWith('\n')) ? (
    <CodeArea raw={markdownCodeText(raw)} styles={context.styles} theme={context.theme} head={className?.replace(/^language-/, '') || ''} reveal={context.reveal} revealToken={context.revealToken} />
  ) : <code>{children}</code>
}
function MarkdownLink({ href, children }: ComponentProps<'a'>) {
  const context = useContext(MarkdownContext)
  if (href?.startsWith('#')) return <a href={href}>{children}</a>
  if (!href || !/^https?:\/\//i.test(href)) return <span>{children}</span>
  return <a href={href} title={href} onClick={event => { event.preventDefault(); context?.openLink(href) }}>{children}</a>
}
function MarkdownImage({ src, alt }: ComponentProps<'img'>) {
  return <span className="ad-md-image">图片：{alt || '附件'}{typeof src === 'string' && /^https?:\/\//i.test(src) ? <> · <MarkdownLink href={src}>查看图片</MarkdownLink></> : null}</span>
}
const markdownComponents: Components = {
  pre: ({ children }) => <div className="ad-md-code">{children}</div>,
  code: MarkdownCode,
  a: MarkdownLink,
  img: MarkdownImage,
  table: ({ children }) => <div className="ad-md-table" tabIndex={0} role="region" aria-label="表格，可横向滚动"><table>{children}</table></div>,
}
function MarkdownContent({ text, styles, theme, reveal, revealToken }: { text: string; styles: Styles; theme: 'light' | 'dark'; reveal: boolean; revealToken: number }) {
  const [error, setError] = useState('')
  useEffect(() => setError(''), [text])
  const rendered = useMemo(() => <Markdown remarkPlugins={markdownPlugins} components={markdownComponents}>{text}</Markdown>, [text])
  return <MarkdownContext.Provider value={{ styles, theme, reveal, revealToken, openLink: url => { void Browser.OpenURL(url).catch(reason => setError(`链接打开失败：${String(reason).slice(0, 160)}`)) } }}>
    <div className="ad-markdown">{rendered}</div>
    {error ? <Text size={200} role="alert" style={{ color: tokens.colorPaletteRedForeground1 }}>{error}</Text> : null}
  </MarkdownContext.Provider>
}
function renderRichText(text: string, styles: Styles, theme: 'light' | 'dark', reveal = false, revealToken = 0) {
  return <MarkdownContent text={text} styles={styles} theme={theme} reveal={reveal} revealToken={revealToken} />
}

// CodeArea:等宽代码区(标题栏 + 语言徽标 + 行号槽 + 关键字/字符串/注释/数字着色 + 复制)。
// head 显式给空串表示"无标题"(工具输出),此时标题栏只显示占位标签与复制。
function CodeArea({ raw, styles, theme, head, scroll, reveal = false, revealToken = 0, foldLong = true }: {
  raw: string
  styles: Styles
  theme: 'light' | 'dark'
  head?: string
  scroll?: boolean
  reveal?: boolean
  revealToken?: number
  foldLong?: boolean
}) {
  const [copied, setCopied] = useState(false)

  const title = head ?? ''
  const body = raw
  // 围栏头为语言名时显示徽标，文件路径则显示文件名。
  let lang = ''
  let name = title
  if (title) {
    const lower = title.toLowerCase()
    const ext = lower.includes('.') ? lower.slice(lower.lastIndexOf('.') + 1) : ''
    if (!lower.includes('.') && !lower.includes('/') && LANG_ALIAS[lower]) {
      lang = LANG_ALIAS[lower]
      name = ''
    } else {
      lang = LANG_ALIAS[ext] ?? ''
    }
  }
  const lines = body.split('\n')
  const bodyId = useId()
  const long = foldLong && isLongCode(body)
  const [expanded, setExpanded] = useState(reveal)
  useEffect(() => { if (reveal) setExpanded(true) }, [reveal, revealToken])
  const shown = !long || expanded

  const copy = () => {
    navigator.clipboard?.writeText(body).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }).catch(() => {})
  }

  return (
    <div className={styles.codeBlock}>
      <div className={styles.codeHead}>
        {lang ? <span className={styles.langBadge}>{lang}</span> : null}
        {name ? <span className={styles.codeName} title={name}>{name}</span> : <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>{head === '' ? '原始内容' : '代码'}</Text>}
        <span className={styles.foldCount}>{lines.length} 行</span>
        {long ? <button className={styles.codeToggle} aria-expanded={shown} aria-controls={bodyId} onClick={() => setExpanded(value => !value)}>{shown ? '收起代码' : '展开代码'}</button> : null}
        <button className={styles.codeCopy} onClick={copy}>
          {copied ? <CheckmarkRegular fontSize={12} /> : <CopyRegular fontSize={12} />}
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <div id={bodyId} hidden={!shown}>{shown ? <div className={styles.codeBody} style={scroll ? { maxHeight: 360, overflowY: 'auto' } : undefined}>
        <div className={styles.gutter}>
          {lines.map((_, i) => <div key={i}>{i + 1}</div>)}
        </div>
        <div className={styles.codeText}>
          {lines.map((l, i) => <div key={i}>{l ? highlight(l, theme) : ' '}</div>)}
        </div>
      </div> : null}</div>
    </div>
  )
}

// 语言/扩展名 → 标题栏徽标文字(概念图里的 [JS] / [GO] 小色块)。
const LANG_ALIAS: Record<string, string> = {
  js: 'JS', mjs: 'JS', cjs: 'JS', javascript: 'JS',
  jsx: 'JSX', ts: 'TS', tsx: 'TSX', typescript: 'TS',
  go: 'GO', py: 'PY', python: 'PY', rb: 'RB', rs: 'RS', java: 'JAVA',
  kt: 'KT', swift: 'SWIFT', cs: 'C#', cpp: 'C++', cc: 'C++', c: 'C', h: 'C',
  sh: 'SH', bash: 'SH', zsh: 'SH', shell: 'SH', ps1: 'PS', powershell: 'PS',
  json: 'JSON', jsonc: 'JSON', yml: 'YAML', yaml: 'YAML', toml: 'TOML', ini: 'INI',
  md: 'MD', markdown: 'MD', sql: 'SQL', html: 'HTML', htm: 'HTML', css: 'CSS',
  scss: 'SCSS', xml: 'XML', diff: 'DIFF', patch: 'DIFF', txt: 'TXT', text: 'TXT',
}
