import { Browser } from '@wailsio/runtime'
import { useEffect, useState } from 'react'
import * as updateService from '../../bindings/github.com/enpiaohj/agentdeck/internal/app/updateservice'
import { matchesSetting } from '../settingsSearch'

const repoURL = 'https://github.com/enpiaohj/agentdeck'
const authorURL = 'https://github.com/enpiaohj'

export const matchesAboutSettings = (query: string) => matchesSetting(query, '关于 版本 开发者 作者 源码仓库 GitHub 许可证 MIT 第三方 隐私 数据 遥测 网络 联网 AgentDeck')

function Link({ href, children }: { href: string; children: string }) {
  return <button className="ad-link" onClick={() => void Browser.OpenURL(href)}>{children}</button>
}

// 关于页：只放事实——产品、版本、作者、仓库、许可证，以及联网行为。
// 版本读自 exe 的 VERSIONINFO（经 update 服务的 Status，不发网络请求）。
export function AboutView() {
  const [version, setVersion] = useState('读取中…')
  useEffect(() => {
    let active = true
    updateService.Status().then(view => {
      if (active) setVersion(view.dev ? '开发版本' : view.current ? `v${view.current}` : '未知')
    }).catch(() => { if (active) setVersion('未知') })
    return () => { active = false }
  }, [])
  return <div className="ad-setting-card ad-about-card">
    <div className="ad-about-brand"><h3>AgentDeck</h3><span>跨 Agent 的历史会话检索与恢复工作台</span></div>
    <dl className="ad-info-list">
      <div><dt>版本</dt><dd>{version}</dd></div>
      <div><dt>开发者</dt><dd>enpiaohj<span className="ad-info-sub"><Link href={authorURL}>GitHub</Link></span></dd></div>
      <div><dt>源码仓库</dt><dd><Link href={repoURL}>{repoURL}</Link></dd></div>
      <div><dt>许可证</dt><dd>MIT</dd></div>
    </dl>
    <p className="ad-about-note">本应用按 MIT 许可证发布；随程序分发的第三方组件清单见仓库 THIRD-PARTY-NOTICES.txt 与 NOTICE.md。</p>
    <section className="ad-about-privacy" aria-label="隐私与数据">
      <h4>隐私与数据</h4>
      <p>AgentDeck 不收集遥测、崩溃报告或使用统计，不需要账号。会话正文、索引、标注与偏好全部保存在本机，不会上传。</p>
      <p>唯一的对外请求是「检查更新」：向 GitHub 发起一次只读请求以读取最新正式发布，只带产品名与版本号，不发送会话、路径或账号信息。可在「更新」设置中关闭启动后的自动检查；下载只在你点击「下载并更新」时发生。</p>
    </section>
  </div>
}
