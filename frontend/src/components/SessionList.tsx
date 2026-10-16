// 中栏:会话列表。按 Sidebar 过滤条件加载;按日分组(今天/昨天/本周/更早);
// 行内收藏切换;多选后浮动操作条(批量收藏/归档,均为可逆操作)。
import {
  makeStyles,
  tokens,
  Text,
  Spinner,
  Button,
  Checkbox,
} from '@fluentui/react-components'
import {
  StarFilled,
  StarRegular,
  ArchiveRegular,
  ArchiveArrowBackRegular,
} from '@fluentui/react-icons'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Events } from '@wailsio/runtime'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { fmtTime, relTime } from '../util'
import { AgentAvatar } from './AgentAvatar'
import type { SidebarFilter } from './Sidebar'

const useStyles = makeStyles({
  wrap: { height: '100%', display: 'flex', flexDirection: 'column', position: 'relative' },
  rows: { flex: 1, overflowY: 'auto', overflowX: 'hidden', paddingBottom: '56px' },
  groupTitle: {
    padding: '14px 16px 6px',
    fontSize: '12px',
    fontWeight: 600,
    color: tokens.colorNeutralForeground3,
    position: 'sticky',
    top: 0,
    backgroundColor: tokens.colorNeutralBackground1,
    zIndex: 1,
  },
  row: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '10px',
    margin: '0 8px',
    padding: '10px 8px',
    borderRadius: '8px',
    cursor: 'pointer',
    ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover },
  },
  rowActive: { backgroundColor: tokens.colorNeutralBackground2 },
  rowBody: { display: 'flex', flexDirection: 'column', gap: '4px', flex: 1, minWidth: 0 },
  titleLine: { display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 },
  title: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1, fontWeight: 500 },
  starBtn: {
    border: 'none', background: 'transparent', cursor: 'pointer',
    display: 'flex', alignItems: 'center', padding: '2px', borderRadius: '4px',
    color: tokens.colorNeutralForeground4, flexShrink: 0,
    ':hover': { color: tokens.colorStatusWarningForeground3 },
  },
  starOn: { color: tokens.colorStatusWarningForeground3 },
  meta: { display: 'flex', alignItems: 'center', gap: '10px', fontSize: '12px', color: tokens.colorNeutralForeground3, flexWrap: 'wrap' },
  time: { marginLeft: 'auto', flexShrink: 0, fontVariantNumeric: 'tabular-nums' },
  tagChip: {
    fontSize: '11px',
    padding: '0 6px',
    borderRadius: '4px',
    backgroundColor: tokens.colorNeutralBackground4,
    color: tokens.colorNeutralForeground2,
  },
  sourceTag: {
    fontSize: '11px',
    padding: '0 6px',
    borderRadius: '4px',
    backgroundColor: tokens.colorPalettePurpleBackground2,
    color: tokens.colorPalettePurpleForeground2,
    flexShrink: 0,
  },
  noResume: { color: tokens.colorPaletteYellowForeground1 },
  center: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: '8px' },
  bulkBar: {
    position: 'absolute',
    bottom: '14px',
    left: '50%',
    transform: 'translateX(-50%)',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 14px',
    borderRadius: '10px',
    backgroundColor: tokens.colorNeutralBackground3,
    boxShadow: tokens.shadow16,
    zIndex: 10,
  },
})

type Props = {
  filter: SidebarFilter
  selectedKey: string | null
  onSelect: (s: SessionView) => void
  listVersion?: number // 详情页标注变化后 +1,触发重拉
}

// 按日分组的标题(概念图:今天/昨天/…)。
function dayGroup(rfc3339: string): string {
  if (!rfc3339) return '时间未知'
  const t = new Date(rfc3339)
  if (isNaN(t.getTime())) return '时间未知'
  const now = new Date()
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const days = Math.floor((midnight - new Date(t.getFullYear(), t.getMonth(), t.getDate()).getTime()) / 86400000)
  if (days <= 0) return '今天'
  if (days === 1) return '昨天'
  if (days < 7) return '本周'
  if (days < 30) return '本月'
  return '更早'
}
const GROUP_ORDER = ['今天', '昨天', '本周', '本月', '更早', '时间未知']

