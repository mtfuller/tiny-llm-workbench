import type { ReactNode } from 'react'
import Pagination from './Pagination'
import { TableSkeleton } from './Skeleton'

// ListPanel is the chrome around every top-level resource list: the
// search-and-actions toolbar, the four mutually-exclusive states
// (error / loading / empty / no-match), the table itself, and pagination.
// Pair it with useResourceList.
interface ListPanelProps {
  search: string
  onSearch: (value: string) => void
  searchPlaceholder: string
  actions?: ReactNode

  // From useResourceList: `items` is null while loading.
  error?: string | null
  loading: boolean
  isEmpty: boolean
  hasMatches: boolean

  emptyMessage: string
  noMatchMessage?: string
  skeletonColumns: number

  // Pagination wiring (spread the rest of useResourceList's return here).
  page: number
  pageCount: number
  setPage: (page: number) => void
  shownCount: number
  totalCount: number
  itemLabel: string

  // The <table> — rendered only when there are matching rows.
  children: ReactNode
}

export default function ListPanel({
  search,
  onSearch,
  searchPlaceholder,
  actions,
  error,
  loading,
  isEmpty,
  hasMatches,
  emptyMessage,
  noMatchMessage = 'Nothing matches your search.',
  skeletonColumns,
  page,
  pageCount,
  setPage,
  shownCount,
  totalCount,
  itemLabel,
  children,
}: ListPanelProps) {
  return (
    <div className="panel panel-flush">
      <div className="list-toolbar panel-toolbar">
        <input
          type="search"
          placeholder={searchPlaceholder}
          value={search}
          onChange={(e) => onSearch(e.target.value)}
          className="list-search"
        />
        {actions && <div className="list-toolbar-actions">{actions}</div>}
      </div>

      {error ? (
        <div className="panel-body">
          <p className="error">{error}</p>
        </div>
      ) : loading ? (
        <div className="panel-body">
          <TableSkeleton columns={skeletonColumns} />
        </div>
      ) : isEmpty ? (
        <div className="panel-body">
          <p className="hint">{emptyMessage}</p>
        </div>
      ) : !hasMatches ? (
        <div className="panel-body">
          <p className="hint">{noMatchMessage}</p>
        </div>
      ) : (
        children
      )}

      <Pagination
        page={page}
        pageCount={pageCount}
        onChange={setPage}
        shownCount={shownCount}
        totalCount={totalCount}
        itemLabel={itemLabel}
      />
    </div>
  )
}
