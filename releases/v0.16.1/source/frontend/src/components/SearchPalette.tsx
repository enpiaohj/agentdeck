// 工作区内的全文搜索结果，与会话预览并排展示。
// 索引未就绪时后端自动降级为逐会话扫描,Reply.mode 明示给用户。
import {
  makeStyles,
  mergeClasses,
  tokens,
  Text,
  Spinner,
} from '@fluentui/react-components'
import { SearchRegular } from '@fluentui/react-icons'
import { useEffect, useRef, useState } from 'react'
import type { SearchReply } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as searchService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/searchservice'
import { agentMeta, relTime } from '../util'
import { AgentMark } from '../icons/AgentMark'

const useStyles = makeStyles({
  panel: {
    width: 'var(--ad-list-width)',
    minHeight: 0,
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: tokens.colorNeutralBackground1,
    borderRight: `1px solid ${tokens.colorNeutralStroke3}`,
    overflow: 'hidden',
    padding: 0,
  },
  inputRow: { display: 'flex', alignItems: 'center', gap: '8px', padding: '12px 14px', borderBottom: `1px solid ${tokens.colorNeutralStroke2}` },
  banner: {
    padding: '6px 14px',
    fontSize: '11px',
    backgroundColor: tokens.colorStatusWarningBackground2,
    color: tokens.colorStatusWarningForeground1,
  },
  bannerOk: { backgroundColor: tokens.colorNeutralBackground2, color: tokens.colorNeutralForeground3 },
  results: { flex: 1, minHeight: 0, overflowY: 'auto', padding: '8px' },
  hit: {
    display: 'block',
    width: '100%',
    borderRadius: '6px',
    marginBottom: '2px',
    textAlign: 'left',
    border: 'none',
    backgroundColor: 'transparent',
    color: tokens.colorNeutralForeground1,
    padding: '8px 9px',
    cursor: 'pointer',

    ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover },
  },
  hitTitle: { display: 'flex', gap: '7px', alignItems: 'center', marginBottom: '5px', minWidth: 0 },
  active: { backgroundColor: tokens.colorBrandBackground2, boxShadow: `inset 2px 0 0 ${tokens.colorBrandBackground}` },
  hitMeta: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '6px', marginBottom: '5px' },
  agentTag: {
    fontSize: '11px',
    padding: '1px 6px',
    borderRadius: '4px',
    backgroundColor: tokens.colorNeutralBackground2,
    color: tokens.colorNeutralForeground2,
    flexShrink: 0,
  },
  snippet: { display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden', fontSize: '12px', lineHeight: '1.6', color: tokens.colorNeutralForeground2, overflowWrap: 'anywhere' },
  // 与会话内查找的正文高亮同色（index.css 的 ::highlight(ad-find-match)），两处看起来是同一种标记。
  mark: { backgroundColor: 'rgba(255, 200, 0, 0.38)', color: 'inherit', borderRadius: '2px', padding: '0 1px' },
  empty: { padding: '24px', textAlign: 'center', color: tokens.colorNeutralForeground3 },
})

type Props = {
  query: string
  revision: number
  selectedKey: string | null
  onFocusSearch: () => void
  onJump: (key: string, partIndex: number, match: string, offset: number) => void
}

