// 右栏:会话信息面板。对齐概念图:顶部 tab、元数据键值、标签、备注、
// 摘要(v1.1 置灰)、操作按钮组。
import {
  makeStyles,
  shorthands,
  tokens,
  Text,
  Button,
  Input,
  Textarea,
  Tooltip,
} from '@fluentui/react-components'
import {
  DismissRegular,
  CopyRegular,
  CheckmarkRegular,
  ArrowDownloadRegular,
  OpenRegular,
  ArrowSyncRegular,
  ChevronDownRegular,
  AddRegular,
  DeleteRegular,
  DocumentTextRegular,
} from '@fluentui/react-icons'
import { useEffect, useState } from 'react'
import { Dialogs } from '@wailsio/runtime'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as resumeService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/resumeservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentMeta, fmtTime, relTime } from '../util'
import { AgentAvatar } from './AgentAvatar'

const useStyles = makeStyles({
  wrap: {
    height: '100%',
    width: '310px',
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
    backgroundColor: tokens.colorNeutralBackground1,
    borderLeft: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  tabs: {
    display: 'flex',
    alignItems: 'center',
    gap: '2px',
    padding: '0 10px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  tab: {
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    fontSize: '13px',
    padding: '10px 8px',
    color: tokens.colorNeutralForeground3,
    borderBottom: '2px solid transparent',
  },
  tabOn: { color: tokens.colorBrandForeground1, borderBottom: `2px solid ${tokens.colorBrandBackground}`, fontWeight: 600 },
  tabOff: { color: tokens.colorNeutralForeground4, cursor: 'default' },
  body: { flex: 1, overflowY: 'auto', padding: '14px 16px', display: 'flex', flexDirection: 'column', gap: '16px' },
  section: { display: 'flex', flexDirection: 'column', gap: '8px' },
  sectionHead: { display: 'flex', alignItems: 'center', gap: '6px' },
  sectionTitle: { fontSize: '13px', fontWeight: 600 },
  linkBtn: {
    marginLeft: 'auto', border: 'none', background: 'transparent', cursor: 'pointer',
    fontSize: '12px', color: tokens.colorBrandForeground1, padding: 0,
  },
  titleRow: { display: 'flex', alignItems: 'flex-start', gap: '8px' },
  paneTitle: { flex: 1, minWidth: 0, fontSize: '14px', lineHeight: 1.4, overflowWrap: 'anywhere', fontWeight: 600 },
  agentRow: {
    display: 'flex', alignItems: 'center', gap: '8px',
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3),
    ...shorthands.borderRadius('6px'),
    padding: '6px 10px',
  },
  kv: { display: 'grid', gridTemplateColumns: '84px minmax(0, 1fr)', gap: '6px 10px', fontSize: '12.5px' },
  k: { color: tokens.colorNeutralForeground3 },
  v: { color: tokens.colorNeutralForeground1, overflowWrap: 'anywhere' },
  chips: { display: 'flex', flexWrap: 'wrap', gap: '6px', alignItems: 'center' },
  chip: {
    display: 'inline-flex', alignItems: 'center', gap: '4px',
    fontSize: '12px', padding: '2px 4px 2px 8px',
    ...shorthands.borderRadius('6px'),
    backgroundColor: tokens.colorNeutralBackground3,
    color: tokens.colorNeutralForeground1,
  },
  chipX: { border: 'none', background: 'transparent', cursor: 'pointer', padding: '0 2px', color: tokens.colorNeutralForeground3, display: 'flex' },
  chipAdd: {
    display: 'inline-flex', alignItems: 'center', gap: '2px',
    fontSize: '12px', padding: '2px 8px',
    ...shorthands.borderRadius('6px'),
    ...shorthands.border('1px', 'dashed', tokens.colorNeutralStroke1),
    color: tokens.colorNeutralForeground3,
  },
  summaryBox: {
    backgroundColor: tokens.colorNeutralBackground2,
    ...shorthands.borderRadius('6px'),
    padding: '8px 10px',
    fontSize: '12.5px',
    color: tokens.colorNeutralForeground3,
    lineHeight: 1.6,
  },
  actions: { display: 'flex', flexDirection: 'column', gap: '8px' },
  actionRow: { display: 'flex', gap: '8px' },
  actionBtn: { flex: 1 },
  grayed: { opacity: 0.5 },
  empty: {
    padding: '72px 24px 24px',
    textAlign: 'center',
    color: tokens.colorNeutralForeground3,
  },
})

// fmtTokens 紧凑显示 token 数(百万级会话很常见)。
function fmtTokens(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
  if (n >= 1e3) return `${(n / 1e3).toFixed(1)}K`
  return String(n)
}

