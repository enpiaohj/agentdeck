// 原生文件对话框的「取消」归一化。
//
// Wails 的文档契约是「取消返回空串/空列表」，但 Windows 实现（cfd 的
// ErrorCancelled）走的是 reject，经 wails 包成
//   Invalid dialog call: Dialog.OpenFile failed, error getting selection: cancelled by user
// 于是调用方按契约写好的 `if (!dir) return` 守卫永远不命中，反而落到 catch 里，
// 把用户主动取消报成「备份失败 / 导出失败 / 操作失败」。
//
// 取消不是失败：这里把两种形态归一成「空串 = 没有选择」，真正的失败仍然抛出，
// 不被吞掉。
import { Dialogs } from '@wailsio/runtime'

type OpenFileOptions = Parameters<typeof Dialogs.OpenFile>[0]
type Opener = (options: OpenFileOptions) => Promise<string | string[]>

// 取消的措辞按平台不同：Windows 是 cfd 的 "cancelled by user"（英式拼写），
// 别的平台还可能是 "canceled"。按 cancel/ed 判，够用且不误伤（这条错误只会
// 来自对话框本身）。
export function isDialogCancelled(reason: unknown): boolean {
  return /cancel(l)?ed/i.test(String(reason))
}

// pickOpenPath 让用户选一个路径：取消返回空串（与 Wails 文档契约一致），
// 真失败照旧抛出。open 只为测试注入，调用方不传。
export async function pickOpenPath(options: OpenFileOptions, open: Opener = (o) => Dialogs.OpenFile(o)): Promise<string> {
  try {
    const picked = await open(options)
    return typeof picked === 'string' ? picked : ''
  } catch (reason) {
    if (isDialogCancelled(reason)) return ''
    throw reason
  }
}