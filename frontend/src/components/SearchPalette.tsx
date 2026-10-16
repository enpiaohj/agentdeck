// Ctrl+K 全局搜索面板:输入 → Query → 结果 → 跳转会话。
// 索引未就绪时后端自动降级为逐会话扫描,Reply.mode 明示给用户。
import {
  makeStyles,
  tokens,
  Text,
  Input,
  Spinner,
} from '@fluentui/react-components'
import { SearchRegular, DismissRegular } from '@fluentui/react-icons'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { SearchReply } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as searchService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import { agentLabel, relTime } from '../util'

const useStyles = makeStyles({
  overlay: {
    position: 'fixed',
    inset: 0,
    backgroundColor: 'rgba(0,0,0,.4)',
    display: 'flex',
    justifyContent: 'center',
    paddingTop: '10vh',
    zIndex: 1000,
  },
  panel: {
    width: '640px',
    maxHeight: '70vh',
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: tokens.colorNeutralBackground1,
    borderRadius: '8px',
    boxShadow: tokens.shadow16,
    overflow: 'hidden',
  },
  inputRow: { display: 'flex', alignItems: 'center', gap: '8px', padding: '12px 14px', borderBottom: `1px solid ${tokens.colorNeutralStroke2}` },
  banner: {
    padding: '6px 14px',
    fontSize: '12px',
    backgroundColor: tokens.colorStatusWarningBackground2,
    color: tokens.colorStatusWarningForeground1,
  },
  bannerOk: { backgroundColor: tokens.colorNeutralBackground2, color: tokens.colorNeutralForeground3 },
  results: { flex: 1, overflowY: 'auto' },
  hit: {
    padding: '10px 14px',
    cursor: 'pointer',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover },
  },
  hitTitle: { display: 'flex', gap: '6px', alignItems: 'center', marginBottom: '4px' },
  agentTag: {
    fontSize: '11px',
    padding: '1px 6px',
    borderRadius: '4px',
    backgroundColor: tokens.colorBrandBackground2,
    color: tokens.colorBrandForeground2,
    flexShrink: 0,
  },
  snippet: { fontSize: '12px', color: tokens.colorNeutralForeground2, overflowWrap: 'anywhere' },
  mark: { color: tokens.colorBrandForeground1, fontWeight: 600 },
  empty: { padding: '24px', textAlign: 'center', color: tokens.colorNeutralForeground3 },
})

type Props = {
  open: boolean
  onClose: () => void
  onJump: (key: string) => void
}

export function SearchPalette({ open, onClose, onJump }: Props) {
  const styles = useStyles()
  const [q, setQ] = useState('')
  const [reply, setReply] = useState<SearchReply | null>(null)
  const [loading, setLoading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const run = useCallback((query: string) => {
    if (!query.trim()) { setReply(null); return }
    setLoading(true)
    searchService
      .Query(query, 20)
      .then(setReply)
      .catch(() => setReply(null))
      .finally(() => setLoading(false))
  }, [])

  // 输入防抖 300ms
  useEffect(() => {
    const t = setTimeout(() => run(q), 300)
    return () => clearTimeout(t)
  }, [q, run])

  useEffect(() => {
    if (open) {
      setQ('')
      setReply(null)
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  if (!open) return null

  const highlight = (hit: { match: string; snippet: string }) => {
    if (!hit.match) return <span className={styles.snippet}>{hit.snippet}</span>
    const i = hit.snippet.indexOf(hit.match)
    if (i < 0) return <span className={styles.snippet}>{hit.snippet}</span>
    return (
      <span className={styles.snippet}>
        {hit.snippet.slice(0, i)}
        <span className={styles.mark}>{hit.match}</span>
        {hit.snippet.slice(i + hit.match.length)}
      </span>
    )
  }

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.panel} onClick={(e) => e.stopPropagation()}>
        <div className={styles.inputRow}>
          <SearchRegular />
          <Input
            ref={inputRef}
            appearance="underline"
            placeholder="搜索会话内容(2 字以上中文词、英文短语)…"
            value={q}
            onChange={(_, d) => setQ(d.value)}
            onKeyDown={(e) => { if (e.key === 'Escape') onClose() }}
            style={{ flex: 1 }}
          />
          {loading ? <Spinner size="extra-tiny" /> : <button onClick={onClose} style={{ border: 'none', background: 'transparent', cursor: 'pointer', display: 'flex' }} title="关闭"><DismissRegular /></button>}
        </div>
        {reply ? (
          <>
            {reply.mode === 'scan' ? (
              <div className={styles.banner}>全文索引尚未就绪,当前为快速搜索(逐会话扫描,仅返回部分结果)</div>
            ) : (
              <div className={styles.banner + ' ' + styles.bannerOk}>全文检索 · 共 {reply.total} 处命中,显示前 {reply.hits.length} 条</div>
            )}
            <div className={styles.results}>
              {reply.hits.length === 0 ? (
                <div className={styles.empty}>没有找到与「{q}」相关的会话内容</div>
              ) : (
                reply.hits.map((h, i) => (
                  <div key={h.part + i} className={styles.hit} onClick={() => onJump(h.key)}>
                    <div className={styles.hitTitle}>
                      <span className={styles.agentTag}>{agentLabel(h.agent)}</span>
                      <Text size={300} weight="semibold" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {h.title || '(无标题)'}
                      </Text>
                      <Text size={200} style={{ marginLeft: 'auto', color: tokens.colorNeutralForeground3, flexShrink: 0 }}>
                        {relTime(h.last)}
                      </Text>
                    </div>
                    {highlight(h)}
                  </div>
                ))
              )}
            </div>
          </>
        ) : (
          <div className={styles.empty}>输入关键词检索全部会话内容,Ctrl+K 随时唤起</div>
        )}
      </div>
    </div>
  )
}
