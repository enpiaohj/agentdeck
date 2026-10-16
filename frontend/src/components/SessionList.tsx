// 中栏:会话列表。按 Sidebar 过滤条件加载,点击选中进详情。
// 行内操作:收藏星切换;归档视图内「恢复」;消息数补齐后自动重拉。
import {
  makeStyles,
  tokens,
  Text,
  Spinner,
  Button,
} from '@fluentui/react-components'
import { StarFilled, StarRegular, ArchiveArrowBackRegular } from '@fluentui/react-icons'
import { useCallback, useEffect, useState } from 'react'
import { Events } from '@wailsio/runtime'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
import * as markService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/markservice'
import { agentLabel, relTime } from '../util'
import type { SidebarFilter } from './Sidebar'

const useStyles = makeStyles({
  wrap: { height: '100%', display: 'flex', flexDirection: 'column' },
  rows: { flex: 1, overflowY: 'auto', overflowX: 'hidden' },
  row: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    padding: '10px 14px',
    borderBottom: `1px solid ${tokens.colorNeutralStroke3}`,
    cursor: 'pointer',
    ':hover': { backgroundColor: tokens.colorNeutralBackground1Hover },
  },
  rowActive: { backgroundColor: tokens.colorNeutralBackground2 },
  titleLine: { display: 'flex', alignItems: 'center', gap: '6px', minWidth: 0 },
  title: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 },
  starBtn: {
    border: 'none', background: 'transparent', cursor: 'pointer',
    display: 'flex', alignItems: 'center', padding: '2px', color: tokens.colorNeutralForeground3,
    flexShrink: 0,
  },
  starOn: { color: tokens.colorStatusWarningForeground3 },
  meta: { display: 'flex', gap: '10px', fontSize: '12px', color: tokens.colorNeutralForeground3 },
  agentTag: {
    fontSize: '11px',
    padding: '1px 6px',
    borderRadius: '4px',
    backgroundColor: tokens.colorBrandBackground2,
    color: tokens.colorBrandForeground2,
    flexShrink: 0,
  },
  center: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', flexDirection: 'column', gap: '8px' },
})

type Props = {
  filter: SidebarFilter
  selectedKey: string | null
  onSelect: (s: SessionView) => void
  listVersion?: number // 详情页标注变化后 +1,触发重拉
}

export function SessionList({ filter, selectedKey, onSelect, listVersion }: Props) {
  const styles = useStyles()
  const { rows, loading, error, reload } = useRows(filter, listVersion)

  const toggleStar = (s: SessionView) => (e: React.MouseEvent) => {
    e.stopPropagation()
    markService.SetFavorite(s.key, !s.favorite).then(reload).catch(() => {})
  }

  const unarchive = (s: SessionView) => (e: React.MouseEvent) => {
    e.stopPropagation()
    markService.SetArchived(s.key, false).then(reload).catch(() => {})
  }

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
          <Spinner size="small" />
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
            {filter.archived ? '没有归档的会话' : '这里还没有会话'}
          </Text>
          <Text size={300} style={{ color: tokens.colorNeutralForeground3 }}>
            {filter.archived
              ? '在会话详情中选择「归档」,即可从这里找到并恢复'
              : '支持的 Agent 产生会话后,重启应用即可看到'}
          </Text>
        </div>
      </div>
    )
  }
  return (
    <div className={styles.wrap}>
      <div className={styles.rows}>
        {rows.map((s) => (
          <div
            key={s.key}
            className={`${styles.row} ${selectedKey === s.key ? styles.rowActive : ''}`}
            onClick={() => onSelect(s)}
          >
            <div className={styles.titleLine}>
              <span className={styles.agentTag}>{agentLabel(s.agent)}</span>
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
              <span>{s.messages} 条消息</span>
              <span>{s.project || '项目未知'}</span>
              {!s.resume ? <span style={{ color: tokens.colorPaletteYellowForeground1 }}>仅可查看历史</span> : null}
              {filter.archived ? (
                <Button size="small" icon={<ArchiveArrowBackRegular />} onClick={unarchive(s)}>恢复</Button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
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
