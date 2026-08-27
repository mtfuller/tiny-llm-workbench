import { useEffect, useRef, useState } from 'react'

export interface PopoverPosition {
  top: number
  right: number
}

// usePopoverMenu is the shared open/position/dismiss logic behind every
// button-triggered dropdown in the app (FilterMenu, VariableMenuButton) —
// pulled out once a second near-identical implementation showed up,
// mirroring this project's own established threshold for when duplicated
// logic is worth extracting (see internal/assertions in CLAUDE.md).
//
// The menu is right-anchored (`right: window.innerWidth - trigger.right`),
// not left-anchored — a trigger button near the right edge of its
// container (as several of these are) would otherwise let the popover's
// own width push it off the viewport, which is a real bug this once
// shipped and had to be fixed after the fact.
export function usePopoverMenu<T extends HTMLElement = HTMLButtonElement>() {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<PopoverPosition | null>(null)
  const triggerRef = useRef<T>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const rect = triggerRef.current?.getBoundingClientRect()
    if (rect) setPosition({ top: rect.bottom + 6, right: window.innerWidth - rect.right })

    const handleClick = (e: MouseEvent) => {
      const target = e.target as Node
      if (triggerRef.current?.contains(target) || menuRef.current?.contains(target)) return
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

  return { open, setOpen, position, triggerRef, menuRef }
}
