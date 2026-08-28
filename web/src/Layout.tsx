import {
  Activity,
  BarChart3,
  BookOpen,
  Box,
  ClipboardCheck,
  Container,
  Database,
  PanelLeftClose,
  PanelLeftOpen,
  Rocket,
  Settings,
  Trophy,
  Wrench,
  Workflow,
} from 'lucide-react'
import { useEffect, useState, type ComponentType } from 'react'
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
    label: 'Environment',
    items: [
      { to: '/workspaces', label: 'Workspaces', icon: Container },
      { to: '/knowledge', label: 'Knowledge', icon: BookOpen },
      { to: '/tools', label: 'Tools', icon: Wrench },
    ],
  },
  {
    label: 'Automation',
    items: [
      { to: '/agents', label: 'Agents', icon: Workflow },
      { to: '/evaluations', label: 'Evaluations', icon: ClipboardCheck },
      { to: '/deployments', label: 'Deployments', icon: Rocket },
    ],
  },
]

const SIDEBAR_COLLAPSED_KEY = 'tlw-sidebar-collapsed'

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

  // Persisted per-browser (not per-agent-data), so it belongs in
  // localStorage rather than anything server-side — purely a per-viewer
  // display preference.
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === 'true'
    } catch {
      return false
    }
  })

  useEffect(() => {
    try {
      localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(collapsed))
    } catch {
      // ignore — a private window or blocked storage just means the
      // preference doesn't survive a reload, which is fine.
    }
  }, [collapsed])

  return (
    <div className="app-shell">
      <aside className={`sidebar${collapsed ? ' sidebar-collapsed' : ''}`}>
        <div className="sidebar-brand">
          <Logo />
          {!collapsed && <span>Tiny LLM Workbench</span>}
        </div>

        <nav className="sidebar-nav">
          {navSections.map((section) => (
            <div className="sidebar-nav-section" key={section.label ?? 'root'}>
              {section.label && !collapsed && <div className="sidebar-nav-label">{section.label}</div>}
              {section.items.map((item) => {
                const Icon = item.icon
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.end}
                    title={collapsed ? item.label : undefined}
                    className={({ isActive }) => `nav-item${isActive ? ' nav-item-active' : ''}`}
                  >
                    <Icon size={17} strokeWidth={2} />
                    {!collapsed && item.label}
                  </NavLink>
                )
              })}
            </div>
          ))}
        </nav>

        <div className="sidebar-footer">
          <NavLink
            to="/settings"
            title={collapsed ? 'Settings' : undefined}
            className={({ isActive }) => `nav-item${isActive ? ' nav-item-active' : ''}`}
          >
            <Settings size={17} strokeWidth={2} />
            {!collapsed && 'Settings'}
          </NavLink>
          <div className={`status-row status-row-${status}`} title={collapsed ? connectionLabel(status) : undefined}>
            <span className="status-dot" />
            {!collapsed && `API: ${connectionLabel(status)}`}
          </div>
          <button
            type="button"
            className="sidebar-collapse-toggle"
            onClick={() => setCollapsed((c) => !c)}
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {collapsed ? <PanelLeftOpen size={16} strokeWidth={2} /> : <PanelLeftClose size={16} strokeWidth={2} />}
            {!collapsed && 'Collapse'}
          </button>
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
