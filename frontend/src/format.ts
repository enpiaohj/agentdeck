// 数值展示辅助：数据大小与进度百分比。
//
// 单独一个模块而不是放进 util.ts：那个模块依赖 Agent 图标注册表，
// 纯数值函数留在这里才能被 Node 直接单测。

// fmtBytes 把字节数写成便于阅读的形式；0 与负值显示为 0 B。
export function fmtBytes(bytes: number): string {
  if (!bytes || bytes < 0) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  let value = bytes
  for (const suffix of ['KB', 'MB', 'GB', 'TB']) {
    value /= 1024
    if (value < 1024) return `${value.toFixed(value < 10 ? 1 : 0)} ${suffix}`
  }
  return `${value.toFixed(1)} PB`
}

// fmtPercent 把已完成量与总量写成整数百分比并封顶 100；总量未知时给空串
// （调用方据此不显示数字，而不是显示一个凭空的 0%）。
export function fmtPercent(done: number, total: number): string {
  if (!total || total <= 0) return ''
  return `${Math.min(100, Math.round((done / total) * 100))}%`
}