import { Browser } from '@wailsio/runtime'
import * as settingsService from '../bindings/github.com/enpiaohj/agentdeck/internal/app/settingsservice'

/**
 * 在浏览器里打开网页链接。
 *
 * 用户常在某个特定浏览器里登录了 ChatGPT 等网站，而系统默认浏览器往往是别的，
 * 所以先问后端：设置里选了浏览器就用它打开；没有选择、所选浏览器找不到或启动失败
 * 时，后端返回 false，这里退回系统默认浏览器——链接总该打得开。
 * 链接必须是 http / https；其余由后端拒绝。
 */
export async function openExternal(url: string): Promise<void> {
  // 后端只会因为链接不合法而报错：那样系统默认浏览器也不该去开它，让错误原样抛出
  const opened = await settingsService.OpenLink(url)
  if (!opened) await Browser.OpenURL(url)
}
