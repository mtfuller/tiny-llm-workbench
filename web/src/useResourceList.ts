import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useConfirm } from './ConfirmDialog'
import { useToast } from './Toast'
import { usePagination } from './usePagination'

// useResourceList owns the state every top-level list page repeated by hand:
// load-into-nullable / error / search / client-side filter / pagination /
// confirm-then-delete-then-toast-then-reload. Pair it with <ListPanel> for
// the matching chrome.
//
// The options object is stashed in a ref, so callers can pass fresh
// closures each render without churning the memoised derivations.
interface Options<T> {
  load: () => Promise<T[]>
  // A stable identity string per item — used for the search haystack, the
  // per-row "deleting" flag, and the default confirm/toast text.
  getName: (item: T) => string
  // Extra text to fold into the search haystack beyond getName.
  searchText?: (item: T) => string
  remove?: (item: T) => Promise<void>
  confirmMessage?: (item: T) => string
  deletedToast?: (item: T) => string
}

export function useResourceList<T>(options: Options<T>) {
  const confirm = useConfirm()
  const showToast = useToast()
  const opts = useRef(options)
  opts.current = options

  const [items, setItems] = useState<T[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const reload = useCallback(() => {
    opts.current
      .load()
      .then(setItems)
      .catch((err: Error) => setError(err.message))
  }, [])

  useEffect(reload, [reload])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    const list = items ?? []
    if (!q) return list
    return list.filter((item) => {
      const hay = `${opts.current.getName(item)} ${opts.current.searchText?.(item) ?? ''}`.toLowerCase()
      return hay.includes(q)
    })
  }, [items, search])

  const pagination = usePagination(filtered)
  useEffect(pagination.resetPage, [search, pagination.resetPage])

  const handleDelete = useCallback(
    async (item: T) => {
      const { remove, getName, confirmMessage, deletedToast } = opts.current
      if (!remove) return
      const name = getName(item)
      const message = confirmMessage?.(item) ?? `Delete "${name}"? This cannot be undone.`
      if (!(await confirm(message))) return

      setDeleting(name)
      setError(null)
      try {
        await remove(item)
        if (deletedToast) showToast(deletedToast(item))
        reload()
      } catch (err) {
        setError((err as Error).message)
      } finally {
        setDeleting(null)
      }
    },
    [confirm, showToast, reload],
  )

  return {
    items,
    error,
    setError,
    search,
    setSearch,
    filtered,
    deleting,
    reload,
    handleDelete,
    ...pagination,
  }
}
