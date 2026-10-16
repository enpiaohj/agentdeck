// 中栏:会话列表。按 Sidebar 过滤条件加载,点击选中进详情。
import {
  makeStyles,
  tokens,
  Text,
  Spinner,
} from '@fluentui/react-components'
import { StarFilled } from '@fluentui/react-icons'
import { useEffect, useState } from 'react'
import type { SessionView } from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/models'
import * as sessionService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/sessionservice'
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
  title: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
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
}

export function SessionList({ filter, selectedKey, onSelect }: Props) {
  const styles = useStyles()
  const { rows, loading, error } = useRows(filter)

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
          <Text size={400} weight="semibold">这里还没有会话</Text>
          <Text size={300} style={{ color: tokens.colorNeutralForeground3 }}>
            支持的 Agent 产生会话后,重启应用即可看到
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
              {s.favorite ? <StarFilled style={{ color: tokens.colorStatusWarningForeground3, fontSize: 12 }} /> : null}
              <Text className={styles.title} size={300}>{s.title || '(无标题)'}</Text>
            </div>
            <div className={styles.meta}>
              <span>{relTime(s.last)}</span>
              <span>{s.messages} 条消息</span>
              <span>{s.project || '项目未知'}</span>
              {!s.resume ? <span style={{ color: tokens.colorPaletteYellowForeground1 }}>仅可查看历史</span> : null}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

type RowsState = { rows: SessionView[]; loading: boolean; error: string }

function useRows(filter: SidebarFilter): RowsState {
  const [state, setState] = useState<RowsState>({ rows: [], loading: true, error: '' })
  useEffect(() => {
    let live = true
    setState((s) => ({ ...s, loading: true }))
    sessionService
      .List({
        agent: filter.agent ?? '',
        favorites: filter.favorites ?? false,
        archived: filter.archived ?? false,
        project: '',
        tag: '',
        search: '',
      })
      .then((rows) => live && setState({ rows, loading: false, error: '' }))
      .catch((e) => live && setState({ rows: [], loading: false, error: String(e) }))
    return () => { live = false }
  }, [filter])
  return state
}
