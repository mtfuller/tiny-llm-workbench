import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'

// SortableHeader is a clickable results-table column header with a
// direction indicator. Used by the Benchmark and Evaluation run-results
// tables (previously each hand-rolled a `sortIcon()` helper plus a
// `<button className="sort-header">` per column).
interface SortableHeaderProps<K extends string> {
  label: string
  columnKey: K
  activeKey: K
  dir: 'asc' | 'desc'
  onSort: (key: K) => void
  title?: string
}

export default function SortableHeader<K extends string>({
  label,
  columnKey,
  activeKey,
  dir,
  onSort,
  title,
}: SortableHeaderProps<K>) {
  return (
    <th>
      <button type="button" className="sort-header" onClick={() => onSort(columnKey)} title={title}>
        {label}
        {activeKey !== columnKey ? (
          <ArrowUpDown size={13} className="sort-icon sort-icon-inactive" />
        ) : dir === 'asc' ? (
          <ArrowUp size={13} className="sort-icon" />
        ) : (
          <ArrowDown size={13} className="sort-icon" />
        )}
      </button>
    </th>
  )
}
