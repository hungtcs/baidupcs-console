import { useEffect } from 'react'

interface ToastProps {
  text: string
  kind: 'info' | 'error'
  onClose: () => void
}

export function Toast({ text, kind, onClose }: ToastProps) {
  useEffect(() => {
    const timer = window.setTimeout(onClose, 3600)
    return () => window.clearTimeout(timer)
  }, [onClose, text])

  return (
    <div className={`toast toast-${kind}`} role="status">
      <span className="toast-text">{text}</span>
      <button type="button" className="toast-close" onClick={onClose} aria-label="关闭">
        ×
      </button>
    </div>
  )
}
