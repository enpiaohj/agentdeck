// 侧栏智能体的 Shift 连选：从锚点连到点中的那个，按侧栏显示顺序。
// 锚点不在当前列表里时（智能体刷新后消失）退回已选的第一个，再退回点中的这个。
// add 为 true（Ctrl+Shift）时把这一段并入已选，否则这一段就是新的选择。
export function agentRange(order: readonly string[], anchor: string | null, current: readonly string[], agent: string, add: boolean): string[] {
  const start = anchor && order.includes(anchor) ? anchor : current.find(a => order.includes(a)) ?? agent
  const from = order.indexOf(start)
  const to = order.indexOf(agent)
  const range = from < 0 || to < 0 ? [agent] : order.slice(Math.min(from, to), Math.max(from, to) + 1)
  return add ? [...current, ...range.filter(a => !current.includes(a))] : range
}
