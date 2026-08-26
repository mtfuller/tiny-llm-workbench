import { useRef, type ChangeEvent, type UIEvent } from 'react'

interface LineNumberedTextareaProps {
  value: string
  onChange: (value: string) => void
  rows?: number
  placeholder?: string
  autoFocus?: boolean
}

// LineNumberedTextarea is a small plaintext-editor-style input — a
// monospace textarea with a synced line-number gutter — for content where
// exact line breaks and spacing matter (e.g. dataset example input/output).
// Lines don't wrap, so each visual row matches one gutter number; long
// lines scroll horizontally instead.
function LineNumberedTextarea({ value, onChange, rows = 6, placeholder, autoFocus }: LineNumberedTextareaProps) {
  const gutterRef = useRef<HTMLDivElement>(null)
  const lineCount = value === '' ? 1 : value.split('\n').length

  const syncGutterScroll = (e: UIEvent<HTMLTextAreaElement>) => {
    if (gutterRef.current) gutterRef.current.scrollTop = e.currentTarget.scrollTop
  }

  return (
    <div className="line-editor">
      <div className="line-editor-gutter" ref={gutterRef} aria-hidden="true">
        {Array.from({ length: lineCount }, (_, i) => (
          <div key={i}>{i + 1}</div>
        ))}
      </div>
      <textarea
        className="line-editor-textarea"
        value={value}
        onChange={(e: ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)}
        onScroll={syncGutterScroll}
        placeholder={placeholder}
        autoFocus={autoFocus}
        spellCheck={false}
        wrap="off"
        rows={rows}
      />
    </div>
  )
}

export default LineNumberedTextarea
