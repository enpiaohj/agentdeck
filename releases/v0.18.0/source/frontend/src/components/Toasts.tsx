import { CheckmarkCircleRegular, DismissRegular, ErrorCircleRegular, InfoRegular } from '@fluentui/react-icons'
import type { Toast } from '../hooks/useToasts'

// 右下角的操作提示：一条一行，最多三条，可关闭；带动作的（如「撤销」）
// 会多停一会儿。
export function ToastHost({ toasts, onDismiss }: { toasts: Toast[]; onDismiss: (id: number) => void }) {
  if (!toasts.length) return null
  return <div className="ad-toasts" role="region" aria-label="操作提示">
    {toasts.map(t => (
      <div key={t.id} className="ad-toast" data-tone={t.tone} role={t.tone === 'error' ? 'alert' : 'status'}>
        <span className="ad-toast-icon" aria-hidden="true">
          {t.tone === 'success' ? <CheckmarkCircleRegular /> : t.tone === 'error' ? <ErrorCircleRegular /> : <InfoRegular />}
        </span>
        <span className="ad-toast-text">{t.text}</span>
        {t.action ? <button className="ad-toast-action" onClick={() => { onDismiss(t.id); t.action?.run() }}>{t.action.label}</button> : null}
        <button className="ad-toast-close" aria-label="关闭提示" onClick={() => onDismiss(t.id)}><DismissRegular fontSize={12} /></button>
      </div>
    ))}
  </div>
}