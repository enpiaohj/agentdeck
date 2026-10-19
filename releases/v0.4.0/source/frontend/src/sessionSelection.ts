export type SessionSelection = { keys: Set<string>; anchor: string | null; focus: string | null }
export type SelectionModifiers = { toggle?: boolean; range?: boolean }

// Context actions preserve an existing group; an unselected row becomes the only target.
export function selectContextSession(state: SessionSelection, orderedKeys: readonly string[], key: string): SessionSelection {
  const current = pruneSelection(state, orderedKeys)
  if (!orderedKeys.includes(key)) return current
  return current.keys.has(key) ? { ...current, focus: key } : selectSession(current, orderedKeys, key)
}

export function selectSession(state: SessionSelection, orderedKeys: readonly string[], key: string, modifiers: SelectionModifiers = {}): SessionSelection {
  const current = pruneSelection(state, orderedKeys)
  const target = orderedKeys.indexOf(key)
  if (target < 0) return current
  if (modifiers.range) {
    const anchor = current.anchor ?? current.focus ?? key
    const from = orderedKeys.indexOf(anchor)
    const range = orderedKeys.slice(Math.min(from, target), Math.max(from, target) + 1)
    return { keys: new Set(modifiers.toggle ? [...current.keys, ...range] : range), anchor, focus: key }
  }
  const keys = modifiers.toggle ? new Set(current.keys) : new Set<string>()
  if (modifiers.toggle && keys.has(key)) keys.delete(key)
  else keys.add(key)
  return { keys, anchor: key, focus: key }
}

export function pruneSelection(state: SessionSelection, orderedKeys: readonly string[]): SessionSelection {
  const visible = new Set(orderedKeys)
  const keys = new Set([...state.keys].filter(key => visible.has(key)))
  const anchor = state.anchor && visible.has(state.anchor) ? state.anchor : null
  const focus = state.focus && visible.has(state.focus) ? state.focus : keys.values().next().value ?? null
  return { keys, anchor, focus }
}
