// 主区:会话消息流。对齐概念图:大标题 + chips 行 + 头像消息流(用户圆头像 /
// Agent 品牌方块)+ Markdown 围栏代码块(文件名头 + 行号 + 复制)+ 底部操作条。
import {
  makeStyles,
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
} from '@fluentui/react-icons'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { SessionView, TranscriptView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import { agentMeta, fmtTime } from '../util'
import { AgentMark } from '../icons/AgentMark'
import { highlight } from '../highlight'
import { useTheme } from '../hooks/useTheme'

const useStyles = makeStyles({
  wrap: {
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    minHeight: 0,
    backgroundColor: tokens.colorNeutralBackground1,
  },
  // ---- 头部:标题 + chips ----
  head: {
    padding: '14px 24px 10px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    display: 'flex',
    flexDirection: 'column',
    gap: '10px',
  },
  headRow: { display: 'flex', alignItems: 'flex-start', gap: '12px' },
  title: { flex: 1, minWidth: 0, fontSize: '18px', lineHeight: 1.35, overflowWrap: 'anywhere' },
  headActions: { display: 'flex', gap: '4px', flexShrink: 0 },
  chips: { display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap' },
  chip: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '4px',
    fontSize: '12px',
    padding: '2px 8px',
    ...shorthands.borderRadius('6px'),
    backgroundColor: tokens.colorNeutralBackground3,
    color: tokens.colorNeutralForeground2,
  },
  chipAgent: {
    backgroundColor: tokens.colorNeutralBackground1,
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke2),
    color: tokens.colorNeutralForeground1,
  },
  chipAdd: {
    fontSize: '12px',
    padding: '2px 8px',
    ...shorthands.borderRadius('6px'),
    ...shorthands.border('1px', 'dashed', tokens.colorNeutralStroke1),
    color: tokens.colorNeutralForeground3,
  },
  // ---- 消息流 ----
  body: { flex: 1, overflowY: 'auto', padding: '18px 24px 8px', minHeight: 0 },
  msg: { display: 'flex', gap: '12px', marginBottom: '22px' },
  avatarCol: { flexShrink: 0, paddingTop: '2px' },
  userAvatar: {
    width: '32px',
    height: '32px',
    ...shorthands.borderRadius('50%'),
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: tokens.colorNeutralBackground4,
    color: tokens.colorNeutralForeground2,
  },
  msgBody: { flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: '6px' },
  msgHead: { display: 'flex', alignItems: 'center', gap: '8px' },
  roleName: { fontWeight: 600, fontSize: '13px' },
  msgTime: { fontSize: '12px', color: tokens.colorNeutralForeground3 },
  msgCopy: {
    marginLeft: 'auto',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    padding: '2px',
    ...shorthands.borderRadius('4px'),
    color: tokens.colorNeutralForeground3,
    opacity: 0,
    ':focus': { opacity: 1 },
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
    backgroundColor: tokens.colorNeutralBackground3,
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
  thinking: {
    backgroundColor: tokens.colorNeutralBackground2,
    ...shorthands.borderRadius('8px'),
    padding: '8px 12px',
    fontSize: '13px',
    color: tokens.colorNeutralForeground2,
  },
  thinkingSummary: { cursor: 'pointer', fontSize: '12px', color: tokens.colorNeutralForeground3 },
  hit: { backgroundColor: tokens.colorBrandBackground2, ...shorthands.borderRadius('8px'), padding: '8px 10px', margin: '-8px -10px' },
  // ---- 底部操作条(概念图:居中一排胶囊按钮)----
  bottom: {
    padding: '10px 24px 14px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
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
  jumpPart: number // 搜索命中定位到 transcript 第 n 段(-1 = 不定位)
  onOpenInfo: () => void
}

export function MessagePane({ session, jumpPart, onOpenInfo }: Props) {
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
  return <Loaded key={session.key} session={session} jumpPart={jumpPart} onOpenInfo={onOpenInfo} />
}

function Loaded({ session, jumpPart, onOpenInfo }: { session: SessionView; jumpPart: number; onOpenInfo: () => void }) {
  const styles = useStyles()
  const theme = useTheme()
  const m = agentMeta(session.agent)
  const [tr, setTr] = useState<TranscriptView | null>(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState('')
  const hitRef = useRef<HTMLElement | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setErr('')
    sessionService
      .Transcript(session.key)
      .then(setTr)
      .catch((e) => setErr(String(e).slice(0, 300)))
      .finally(() => setLoading(false))
  }, [session.key])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    if (jumpPart >= 0 && tr && !loading) hitRef.current?.scrollIntoView({ block: 'center' })
  }, [jumpPart, tr, loading])

  const copy = (text: string, tag: string) => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(tag)
      setTimeout(() => setCopied(''), 1500)
    }).catch(() => {})
  }

  const exportMd = () => {
    import('@wailsio/runtime').then(({ Dialogs }) => {
      Dialogs.OpenFile({
        CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true,
        Title: '选择 Markdown 导出目录', ButtonText: '在此导出',
      }).then((dir) => {
        if (!dir || typeof dir !== 'string') return
        sessionService.ExportMarkdown(session.key, dir).then(
          (p) => copy(String(p), 'export'),
          () => {},
        )
      }).catch(() => {})
    })
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.head}>
        <div className={styles.headRow}>
          <Text className={styles.title} weight="semibold">{session.title || '(无标题)'}</Text>
          <div className={styles.headActions}>
            <Tooltip content={copied === 'title' ? '已复制标题' : '复制标题'} relationship="description">
              <Button
                appearance="subtle"
                icon={copied === 'title' ? <CheckmarkRegular /> : <CopyRegular />}
                onClick={() => copy(session.title, 'title')}
              />
            </Tooltip>
            <Tooltip content="导出为 Markdown" relationship="description">
              <Button appearance="subtle" icon={<ArrowDownloadRegular />} onClick={exportMd} />
            </Tooltip>
            <Tooltip content="会话信息面板" relationship="description">
              <Button appearance="subtle" icon={<OpenRegular />} onClick={onOpenInfo} />
            </Tooltip>
          </div>
        </div>
        <div className={styles.chips}>
          {session.project ? <span className={styles.chip}>{session.project}</span> : null}
          {session.tags.map((t) => <span key={t} className={styles.chip}>{t}</span>)}
          <span className={styles.chipAdd}>+ 标签</span>
          <span className={styles.chip + ' ' + styles.chipAgent}>
            <AgentMark agent={session.agent} size={14} />
            {m.label}
          </span>
        </div>
      </div>

      <div className={styles.body}>
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
            const hit = i === jumpPart
            const setHitRef = (el: HTMLElement | null) => { if (hit) hitRef.current = el }
            const isUser = p.role === 'user'
            const isThinking = p.kind === 'thinking' || p.role === 'thinking'
            const isToolish = p.kind === 'tool_use' || p.kind === 'tool_result' || p.kind === 'context'
            const time = (p as { at?: string }).at
            return (
              <div key={i} ref={setHitRef} className={styles.msg + (hit ? ' ' + styles.hit : '')}>
                  <div className={styles.avatarCol}>
                    {isUser ? (
                      <span className={styles.userAvatar}><PersonRegular fontSize={18} /></span>
                    ) : (
                      <AgentMark agent={session.agent} size={30} />
                    )}
                  </div>
                  <div className={styles.msgBody}>
                    <div className={styles.msgHead}>
                      <span className={styles.roleName}>{isUser ? '你' : m.label}</span>
                      {time ? <span className={styles.msgTime}>{fmtTime(time)}</span> : null}
                      {p.model && !isUser ? <span className={styles.msgTime}>{p.model}</span> : null}
                      <button className={styles.msgCopy} onClick={() => copy(p.text, String(i))} title="复制此段">
                        {copied === String(i) ? <CheckmarkRegular fontSize={13} /> : <CopyRegular fontSize={13} />}
                      </button>
                    </div>
                    {isThinking ? (
                      <details className={styles.thinking} open={hit ? true : undefined}>
                        <summary className={styles.thinkingSummary}>思考过程(点击展开)</summary>
                        <div className={styles.text} style={{ marginTop: 6 }}>{p.text}</div>
                      </details>
                    ) : isToolish ? (
                      <CodeArea raw={p.text} styles={styles} theme={theme} head="" scroll />
                    ) : (
                      renderRichText(p.text, styles, theme)
                    )}
                  </div>
              </div>
            )
          })
        ) : null}
      </div>

      <div className={styles.bottom}>
        {session.resume ? (
          <>
            <Button size="small" appearance="primary" icon={<CommentMultipleRegular />}
              onClick={() => void resumeService.OpenInTerminal(session.key)}>继续对话</Button>
            <Button size="small" icon={<OpenRegular />}
              onClick={() => void resumeService.OpenInTerminal(session.key)}>在终端中打开</Button>
          </>
        ) : (
          <Text size={200} style={{ color: tokens.colorPaletteYellowForeground1 }}>该会话不可恢复,仅可查看历史</Text>
        )}
        <Button size="small" icon={<ArrowDownloadRegular />} onClick={exportMd}>导出为 Markdown</Button>
        <Tooltip content="v1.1 提供,当前版本未实现" relationship="description">
          <Button size="small" disabled>交接到其他 Agent</Button>
        </Tooltip>
        {!session.resume ? (
          <Button size="small" icon={<CopyRegular />}
            onClick={() => void resumeService.CopyToClipboard(session.key)}>复制恢复命令</Button>
        ) : null}
      </div>
    </div>
  )
}

