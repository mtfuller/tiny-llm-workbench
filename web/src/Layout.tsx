import { Activity, BarChart3, Box, ClipboardCheck, Container, Database, Settings, Trophy, Workflow } from 'lucide-react'
import type { ComponentType } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { useEventStream } from './eventStream'
import './index.css'

interface NavItem {
  to: string
  label: string
  icon: ComponentType<{ size?: number; strokeWidth?: number }>
  end?: boolean
}

interface NavSection {
  label?: string
  items: NavItem[]
}

const navSections: NavSection[] = [
  { items: [{ to: '/', label: 'Home', icon: Activity, end: true }] },
  {
    label: 'Workbench',
    items: [
      { to: '/models', label: 'Models', icon: Box },
      { to: '/datasets', label: 'Datasets', icon: Database },
      { to: '/training', label: 'Training', icon: BarChart3 },
      { to: '/benchmarks', label: 'Benchmarks', icon: Trophy },
    ],
  },
  {
    label: 'Automation',
    items: [
      { to: '/environments', label: 'Environments', icon: Container },
      { to: '/agents', label: 'Agents', icon: Workflow },
      { to: '/evaluations', label: 'Evaluations', icon: ClipboardCheck },
    ],
  },
]

function Logo() {
  return (
    <svg width="28" height="28" viewBox="0 0 28 28" fill="none" aria-hidden="true">
      <rect width="28" height="28" rx="8" fill="var(--accent-soft)" />
      <circle cx="11" cy="10.5" r="3.4" fill="var(--accent)" />
      <circle cx="18" cy="9.5" r="2.4" fill="#f2a33f" />
      <circle cx="14.5" cy="18" r="3" fill="#e8618c" />
    </svg>
  )
}

function connectionLabel(status: ReturnType<typeof useEventStream>['status']): string {
  if (status === 'open') return 'Online'
  if (status === 'connecting') return 'Connecting'
  return 'Offline'
}

function Layout() {
  const { status } = useEventStream()
  const location = useLocation()

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <Logo />
          <span>Tiny LLM Workbench</span>
        </div>

        <nav className="sidebar-nav">
          {navSections.map((section) => (
            <div className="sidebar-nav-section" key={section.label ?? 'root'}>
              {section.label && <div className="sidebar-nav-label">{section.label}</div>}
              {section.items.map((item) => {
                const Icon = item.icon
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.end}
                    className={({ isActive }) => `nav-item${isActive ? ' nav-item-active' : ''}`}
                  >
                    <Icon size={17} strokeWidth={2} />
                    {item.label}
                  </NavLink>
                )
              })}
            </div>
          ))}
        </nav>

        <div className="sidebar-footer">
          <NavLink to="/settings" className={({ isActive }) => `nav-item${isActive ? ' nav-item-active' : ''}`}>
            <Settings size={17} strokeWidth={2} />
            Settings
          </NavLink>
          <div className={`status-row status-row-${status}`}>
            <span className="status-dot" />
            API: {connectionLabel(status)}
          </div>
        </div>
      </aside>

      <div className="app-main">
        <main className="page-content">
          <div className="page-transition" key={location.pathname}>
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}

export default Layout