export function SessionList({ filter, selectedKey, onSelect, listVersion }: Props) {
  const styles = useStyles()
  const { rows, loading, error, reload } = useRows(filter, listVersion)
  const [checked, setChecked] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)

  const toggleStar = (s: SessionView) => (e: React.MouseEvent) => {
    e.stopPropagation()
    markService.SetFavorite(s.key, !s.favorite).then(reload).catch(() => {})
  }

  const unarchive = (s: SessionView) => (e: React.MouseEvent) => {
    e.stopPropagation()
    markService.SetArchived(s.key, false).then(reload).catch(() => {})
  }

  // 批量操作:逐个调用后端(均为可逆动作,完成后清选并重拉)。
  const bulk = (op: (key: string) => Promise<void>) => async () => {
    if (checked.size === 0 || busy) return
    setBusy(true)
    try {
      for (const key of checked) await op(key)
    } finally {
      setBusy(false)
      setChecked(new Set())
      reload()
    }
  }

  const groups = useMemo(() => {
    const g = new Map<string, SessionView[]>()
    for (const s of rows) {
      const d = dayGroup(s.last)
      if (!g.has(d)) g.set(d, [])
      g.get(d)!.push(s)
    }
    return GROUP_ORDER.filter((k) => g.has(k)).map((k) => [k, g.get(k)!] as const)
  }, [rows])

  if (error) {
    return (
      <div className={styles.wrap}>
        <div className={styles.center}>
          <Text>会话加载失败:{error}</Text>
        </div>
      </div>
    )
  }
  if (loading && rows.length === 0) {
    return (
      <div className={styles.wrap}>
        <div className={styles.center}>
          <Spinner size="tiny" />
          <Text size={300}>正在扫描本机会话…</Text>
        </div>
      </div>
    )
  }
  if (rows.length === 0) {
    return (
      <div className={styles.wrap}>
        <div className={styles.center}>
          <Text size={400} weight="semibold">
            {filter.archived ? '没有归档的会话' : filter.favorites ? '还没有收藏的会话' : '这里还没有会话'}
          </Text>
          <Text size={300} style={{ color: tokens.colorNeutralForeground3 }}>
            {filter.archived
              ? '在会话详情中选择「归档」,即可从这里找到并恢复'
              : '支持的 Agent 产生会话后,点顶部刷新即可看到'}
          </Text>
        </div>
      </div>
    )
  }
  return (
    <div className={styles.wrap}>
      <div className={styles.rows}>
        {groups.map(([day, list]) => (
          <div key={day}>
            <div className={styles.groupTitle}>{day} · {list.length}</div>
            {list.map((s) => (
              <div
                key={s.key}
                className={`sessionRow ${styles.row} ${selectedKey === s.key ? styles.rowActive : ''}`}
                onClick={() => onSelect(s)}
                title={fmtTime(s.last)}
              >
                <span className="rowCheck" data-checked={checked.has(s.key) ? 'true' : 'false'}>
                  <Checkbox
                    checked={checked.has(s.key)}
                    onChange={(_e, d) => {
                      const k = s.key
                      setChecked((prev) => {
                        const n = new Set(prev)
                        if (d.checked) n.add(k)
                        else n.delete(k)
                        return n
                      })
                    }}
                    onClick={(e) => e.stopPropagation()}
                    aria-label={`选择会话 ${s.title}`}
                  />
                </span>
                <AgentAvatar agent={s.agent} size={20} />
                <div className={styles.rowBody}>
                  <div className={styles.titleLine}>
                    <Text className={styles.title} size={300}>{s.title || '(无标题)'}</Text>
                    <button
                      className={`${styles.starBtn} ${s.favorite ? styles.starOn : ''}`}
                      onClick={toggleStar(s)}
                      title={s.favorite ? '取消收藏' : '收藏'}
                    >
                      {s.favorite ? <StarFilled fontSize={14} /> : <StarRegular fontSize={14} />}
                    </button>
                  </div>
                  <div className={styles.meta}>
                    <span>{relTime(s.last)}</span>
                    <span>·</span>
                    <span>{s.messages} 条消息</span>
                    <span>·</span>
                    <span>{s.project || '项目未知'}</span>
                    {s.source === 'wsl' ? <span className={styles.sourceTag}>WSL</span> : null}
                    {s.source === 'gateway' ? <span className={styles.sourceTag}>网关</span> : null}
                    {s.tags.map((t) => <span key={t} className={styles.tagChip}>{t}</span>)}
                    {!s.resume ? <span className={styles.noResume}>仅可查看历史</span> : null}
                    {filter.archived ? (
                      <Button size="small" icon={<ArchiveArrowBackRegular />} onClick={unarchive(s)}>恢复</Button>
                    ) : null}
                    <span className={styles.time}>{dayGroup(s.last) === '今天' ? new Date(s.last).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : ''}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        ))}
      </div>
      {checked.size > 0 ? (
        <div className={styles.bulkBar}>
          <Text size={300} weight="semibold">已选 {checked.size} 项</Text>
          <Button
            size="small"
            icon={<StarRegular />}
            disabled={busy}
            onClick={bulk((k) => markService.SetFavorite(k, true))}
          >收藏</Button>
          <Button
            size="small"
            icon={<ArchiveRegular />}
            disabled={busy}
            onClick={bulk((k) => markService.SetArchived(k, true))}
          >归档</Button>
          <Button size="small" disabled={busy} onClick={() => setChecked(new Set())}>取消</Button>
        </div>
      ) : null}
    </div>
  )
}

type RowsState = { rows: SessionView[]; loading: boolean; error: string; reload: () => void }

function useRows(filter: SidebarFilter, listVersion?: number): RowsState {
  const [state, setState] = useState({ rows: [] as SessionView[], loading: true, error: '' })

  const load = useCallback(() => {
    setState((s) => ({ ...s, loading: true }))
    sessionService
      .List({
        agent: filter.agent ?? '',
        favorites: filter.favorites ?? false,
        archived: filter.archived ?? false,
        project: filter.project ?? '',
        tag: '',
        search: '',
      })
      .then((rows) => setState({ rows, loading: false, error: '' }))
      .catch((e) => setState({ rows: [], loading: false, error: String(e) }))
  }, [filter])

  useEffect(() => {
    load()
  }, [load, listVersion])

  useEffect(() => {
    // 消息数补齐完成后重拉一次,列表上的「N 条消息」才会从 0 变真值
    const off = Events.On('sessions:messages', () => load())
    return () => { off() }
  }, [load])

  return { ...state, reload: load }
}
