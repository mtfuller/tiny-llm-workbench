import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'

interface Options {
  min: number
  max: number
  // A 'left' sidebar grows as the handle is dragged right; a 'right' sidebar
  // grows as it's dragged left.
  side: 'left' | 'right'
}

// useResizableSidebar drives a draggable width for one workspace sidebar,
// persisted to localStorage. Spread `resizerProps` onto a thin handle element
// sitting between the sidebar and the canvas; apply `width` as the sidebar's
// inline width. Double-click the handle to reset to `defaultWidth`.
export function useResizableSidebar(storageKey: string, defaultWidth: number, { min, max, side }: Options) {
  const clamp = (n: number) => Math.min(max, Math.max(min, n))

  const [width, setWidth] = useState<number>(() => {
    try {
      const saved = Number(localStorage.getItem(storageKey))
      if (Number.isFinite(saved) && saved > 0) return clamp(saved)
    } catch {
      /* ignore unavailable storage */
    }
    return defaultWidth
  })
  const [dragging, setDragging] = useState(false)
  const start = useRef({ x: 0, w: 0 })

  const onPointerDown = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.currentTarget.setPointerCapture(e.pointerId)
      start.current = { x: e.clientX, w: width }
      setDragging(true)
    },
    [width],
  )

  const onPointerMove = useCallback(
    (e: ReactPointerEvent<HTMLDivElement>) => {
      if (!dragging) return
      const raw = e.clientX - start.current.x
      setWidth(clamp(start.current.w + (side === 'left' ? raw : -raw)))
    },
    // clamp closes over min/max, which never change for a given mount
    [dragging, side], // eslint-disable-line react-hooks/exhaustive-deps
  )

  const stop = useCallback((e: ReactPointerEvent<HTMLDivElement>) => {
    setDragging(false)
    try {
      e.currentTarget.releasePointerCapture(e.pointerId)
    } catch {
      /* ignore */
    }
  }, [])

  const onDoubleClick = useCallback(() => setWidth(defaultWidth), [defaultWidth])

  // Persist once the drag settles (and on any programmatic reset).
  useEffect(() => {
    if (dragging) return
    try {
      localStorage.setItem(storageKey, String(width))
    } catch {
      /* ignore */
    }
  }, [dragging, width, storageKey])

  // Keep the col-resize cursor and kill text selection for the whole page
  // while dragging, so it doesn't flicker as the pointer crosses elements.
  useEffect(() => {
    if (!dragging) return
    const prevCursor = document.body.style.cursor
    const prevSelect = document.body.style.userSelect
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    return () => {
      document.body.style.cursor = prevCursor
      document.body.style.userSelect = prevSelect
    }
  }, [dragging])

  const resizerProps = {
    className: `workspace-resizer${dragging ? ' workspace-resizer-active' : ''}`,
    role: 'separator' as const,
    'aria-orientation': 'vertical' as const,
    onPointerDown,
    onPointerMove,
    onPointerUp: stop,
    onPointerCancel: stop,
    onDoubleClick,
  }

  return { width, dragging, resizerProps }
}
