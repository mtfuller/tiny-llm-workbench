import { ArrowRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

interface LinkArrowProps {
  to: string
  children: ReactNode
}

// LinkArrow is a "go to X" navigational link — label plus a trailing arrow
// that nudges right on hover — used in place of a raw "→" glyph wherever a
// link's whole job is sending the reader somewhere else (not an inline
// reference, like a name in a table row).
function LinkArrow({ to, children }: LinkArrowProps) {
  return (
    <Link className="link-arrow" to={to}>
      {children}
      <ArrowRight size={14} />
    </Link>
  )
}

export default LinkArrow
