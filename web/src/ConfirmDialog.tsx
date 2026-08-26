import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'
import Modal from './Modal'

interface ConfirmOptions {
  title?: string
  confirmLabel?: string
  /** Whether the confirm button reads as destructive. Defaults to true. */
  danger?: boolean
}

type ConfirmFn = (message: string, options?: ConfirmOptions) => Promise<boolean>

const ConfirmContext = createContext<ConfirmFn | null>(null)

interface PendingConfirm extends ConfirmOptions {
  message: string
  resolve: (value: boolean) => void
}

// ConfirmProvider replaces window.confirm with an in-app dialog styled like
// the rest of the app, so destructive actions don't kick the user out to a
// native browser popup mid-flow.
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<PendingConfirm | null>(null)

  const confirm = useCallback<ConfirmFn>((message, options) => {
    return new Promise((resolve) => setPending({ message, resolve, ...options }))
  }, [])

  const settle = (value: boolean) => {
    pending?.resolve(value)
    setPending(null)
  }

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {pending && (
        <Modal title={pending.title ?? 'Are you sure?'} onClose={() => settle(false)}>
          <p>{pending.message}</p>
          <div className="row-actions confirm-actions">
            <button type="button" onClick={() => settle(false)}>
              Cancel
            </button>
            <button
              type="button"
              className={pending.danger === false ? 'button-primary' : 'danger-button'}
              onClick={() => settle(true)}
              autoFocus
            >
              {pending.confirmLabel ?? 'Confirm'}
            </button>
          </div>
        </Modal>
      )}
    </ConfirmContext.Provider>
  )
}

export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirm must be used within a ConfirmProvider')
  return ctx
}
