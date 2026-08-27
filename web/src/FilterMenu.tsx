import { ListFilter } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

export interface FilterGroup {
  key: string
  title: string
  options: string[]
  active: Set<string>
  onToggle: (value: string) => void
}

interface FilterMenuProps {
  groups: FilterGroup[]
  onClearAll: () => void
}

// FilterMenu is a single toggle button + popover covering every filterable
// dimension a list page has — currently just a "Tags" group on the pages
// that use it, but the group list is what a future dimension (e.g.
// filtering Benchmark results by version) would slot into, rather than a
// second bespoke dropdown. Groups with no options are skipped rather than
// rendered empty. The popover is portaled to document.body (positioned from
// the button's own bounding rect, right-anchored so it can't clip off the
// viewport edge) rather than rendered in place, since this typically lives
// inside a .panel-flush ancestor whose overflow:hidden (needed to clip the
// table's square corners to the panel's rounded ones) would otherwise cut
// it off.
function FilterMenu({ groups, onClearAll }: FilterMenuProps) {
  const [open, setOpen] = useState(false)
  const [menuPos, setMenuPos] = useState<{ top: number; right: number } | null>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const visibleGroups = groups.filter((g) => g.options.length > 0)
  const activeCount = groups.reduce((sum, g) => sum + g.active.size, 0)

  useEffect(() => {
    if (!open) return
    const rect = buttonRef.current?.getBoundingClientRect()
    if (rect) setMenuPos({ top: rect.bottom + 6, right: window.innerWidth - rect.right })

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
    <div className="filter-menu-anchor">
      <button
        ref={buttonRef}
        type="button"
        className={`icon-button${activeCount > 0 ? ' icon-button-active' : ''}`}
        title="Filter"
        aria-label="Filter"
        onClick={() => setOpen((o) => !o)}
      >
        <ListFilter size={16} />
        {activeCount > 0 && <span className="icon-button-badge">{activeCount}</span>}
      </button>
      {open &&
        menuPos &&
        createPortal(
          <div className="filter-menu" ref={menuRef} style={{ top: menuPos.top, right: menuPos.right }}>
            {visibleGroups.map((group, i) => (
              <div className="filter-menu-group" key={group.key}>
                {i > 0 && <div className="filter-menu-divider" />}
                <div className="filter-menu-group-title">{group.title}</div>
                {group.options.map((opt) => (
                  <label className="filter-menu-item" key={opt}>
                    <input type="checkbox" checked={group.active.has(opt)} onChange={() => group.onToggle(opt)} />
                    {opt}
                  </label>
                ))}
              </div>
            ))}
            {activeCount > 0 && (
              <button type="button" className="filter-menu-clear" onClick={onClearAll}>
                Clear filters
              </button>
            )}
          </div>,
          document.body,
        )}
    </div>
  )
}

export default FilterMenu
