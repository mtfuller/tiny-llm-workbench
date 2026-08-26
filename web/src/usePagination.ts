import { useCallback, useState } from 'react'

const PAGE_SIZE = 20

// usePagination slices items into PAGE_SIZE-sized pages. The current page
// clamps automatically when items shrinks (e.g. a search narrows the list)
// so callers never end up stuck on a page past the end; call resetPage
// yourself (typically in a useEffect keyed on your filter inputs) to jump
// back to the first page when the user changes what they're filtering by.
// resetPage is wrapped in useCallback so it has a stable identity across
// renders — callers put it in a useEffect dependency array, and a
// same-render-always-a-new-function resetPage would make that effect (and
// its setPage(0)) fire on every render, not just when the real filter
// inputs change, snapping the page back to 0 the instant the user clicked
// "Next".
export function usePagination<T>(items: T[], pageSize = PAGE_SIZE) {
  const [page, setPage] = useState(0)
  const pageCount = Math.max(1, Math.ceil(items.length / pageSize))
  const currentPage = Math.min(page, pageCount - 1)
  const pageStart = currentPage * pageSize
  const pageItems = items.slice(pageStart, pageStart + pageSize)
  const resetPage = useCallback(() => setPage(0), [])

  return { page: currentPage, setPage, resetPage, pageCount, pageItems }
}
