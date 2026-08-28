import { useState } from 'react'

interface ExpandableProps {
  text: string
  // Character budget before truncating; the full text is one click away.
  limit?: number
  // Render as a <pre> (monospace, wrapped) rather than inline text.
  pre?: boolean
  className?: string
}

// Expandable shows `text` truncated to `limit` characters with a "Show more"
// toggle; short text renders as-is with no toggle. Used for the debug
// activity feed and step output, which can be long.
export default function Expandable({ text, limit = 220, pre = false, className }: ExpandableProps) {
  const [open, setOpen] = useState(false)
  const long = text.length > limit
  const shown = open || !long ? text : text.slice(0, limit).trimEnd() + '…'

  const body = pre ? (
    <pre className={`exec-output ${className ?? ''}`}>{shown || '(empty)'}</pre>
  ) : (
    <span className={className}>{shown}</span>
  )

  if (!long) return body

  return (
    <span className="expandable">
      {body}
      <button type="button" className="link-button expandable-toggle" onClick={() => setOpen((o) => !o)}>
        {open ? 'Show less' : 'Show more'}
      </button>
    </span>
  )
}
