// 中栏:会话列表(340px)。按日分组;卡片式行 = 标题 + Agent 徽章行(项目/标签/时间);
// 选中态蓝底 + 左侧指示条;hover 显示勾选框;多选后浮动批量条。
import {
  makeStyles,
  shorthands,
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
  FilterRegular,
  ArrowSortRegular,
} from '@fluentui/react-icons'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Events } from '@wailsio/runtime'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentMeta, relTime } from '../util'
import { AgentAvatar } from './AgentAvatar'
import type { SidebarFilter } from './Sidebar'

const useStyles = makeStyles({
  wrap: {
    width: '340px',
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
    minWidth: 0,
    minHeight: 0,
    backgroundColor: tokens.colorNeutralBackground1,
    borderRight: `1px solid ${tokens.colorNeutralStroke3}`,
  },
  head: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '12px 14px 8px',
  },
  headTitle: { fontSize: '16px', fontWeight: 600 },
  headBtn: {
    marginLeft: 'auto',
    border: 'none',
    background: 'transparent',
    cursor: 'pointer',
    color: tokens.colorNeutralForeground3,
    display: 'flex',
    alignItems: 'center',
    padding: '4px',
    ...shorthands.borderRadius('4px'),
    ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover },
  },
  rows: { flex: 1, overflowY: 'auto', overflowX: 'hidden', paddingBottom: '56px' },
  groupTitle: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '12px 16px 6px',
    fontSize: '12px',
    fontWeight: 600,
    color: tokens.colorNeutralForeground3,
  },
  groupCount: { color: tokens.colorNeutralForeground4, fontWeight: 400 },
  // 行 = 卡片(概念图):圆角、细边框、行间留白;选中为蓝色淡底 + 左侧色条。
  row: {
    position: 'relative',
    display: 'flex',
    gap: '8px',
    margin: '0 10px 6px',
    padding: '10px 12px 10px 9px',
    ...shorthands.border('1px', 'solid', tokens.colorNeutralStroke3),
    ...shorthands.borderRadius('8px'),
    borderLeftWidth: '3px',
    borderLeftColor: 'transparent',
    backgroundColor: tokens.colorNeutralBackground1,
    cursor: 'pointer',
    ':hover': {
      backgroundColor: tokens.colorNeutralBackground1Hover,
      ...shorthands.borderColor(tokens.colorNeutralStroke1),
    },
  },
  rowActive: {
    backgroundColor: tokens.colorBrandBackground2,
    borderLeftColor: tokens.colorBrandBackground,
    ':hover': { backgroundColor: tokens.colorBrandBackground2Hover },
  },
  rowBody: { display: 'flex', flexDirection: 'column', gap: '5px', flex: 1, minWidth: 0 },
  titleLine: { display: 'flex', alignItems: 'center', gap: '6px', minWidth: 0 },
  title: { flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: '14px', fontWeight: 600 },
  starBtn: {
    border: 'none', background: 'transparent', cursor: 'pointer',
    display: 'flex', alignItems: 'center', padding: '2px', flexShrink: 0,
    ...shorthands.borderRadius('4px'),
    color: tokens.colorNeutralForeground4,
    ':hover': { color: tokens.colorStatusWarningForeground3 },
  },
  starOn: { color: tokens.colorStatusWarningForeground3 },
  titleOn: { color: tokens.colorBrandForeground1 },
  meta: { display: 'flex', alignItems: 'center', gap: '6px', minWidth: 0 },
  agentName: { fontSize: '12px', color: tokens.colorNeutralForeground2, flexShrink: 0 },
  chip: {
    fontSize: '11.5px',
    padding: '1px 7px',
    ...shorthands.borderRadius('5px'),
    backgroundColor: tokens.colorNeutralBackground3,
    color: tokens.colorNeutralForeground2,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    maxWidth: '130px',
    flexShrink: 1,
  },
  time: { marginLeft: 'auto', fontSize: '11.5px', color: tokens.colorNeutralForeground3, flexShrink: 0 },
  noResume: { fontSize: '11.5px', color: tokens.colorPaletteYellowForeground1, flexShrink: 0 },
  center: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: '8px', padding: '0 16px', textAlign: 'center' },
  bulkBar: {
    position: 'absolute',
    bottom: '14px',
    left: '50%',
    transform: 'translateX(-50%)',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 14px',
    ...shorthands.borderRadius('10px'),
    backgroundColor: tokens.colorNeutralBackground3,
    boxShadow: tokens.shadow16,
    zIndex: 10,
  },
})