type Props = {
  session: SessionView | null
  onChanged: () => void
  onClose: () => void
}

export function InfoPane({ session, onChanged, onClose }: Props) {
  const styles = useStyles()
  const [tagDraft, setTagDraft] = useState('')
  const [adding, setAdding] = useState(false)
  const [noteDraft, setNoteDraft] = useState(session?.note ?? '')
  const [copied, setCopied] = useState('')
  const [msg, setMsg] = useState('')

  useEffect(() => { setNoteDraft(session?.note ?? '') }, [session?.key, session?.note])

  if (!session) {
    return (
      <div className={styles.wrap}>
        <div className={styles.tabs}>
          <button className={styles.tab + ' ' + styles.tabOn}>会话信息</button>
        </div>
        <div className={styles.empty}>
          <DocumentTextRegular fontSize={24} />
          <Text size={200} style={{ display: 'block', marginTop: 8 }}>选中会话后显示其元数据、标签与操作</Text>
        </div>
      </div>
    )
  }

  const m = agentMeta(session.agent)

  const copy = (text: string, tag: string) => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(tag)
      setTimeout(() => setCopied(''), 1500)
    }).catch(() => {})
  }

  const addTag = () => {
    const t = tagDraft.trim()
    if (!t || session.tags.includes(t)) { setTagDraft(''); setAdding(false); return }
    markService.SetTags(session.key, [...session.tags, t]).then(() => {
      setTagDraft(''); setAdding(false); onChanged()
    }).catch(() => {})
  }
  const removeTag = (t: string) => {
    markService.SetTags(session.key, session.tags.filter((x) => x !== t)).then(onChanged).catch(() => {})
  }
  const saveNote = () => {
    markService.SetNote(session.key, noteDraft).then(() => { setMsg('备注已保存'); onChanged() }).catch((e) => setMsg(String(e)))
  }
  const toggleStar = () => {
    markService.SetFavorite(session.key, !session.favorite).then(onChanged).catch(() => {})
  }
  const toggleArchive = () => {
    markService.SetArchived(session.key, !session.archived).then(onChanged).catch(() => {})
  }
  const exportMd = () => {
    Dialogs.OpenFile({
      CanChooseDirectories: true, CanChooseFiles: false, CanCreateDirectories: true,
      Title: '选择 Markdown 导出目录', ButtonText: '在此导出',
    }).then((dir) => {
      if (!dir || typeof dir !== 'string') return
      sessionService.ExportMarkdown(session.key, dir).then(
        (p) => setMsg(`已导出:${p}`),
        (e) => setMsg(`导出失败:${String(e).slice(0, 120)}`),
      )
    }).catch(() => {})
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.tabs}>
        <button className={styles.tab + ' ' + styles.tabOn}>会话信息</button>
        <Tooltip content="v1.1 提供" relationship="description">
          <button className={styles.tab + ' ' + styles.tabOff} disabled>项目</button>
        </Tooltip>
        <Tooltip content="v1.1 提供" relationship="description">
          <button className={styles.tab + ' ' + styles.tabOff} disabled>文件</button>
        </Tooltip>
        <Tooltip content="v1.1 提供" relationship="description">
          <button className={styles.tab + ' ' + styles.tabOff} disabled>工具调用</button>
        </Tooltip>
        <Button appearance="subtle" size="small" icon={<DismissRegular />} onClick={onClose} title="收起信息面板" style={{ marginLeft: 'auto' }} />
      </div>

      <div className={styles.body}>
        <div className={styles.titleRow}>
          <Text className={styles.paneTitle}>{session.title || '(无标题)'}</Text>
          <Button
            appearance="subtle" size="small"
            icon={copied === 'title' ? <CheckmarkRegular /> : <CopyRegular />}
            onClick={() => copy(session.title, 'title')}
            title="复制标题"
          />
        </div>

        <div className={styles.agentRow}>
          <AgentAvatar agent={session.agent} size={18} />
          <Text size={300}>{m.label}{session.source === 'wsl' ? '(WSL)' : ''}</Text>
          <ChevronDownRegular fontSize={12} style={{ marginLeft: 'auto', color: tokens.colorNeutralForeground3 }} />
        </div>

        <div className={styles.kv}>
          <span className={styles.k}>创建时间</span><span className={styles.v}>{fmtTime(session.created)}</span>
          <span className={styles.k}>最后更新</span><span className={styles.v}>{relTime(session.last)} · {fmtTime(session.last)}</span>
          <span className={styles.k}>消息数量</span><span className={styles.v}>{session.messages}</span>
          {session.tokens > 0 ? (<>
            <span className={styles.k}>Token 消耗</span>
            <span className={styles.v}>约 {fmtTokens(session.tokens)}</span>
          </>) : null}
          <span className={styles.k}>项目</span><span className={styles.v}>{session.project || '项目未知'}</span>
          {session.cwd ? (<>
            <span className={styles.k}>工作目录</span><span className={styles.v}>{session.cwd}</span>
          </>) : null}
          {session.source_path ? (<>
            <span className={styles.k}>源路径</span><span className={styles.v}>{session.source_path}</span>
          </>) : null}
          <span className={styles.k}>恢复能力</span>
          <span className={styles.v} style={{ color: session.resume ? undefined : tokens.colorPaletteYellowForeground1 }}>
            {session.resume ? '可恢复' : '仅可查看历史'}
          </span>
        </div>

        <div className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.sectionTitle}>标签</span>
            <button
              className={styles.linkBtn}
              onClick={toggleStar}
            >
              {session.favorite ? '★ 已收藏' : '☆ 收藏'}
            </button>
          </div>
          <div className={styles.chips}>
            {session.tags.map((t) => (
              <span key={t} className={styles.chip}>
                {t}
                <button className={styles.chipX} onClick={() => removeTag(t)} title={`移除 ${t}`}>
                  <DismissRegular fontSize={11} />
                </button>
              </span>
            ))}
            {adding ? (
              <Input
                size="small" autoFocus placeholder="标签名" value={tagDraft} style={{ width: 100 }}
                onChange={(_, d) => setTagDraft(d.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') addTag(); if (e.key === 'Escape') setAdding(false) }}
                onBlur={addTag}
              />
            ) : (
              <button className={styles.chipAdd} onClick={() => setAdding(true)}>
                <AddRegular fontSize={12} />标签
              </button>
            )}
          </div>
        </div>

        <div className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.sectionTitle}>备注</span>
            {noteDraft !== session.note ? (
              <button className={styles.linkBtn} onClick={saveNote}>保存</button>
            ) : null}
          </div>
          <Textarea
            size="small"
            placeholder="为这个会话记点什么(只存本机标注库)…"
            value={noteDraft}
            onChange={(_, d) => setNoteDraft(d.value)}
            rows={3}
            resize="vertical"
          />
        </div>

        <div className={styles.section}>
          <div className={styles.sectionHead}>
            <span className={styles.sectionTitle}>摘要</span>
            <Tooltip content="v1.1 提供,当前版本未实现" relationship="description">
              <span className={styles.grayed}><button className={styles.linkBtn} disabled>生成新摘要</button></span>
            </Tooltip>
          </div>
          <div className={styles.summaryBox}>v1.1 将基于本地模型生成会话摘要;当前版本请用「导出为 Markdown」离线阅读。</div>
        </div>

        <div className={styles.section}>
          <span className={styles.sectionTitle}>操作</span>
          <div className={styles.actions}>
            <div className={styles.actionRow}>
              {session.resume ? (
                <>
                  <Button
                    className={styles.actionBtn} size="small" icon={<OpenRegular />}
                    onClick={() => void resumeService.OpenInTerminal(session.key)}
                  >在终端中打开</Button>
                  <Button
                    className={styles.actionBtn} size="small" appearance="primary" icon={<ArrowSyncRegular />}
                    onClick={() => void resumeService.OpenInTerminal(session.key)}
                  >恢复此会话</Button>
                </>
              ) : (
                <Text size={200} style={{ color: tokens.colorPaletteYellowForeground1 }}>
                  该会话不可恢复,仅可查看历史
                </Text>
              )}
            </div>
            <Button size="small" icon={<CopyRegular />} onClick={() => void resumeService.CopyToClipboard(session.key)}>
              复制恢复命令
            </Button>
            <Button size="small" icon={<ArrowDownloadRegular />} onClick={exportMd}>导出为 Markdown</Button>
            <Button size="small" icon={<DocumentTextRegular />} onClick={toggleArchive}>
              {session.archived ? '取消归档' : '归档(不删源文件)'}
            </Button>
            <Tooltip content="v1.1 提供,当前版本未实现" relationship="description">
              <span className={styles.grayed}>
                <Button size="small" disabled style={{ width: '100%' }}>交接到其他 Agent</Button>
              </span>
            </Tooltip>
            <Tooltip content="本工具不做物理删除,源会话文件只增不改;如需隐藏请用归档" relationship="description">
              <span className={styles.grayed}>
                <Button size="small" disabled icon={<DeleteRegular />} style={{ width: '100%' }}>删除会话(不提供)</Button>
              </span>
            </Tooltip>
          </div>
        </div>

        {msg ? <Text size={200} style={{ color: tokens.colorNeutralForeground3, overflowWrap: 'anywhere' }}>{msg}</Text> : null}
      </div>
    </div>
  )
}
