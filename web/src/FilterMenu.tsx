import { ListFilter } from 'lucide-react'
import { createPortal } from 'react-dom'
import { usePopoverMenu } from './usePopoverMenu'

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
// rendered empty.
function FilterMenu({ groups, onClearAll }: FilterMenuProps) {
  const { open, setOpen, position, triggerRef, menuRef } = usePopoverMenu()

  const visibleGroups = groups.filter((g) => g.options.length > 0)
  const activeCount = groups.reduce((sum, g) => sum + g.active.size, 0)

  return (
    <div className="dropdown-menu-anchor">
      <button
        ref={triggerRef}
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
        position &&
        createPortal(
          <div className="dropdown-menu" ref={menuRef} style={{ top: position.top, right: position.right }}>
            {visibleGroups.map((group, i) => (
              <div className="dropdown-menu-group" key={group.key}>
                {i > 0 && <div className="dropdown-menu-divider" />}
                <div className="dropdown-menu-group-title">{group.title}</div>
                {group.options.map((opt) => (
                  <label className="dropdown-menu-item dropdown-menu-item-checkbox" key={opt}>
                    <input type="checkbox" checked={group.active.has(opt)} onChange={() => group.onToggle(opt)} />
                    {opt}
                  </label>
                ))}
              </div>
            ))}
            {activeCount > 0 && (
              <button type="button" className="dropdown-menu-clear" onClick={onClearAll}>
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
