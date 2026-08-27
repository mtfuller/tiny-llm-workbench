import { Tags } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

interface TagFilterDropdownProps {
  tags: string[]
  active: Set<string>
  onToggle: (tag: string) => void
  onClear: () => void
}

// TagFilterDropdown collapses the per-tag filter chips into a single toggle
// button + popover, so the toolbar stays a fixed width regardless of how
// many tags a list has accumulated. The popover is portaled to
// document.body (positioned from the button's own bounding rect) rather
// than rendered in place, since it typically lives inside a .panel-flush
// ancestor whose overflow:hidden (needed to clip the table's square corners
// to the panel's rounded ones) would otherwise cut it off.
function TagFilterDropdown({ tags, active, onToggle, onClear }: TagFilterDropdownProps) {
  const [open, setOpen] = useState(false)
  const [menuPos, setMenuPos] = useState<{ top: number; left: number } | null>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const rect = buttonRef.current?.getBoundingClientRect()
    if (rect) setMenuPos({ top: rect.bottom + 6, left: rect.left })

    const handleClick = (e: MouseEvent) => {
      const target = e.target as Node
      if (buttonRef.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    const handleScroll = () => setOpen(false)
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    window.addEventListener('scroll', handleScroll, true)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
      window.removeEventListener('scroll', handleScroll, true)
    }
  }, [open])

  return (
    <div className="tag-filter-dropdown">
      <button
        ref={buttonRef}
        type="button"
        className={`icon-button${active.size > 0 ? ' icon-button-active' : ''}`}
        title="Filter by tag"
        aria-label="Filter by tag"
        onClick={() => setOpen((o) => !o)}
      >
        <Tags size={16} />
        {active.size > 0 && <span className="icon-button-badge">{active.size}</span>}
      </button>
      {open &&
        menuPos &&
        createPortal(
          <div className="tag-filter-menu" ref={menuRef} style={{ top: menuPos.top, left: menuPos.left }}>
            {tags.map((tag) => (
              <label className="tag-filter-menu-item" key={tag}>
                <input type="checkbox" checked={active.has(tag)} onChange={() => onToggle(tag)} />
                {tag}
              </label>
            ))}
            {active.size > 0 && (
              <button type="button" className="tag-filter-menu-clear" onClick={onClear}>
                Clear filter
              </button>
            )}
          </div>,
          document.body,
        )}
    </div>
  )
}

export default TagFilterDropdown
