// 标题、说明和分类共同参与搜索；空格分隔的关键词需全部匹配。
export const settingSearchKeywords: Record<string, string> = {
  theme: '跟随系统 浅色 深色 颜色 外观',
  fontSize: '字体 13 14 15 16 18 px',
  width: '适中 宽版 自适应 820 1120',
  navCollapsed: '侧栏 展开 收起 导航',
  navWidth: '侧栏 导航 宽度 拖拽 分隔条',
  listWidth: '会话列表 宽度 拖拽 分隔条',
  infoWidth: '会话详情 详情栏 信息栏 宽度 拖拽 分隔条',
  infoPanel: '详情栏 信息栏 展开 收起',
  listOrder: '会话列表 排序 最近活动 最早活动',
  hideEmptyProjects: '隐藏没有会话的项目 空项目',
  start: '最新一轮问答 会话开头 默认阅读位置',
  spacing: '舒适 紧凑 正文行距',
  deleteBackup: '删除会话 备份 恢复备份 撤销 直接删除 不保留备份',
  terminalShell: 'Windows PowerShell 5.1 PowerShell 7+ 自动 自定义 可执行文件 pwsh.exe powershell.exe',
}

export function matchesSetting(query: string, ...content: string[]): boolean {
  const normalize = (value: string) => value.normalize('NFKC').toLocaleLowerCase()
  const text = normalize(content.join(' '))
  return normalize(query).trim().split(/\s+/).every(term => text.includes(term))
}
