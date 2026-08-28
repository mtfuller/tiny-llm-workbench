// ModalActions is the Cancel + submit button row every form modal ends
// with. Render it as the last child of a <form onSubmit={…}> — the submit
// button is type="submit", so the form's onSubmit fires.
interface ModalActionsProps {
  onCancel: () => void
  submitLabel?: string
  busyLabel?: string
  busy?: boolean
  disabled?: boolean
  cancelLabel?: string
}

export default function ModalActions({
  onCancel,
  submitLabel = 'Save',
  busyLabel = 'Saving…',
  busy = false,
  disabled = false,
  cancelLabel = 'Cancel',
}: ModalActionsProps) {
  return (
    <div className="row-actions confirm-actions">
      <button type="button" onClick={onCancel}>
        {cancelLabel}
      </button>
      <button type="submit" disabled={busy || disabled}>
        {busy ? busyLabel : submitLabel}
      </button>
    </div>
  )
}
