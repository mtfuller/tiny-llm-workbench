import type { ButtonHTMLAttributes, ReactNode } from 'react'

// IconButton is the small square `.icon-button` used for row actions and
// toolbar actions throughout the app. It forces `title`/`aria-label` to be
// set (they were occasionally missing) from a single `label` prop.
interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'aria-label' | 'children'> {
  icon: ReactNode
  label: string
}

export default function IconButton({ icon, label, className, type = 'button', ...rest }: IconButtonProps) {
  return (
    <button type={type} className={`icon-button${className ? ` ${className}` : ''}`} title={label} aria-label={label} {...rest}>
      {icon}
    </button>
  )
}
