import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'

type ToastVariant = 'success' | 'error'
type ToastFn = (message: string, variant?: ToastVariant) => void

interface ToastItem {
  id: number
  message: string
  variant: ToastVariant
}

const ToastContext = createContext<ToastFn | null>(null)

const AUTO_DISMISS_MS = 3200

// ToastProvider gives pages a lightweight way to confirm that an action
// happened (a delete went through, a run was cancelled) without a jarring
// native alert() or a paragraph of text that lingers in the page forever.
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const nextId = useRef(0)

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const showToast = useCallback<ToastFn>(
    (message, variant = 'success') => {
      const id = ++nextId.current
      setToasts((prev) => [...prev, { id, message, variant }])
      setTimeout(() => dismiss(id), AUTO_DISMISS_MS)
    },
    [dismiss],
  )

  return (
    <ToastContext.Provider value={showToast}>
      {children}
      <div className="toast-stack">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.variant}`} onClick={() => dismiss(t.id)}>
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastFn {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within a ToastProvider')
  return ctx
}