type Props = {
  filter: SidebarFilter
  selectedKey: string | null
  onSelect: (s: SessionView) => void
  listVersion?: number
}

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

  const listTitle = filter.archived ? '归档会话' : filter.favorites ? '收藏会话' : filter.project ? `项目 · ${filter.project}` : filter.agent ? `Agent · ${filter.agent}` : '会话列表'

  return (
    <div className={styles.wrap}>
      <div className={styles.head}>
        <span className={styles.headTitle}>{listTitle}</span>
        <button className={styles.headBtn} title="重新扫描(后端按 agent 过滤)" onClick={reload}>
          <FilterRegular fontSize={15} />
        </button>
        <button className={styles.headBtn} title="列表按最后活动时间倒序" onClick={reload}>
          <ArrowSortRegular fontSize={15} />
        </button>
      </div>

      {error ? (
        <div className={styles.center}>
          <Text size={300}>会话加载失败:{error}</Text>
        </div>
      ) : loading && rows.length === 0 ? (
        <div className={styles.center}>
          <Spinner size="tiny" />
          <Text size={300}>正在扫描本机会话…</Text>
        </div>
      ) : rows.length === 0 ? (
        <div className={styles.center}>
          <Text size={400} weight="semibold">
            {filter.archived ? '没有归档的会话' : filter.favorites ? '还没有收藏的会话' : '这里还没有会话'}
          </Text>
          <Text size={200} style={{ color: tokens.colorNeutralForeground3 }}>
            {filter.archived
              ? '在会话信息面板中选择「归档」,即可从这里找到并恢复'
              : '支持的 Agent 产生会话后,点顶部刷新即可看到'}
          </Text>
        </div>
      ) : (
        <div className={styles.rows}>
          {groups.map(([day, list]) => (
            <div key={day}>
              <div className={styles.groupTitle}>
                {day}<span className={styles.groupCount}>({list.length})</span>
              </div>
              {list.map((s) => {
                const active = selectedKey === s.key
                const m = agentMeta(s.agent)
                return (
                  <div
                    key={s.key}
                    className={`sessionRow ${styles.row} ${active ? styles.rowActive : ''}`}
                    onClick={() => onSelect(s)}
                  >
                    <span className="rowCheck" data-checked={checked.has(s.key) ? 'true' : 'false'} style={{ display: 'flex', alignItems: 'center' }}>
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
                    <div className={styles.rowBody}>
                      <div className={styles.titleLine}>
                        <Text className={styles.title + (active ? ' ' + styles.titleOn : '')}>{s.title || '(无标题)'}</Text>
                        <button
                          className={styles.starBtn + (s.favorite ? ' ' + styles.starOn : '')}
                          onClick={toggleStar(s)}
                          title={s.favorite ? '取消收藏' : '收藏'}
                        >
                          {s.favorite ? <StarFilled fontSize={13} /> : <StarRegular fontSize={13} />}
                        </button>
                      </div>
                      <div className={styles.meta}>
                        <AgentAvatar agent={s.agent} size={14} />
                        <span className={styles.agentName}>{m.label}</span>
                        {s.project ? <span className={styles.chip}>{s.project}</span> : null}
                        {s.tags.slice(0, 2).map((t) => <span key={t} className={styles.chip}>{t}</span>)}
                        {!s.resume ? <span className={styles.noResume}>仅可查看历史</span> : null}
                        {filter.archived ? (
                          <Button size="small" icon={<ArchiveArrowBackRegular />} onClick={unarchive(s)}>恢复</Button>
                        ) : null}
                        <span className={styles.time}>{relTime(s.last)}</span>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      )}

      {checked.size > 0 ? (
        <div className={styles.bulkBar}>
          <Text size={300} weight="semibold">已选 {checked.size} 项</Text>
          <Button size="small" icon={<StarRegular />} disabled={busy}
            onClick={bulk((k) => markService.SetFavorite(k, true))}>收藏</Button>
          <Button size="small" icon={<ArchiveRegular />} disabled={busy}
            onClick={bulk((k) => markService.SetArchived(k, true))}>归档</Button>
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

  useEffect(() => { load() }, [load, listVersion])

  useEffect(() => {
    const off = Events.On('sessions:messages', () => load())
    return () => { off() }
  }, [load])

  return { ...state, reload: load }
}
