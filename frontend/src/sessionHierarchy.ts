type HierarchyRow = { key: string; role?: string; group_key?: string }

// A filtered-out parent never hides its children. Grouping affects display
// only; the original rows, keys and transcripts stay intact.
export function groupSessionRows<T extends HierarchyRow>(rows: readonly T[], expanded: ReadonlySet<string>) {
  const byKey = new Map(rows.map(row => [row.key, row]))
  const children = new Map<string, T[]>()
  const childKeys = new Set<string>()
  for (const row of rows) {
    const parent = row.group_key ? byKey.get(row.group_key) : undefined
    if (row.role !== 'subagent' || !parent || parent.role !== 'main' || parent.key === row.key) continue
    if (!children.has(parent.key)) children.set(parent.key, [])
    children.get(parent.key)!.push(row)
    childKeys.add(row.key)
  }
  const roots = rows.filter(row => !childKeys.has(row.key))
  const visible = roots.flatMap(row => expanded.has(row.key) ? [row, ...(children.get(row.key) ?? [])] : [row])
  return { roots, visible, children, childKeys }
}
