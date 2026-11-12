// 标注（收藏 / 归档 / 标签 / 备注 / 改名）写完后，列表不需要整表重载：
// 只有这五项会变，其余字段（家族分组、项目名、消息数……）原样保留。
// 本模块只做判定与合并，不碰网络，便于单测。

export type MarkFields = {
  key: string
  favorite: boolean
  archived: boolean
  tags: string[]
  note: string
  title: string
}

export type MarkScope = { favorites?: boolean; archived?: boolean }

/** 一行在当前侧栏范围里是否可见：与后端 List 的归档 / 收藏口径一致。 */
export function inMarkScope(row: Pick<MarkFields, 'favorite' | 'archived'>, scope: MarkScope): boolean {
  if (!!row.archived !== !!scope.archived) return false
  if (scope.favorites && !row.favorite) return false
  return true
}

/**
 * 把一批刚读到的标注并进当前列表。
 * 返回新的行数组；返回 null 表示“无法只靠局部更新判断”（有行新进入了
 * 当前范围，而列表里还没有它），调用方应退回整表重载。
 */
export function applyMarkPatch<T extends MarkFields>(rows: readonly T[], fresh: readonly MarkFields[], scope: MarkScope): T[] | null {
  const byKey = new Map(fresh.map(item => [item.key, item]))
  const present = new Set<string>()
  const next: T[] = []
  for (const row of rows) {
    const update = byKey.get(row.key)
    if (!update) { next.push(row); continue }
    present.add(row.key)
    const merged = { ...row, favorite: update.favorite, archived: update.archived, tags: update.tags, note: update.note, title: update.title }
    if (inMarkScope(merged, scope)) next.push(merged)
  }
  for (const item of fresh) {
    if (!present.has(item.key) && inMarkScope(item, scope)) return null
  }
  return next
}

/** 合并同一时间窗内的多个 key；空串不是 key。 */
export function eventKey(data: unknown): string {
  const key = Array.isArray(data) ? data[0] : data
  return typeof key === 'string' ? key : ''
}