export function SearchResults({ query: q, revision, selectedKey, onFocusSearch, onJump }: Props) {
  const styles = useStyles()
  const [reply, setReply] = useState<SearchReply | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const resultRefs = useRef<(HTMLButtonElement | null)[]>([])
  const requestVersion = useRef(0)

  // 新输入和离开搜索立即使旧请求失效，避免慢查询覆盖新结果。
  useEffect(() => {
    const version = ++requestVersion.current
    setReply(null); setError(''); setLoading(false)
    if (!q.trim()) return
    const timer = setTimeout(() => {
      setLoading(true)
      searchService.Query(q, 50)
        .then(result => { if (requestVersion.current === version) setReply(result) })
        .catch(reason => { if (requestVersion.current === version) setError(String(reason).slice(0, 200)) })
        .finally(() => { if (requestVersion.current === version) setLoading(false) })
    }, 300)
    return () => { clearTimeout(timer); requestVersion.current++ }
  }, [q, revision])

  const highlight = (hit: { match: string; snippet: string; snippet_start: number; snippet_end: number }) => {
    if (!hit.match) return <span className={styles.snippet}>{hit.snippet}</span>
    const i = hit.snippet_start
    if (i < 0) return <span className={styles.snippet}>{hit.snippet}</span>
    return (
      <span className={styles.snippet}>
        {hit.snippet.slice(0, i)}
        <span className={styles.mark}>{hit.snippet.slice(i, hit.snippet_end)}</span>
        {hit.snippet.slice(hit.snippet_end)}
      </span>
    )
  }

  return (
    <section className={styles.panel} aria-label="全文搜索结果">
        <div className={styles.inputRow}>
          <SearchRegular style={{ color: tokens.colorBrandForeground1 }} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <Text size={300} weight="semibold" block>搜索结果</Text>
            <Text size={200} block style={{ color: tokens.colorNeutralForeground3, marginTop: 4, overflowWrap: 'anywhere' }}>「{q}」</Text>
          </div>
          {loading ? <Spinner size="extra-tiny" /> : null}
        </div>
        {error ? <div className={styles.empty} role="alert">搜索失败：{error}</div> : reply ? (
          <>
            {reply.mode === 'scan' ? (
              <div className={styles.banner}>全文索引尚未就绪，当前使用逐会话扫描，仅显示部分结果。</div>
            ) : (
              <div className={mergeClasses(styles.banner, styles.bannerOk)}>全文检索 · {reply.total} 处命中 · 显示 {reply.hits.length} 条</div>
            )}
            <div className={styles.results}>
              {reply.hits.length === 0 ? (
                <div className={styles.empty}>未找到与「{q}」相关的会话内容</div>
              ) : (
                reply.hits.map((h, i) => (
                  <button title={h.title || '未命名会话'} key={h.part + i} ref={element => { resultRefs.current[i] = element }} className={mergeClasses(styles.hit, selectedKey === h.key && styles.active)} aria-current={selectedKey === h.key ? 'true' : undefined} onClick={() => onJump(h.key, partIndexOf(h.part), h.match, h.start)}
                    onKeyDown={event => {
                      if (event.key === 'ArrowDown') { event.preventDefault(); resultRefs.current[(i + 1) % reply.hits.length]?.focus() }
                      if (event.key === 'ArrowUp') { event.preventDefault(); if (i === 0) onFocusSearch(); else resultRefs.current[i - 1]?.focus() }
                    }}>
                    <span className={styles.hitTitle}>
                      <AgentMark agent={h.agent} size={18} />
                      <Text size={300} weight="semibold" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {h.title || '未命名会话'}
                      </Text>
                    </span>
                    <span className={styles.hitMeta}>
                      <span className={styles.agentTag}>{agentMeta(h.agent).label}</span>
                      <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>{relTime(h.last)}</Text>
                    </span>
                    {highlight(h)}
                  </button>
                ))
              )}
            </div>
          </>
        ) : (
          <div className={styles.empty}>{loading ? '正在搜索会话正文…' : q.trim() ? '正在准备搜索…' : '输入关键词以检索历史会话正文。'}</div>
        )}
        <div className={mergeClasses(styles.banner, styles.bannerOk)}>选择结果预览会话 · Esc 返回会话列表</div>
    </section>
  )
}

// doc key 形如 "agent:id#n",n 与 transcript 分段同序;-1 = 无定位。
function partIndexOf(docKey: string): number {
  const i = docKey.lastIndexOf('#')
  if (i < 0) return -1
  const n = Number(docKey.slice(i + 1))
  return Number.isFinite(n) ? n : -1
}
