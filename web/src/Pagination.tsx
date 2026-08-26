interface PaginationProps {
  page: number
  pageCount: number
  onChange: (page: number) => void
  shownCount: number
  totalCount: number
  itemLabel: string
}

// Pagination renders the "Previous / Page X of Y / Next" footer shared by
// every paginated table-list page. Renders nothing for a single page, so
// callers can render it unconditionally.
function Pagination({ page, pageCount, onChange, shownCount, totalCount, itemLabel }: PaginationProps) {
  if (pageCount <= 1) return null

  return (
    <div className="pagination">
      <button type="button" disabled={page === 0} onClick={() => onChange(page - 1)}>
        Previous
      </button>
      <span className="hint">
        Page {page + 1} of {pageCount} ({shownCount} of {totalCount} {itemLabel})
      </span>
      <button type="button" disabled={page >= pageCount - 1} onClick={() => onChange(page + 1)}>
        Next
      </button>
    </div>
  )
}

export default Pagination
