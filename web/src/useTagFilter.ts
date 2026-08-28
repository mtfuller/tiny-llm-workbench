import { useCallback, useMemo, useRef, useState } from 'react'

// useTagFilter derives the sorted set of all tags across `items` and tracks
// which ones are active, plus a `matchesTags` predicate (an item passes if
// it carries at least one active tag, or if nothing is filtered). Pair
// `allTags` / `activeTags` / `toggleTag` / `clearTags` with <FilterMenu>.
//
// This was copy-pasted into DatasetDetail, KnowledgeDetail, BenchmarkDetail
// and EvaluationDetail.
export function useTagFilter<T>(items: T[], getTags: (item: T) => string[] | undefined) {
  const getTagsRef = useRef(getTags)
  getTagsRef.current = getTags

  const [activeTags, setActiveTags] = useState<Set<string>>(new Set())

  const allTags = useMemo(() => {
    const set = new Set<string>()
    for (const item of items) for (const tag of getTagsRef.current(item) ?? []) set.add(tag)
    return [...set].sort()
  }, [items])

  const toggleTag = useCallback((tag: string) => {
    setActiveTags((prev) => {
      const next = new Set(prev)
      if (next.has(tag)) next.delete(tag)
      else next.add(tag)
      return next
    })
  }, [])

  const clearTags = useCallback(() => setActiveTags(new Set()), [])

  const matchesTags = useCallback(
    (item: T) => {
      if (activeTags.size === 0) return true
      const tags = getTagsRef.current(item) ?? []
      return [...activeTags].some((t) => tags.includes(t))
    },
    [activeTags],
  )

  return { allTags, activeTags, toggleTag, clearTags, matchesTags }
}