// ---- Markdown 轻解析:围栏代码块(带文件名/language 头 + 行号 + 高亮)与普通文本 ----

type Styles = ReturnType<typeof useStyles>

function renderRichText(text: string, styles: Styles, theme: 'light' | 'dark') {
  if (!text.includes('```')) return <div className={styles.text}>{text}</div>
  const chunks = text.split('```')
  return (
    <>
      {chunks.map((chunk, i) =>
        i % 2 === 1 ? <CodeArea key={i} raw={chunk} styles={styles} theme={theme} /> : (
          chunk.trim() ? <div key={i} className={styles.text}>{chunk.trim()}</div> : null
        ),
      )}
    </>
  )
}

// CodeArea:等宽代码区(标题栏 + 语言徽标 + 行号槽 + 关键字/字符串/注释/数字着色 + 复制)。
// head 显式给空串表示"无标题"(工具输出),此时标题栏只显示占位标签与复制。
function CodeArea({ raw, styles, theme, head, scroll }: {
  raw: string
  styles: Styles
  theme: 'light' | 'dark'
  head?: string
  scroll?: boolean
}) {
  const [copied, setCopied] = useState(false)

  // head 未显式给出时:围栏首行若像语言标识/文件名(无空格),当作标题行
  let title = head
  let body = raw.replace(/^\n+|\n+$/g, '')
  if (title === undefined) {
    const lines = body.split('\n')
    const first = (lines[0] ?? '').trim()
    if (/^[\w./@-]{1,60}$/.test(first)) {
      title = first
      body = lines.slice(1).join('\n')
    } else {
      title = ''
    }
  }
  // 首行是纯语言词 → 只作徽标,不再重复显示成文件名
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
        {name ? <span className={styles.codeName}>{name}</span> : <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>输出</Text>}
        <button className={styles.codeCopy} onClick={copy}>
          {copied ? <CheckmarkRegular fontSize={12} /> : <CopyRegular fontSize={12} />}
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <div className={styles.codeBody} style={scroll ? { maxHeight: 360, overflowY: 'auto' } : undefined}>
        <div className={styles.gutter}>
          {lines.map((_, i) => <div key={i}>{i + 1}</div>)}
        </div>
        <div className={styles.codeText}>
          {lines.map((l, i) => <div key={i}>{l ? highlight(l, theme) : ' '}</div>)}
        </div>
      </div>
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
