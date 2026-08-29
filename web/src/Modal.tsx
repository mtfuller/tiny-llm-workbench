import { X } from 'lucide-react'
import { useEffect, useRef, type MouseEvent, type ReactNode } from 'react'

interface ModalProps {
  title: string
  onClose: () => void
  children: ReactNode
  size?: 'md' | 'lg' | 'xl'
}

function Modal({ title, onClose, children, size = 'md' }: ModalProps) {
  // Only a click that both *starts* and *ends* on the overlay (outside the
  // modal box) dismisses it. A drag that begins inside a textarea and
  // releases over the overlay — e.g. selecting text and overshooting — no
  // longer closes the modal and loses the user's edits.
  const downOnOverlay = useRef(false)

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  const handleOverlayMouseDown = (e: MouseEvent<HTMLDivElement>) => {
    downOnOverlay.current = e.target === e.currentTarget
  }

  const handleOverlayMouseUp = (e: MouseEvent<HTMLDivElement>) => {
    if (downOnOverlay.current && e.target === e.currentTarget) onClose()
    downOnOverlay.current = false
  }

  const sizeClass = size === 'lg' ? ' modal-lg' : size === 'xl' ? ' modal-xl' : ''

  return (
    <div className="modal-overlay" onMouseDown={handleOverlayMouseDown} onMouseUp={handleOverlayMouseUp}>
      <div className={`modal${sizeClass}`} role="dialog" aria-modal="true" aria-label={title}>
        <div className="modal-header">
          <h3>{title}</h3>
          <button type="button" className="icon-button" onClick={onClose} aria-label="Close">
            <X size={18} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  )
}

export default Modal
