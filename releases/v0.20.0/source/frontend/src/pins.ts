// 置顶：会话、智能体、项目各一份列表，存在界面偏好里（见 preferences.ts）。
// 只决定显示顺序，不改动任何会话数据；本模块只做纯计算，便于单测。

/** 偏好里每类置顶最多保留的条数：防止异常数据把偏好撑大。 */
export const pinLimit = 200

/** 项目路径大小写不敏感（Windows 路径），其余 key 原样比较。 */
export function pinKey(value: string, caseInsensitive = false): string {
  return caseInsensitive ? value.toLocaleLowerCase() : value
}

export function isPinned(pins: readonly string[], key: string, caseInsensitive = false): boolean {
  const wanted = pinKey(key, caseInsensitive)
  return pins.some(pin => pinKey(pin, caseInsensitive) === wanted)
}

/** 置顶或取消置顶一批 key：全部已置顶时取消，否则全部置顶（与「收藏」的批量语义一致）。 */
export function togglePins(pins: readonly string[], keys: readonly string[], caseInsensitive = false): string[] {
  if (!keys.length) return [...pins]
  const allPinned = keys.every(key => isPinned(pins, key, caseInsensitive))
  if (allPinned) {
    const drop = new Set(keys.map(key => pinKey(key, caseInsensitive)))
    return pins.filter(pin => !drop.has(pinKey(pin, caseInsensitive)))
  }
  const next = [...pins]
  for (const key of keys) if (!isPinned(next, key, caseInsensitive)) next.push(key)
  return next.slice(-pinLimit)
}

/** 去掉一批 key（会话被删除时调用）。 */
export function removePins(pins: readonly string[], keys: readonly string[]): string[] {
  const drop = new Set(keys)
  return pins.filter(pin => !drop.has(pin))
}

/** 置顶的排在前面，两部分各自保持原有顺序（稳定排序）。 */
export function pinnedFirst<T>(items: readonly T[], keyOf: (item: T) => string, pins: readonly string[], caseInsensitive = false): T[] {
  if (!pins.length) return [...items]
  const set = new Set(pins.map(pin => pinKey(pin, caseInsensitive)))
  const pinned: T[] = []
  const rest: T[] = []
  for (const item of items) (set.has(pinKey(keyOf(item), caseInsensitive)) ? pinned : rest).push(item)
  return [...pinned, ...rest]
}

/** 解析偏好里的置顶列表：只留非空字符串，去重，超过上限的截掉最早的。 */
export function parsePins(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const out: string[] = []
  for (const item of value) {
    if (typeof item !== 'string') continue
    const key = item.trim()
    if (key && !out.includes(key)) out.push(key)
  }
  return out.slice(-pinLimit)
}
