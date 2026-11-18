// 收藏 / 归档的操作反馈：成功后的一句话提示，以及撤销要执行的反向操作。
// 单独成模块只为了可以单测，不依赖 React。

export type MarkAction = 'favorite' | 'unfavorite' | 'archive' | 'unarchive'

const VERB: Record<MarkAction, string> = {
  favorite: '收藏',
  unfavorite: '取消收藏',
  archive: '归档',
  unarchive: '取消归档',
}

const INVERSE: Record<MarkAction, MarkAction> = {
  favorite: 'unfavorite',
  unfavorite: 'favorite',
  archive: 'unarchive',
  unarchive: 'archive',
}

export function inverseAction(action: MarkAction): MarkAction { return INVERSE[action] }

/** 「已归档 3 个会话」「已收藏 1 个会话」。 */
export function markDoneMessage(action: MarkAction, count: number): string {
  return `已${VERB[action]} ${count} 个会话`
}

/** 部分成功时的提示：成功的照常报，失败的数量单独说。 */
export function markPartialMessage(action: MarkAction, done: number, failed: number): string {
  return `${markDoneMessage(action, done)}，${failed} 个会话操作失败`
}

/** 撤销结果：全部成功返回空串（不需要再提示）。 */
export function undoFailureMessage(failed: number): string {
  return failed > 0 ? `撤销失败：${failed} 个会话未能恢复，请手动处理。` : ''
}
