import { useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import {
  Activity,
  Server,
  Target,
  AlertCircle,
  Camera,
  Bell,
  Settings,
  ChevronRight,
  Rocket,
  BarChart3,
  Sun,
  Moon,
  Network,
  ClipboardCheck,
  Grid,
  Database,
  Menu,
  X,
} from 'lucide-react';
import { useTheme } from '../context/ThemeContext';
import { PilotLogo } from './PilotLogo';

const navigation = [
  { name: 'Dashboard', href: '/', icon: Activity },
  { name: 'Agents', href: '/agents', icon: Server },
  { name: 'Infrastructure', href: '/infrastructure', icon: Database },
  { name: 'Fleet Management', href: '/fleet', icon: Rocket },
  { name: 'Targets', href: '/targets', icon: Target },
  { name: 'Subnets', href: '/subnets', icon: Network },
  { name: 'Review Queue', href: '/review', icon: ClipboardCheck },
  { name: 'Metrics Explorer', href: '/metrics', icon: BarChart3 },
  { name: 'Latency Matrix', href: '/latency-matrix', icon: Grid },
  { name: 'Incidents', href: '/incidents', icon: AlertCircle },
  { name: 'Snapshots', href: '/snapshots', icon: Camera },
  { name: 'Alerts', href: '/alerts', icon: Bell },
];

const secondaryNavigation = [
  { name: 'Settings', href: '/settings', icon: Settings },
];

export function Layout() {
  const { theme, toggleTheme } = useTheme();
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const location = useLocation();

  // Close sidebar on navigation
  const handleNavClick = () => {
    setSidebarOpen(false);
  };

  return (
    <div className="flex h-screen bg-surface-primary">
      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-30 md:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside className={`
        fixed inset-y-0 left-0 z-40 w-64 bg-surface-secondary flex flex-col border-r border-theme
        transform transition-transform duration-200 ease-in-out
        ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}
        md:relative md:translate-x-0
      `}>
        {/* Logo */}
        <div className="h-14 md:h-16 flex items-center justify-between px-4 md:px-6 border-b border-theme">
          <div className="flex items-center gap-3">
            <PilotLogo dark={theme === 'dark'} />
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={toggleTheme}
              className="p-2 rounded-lg text-theme-muted hover:text-theme-primary hover:bg-surface-elevated transition-colors"
              title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
            >
              {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
            </button>
            {/* Close button on mobile */}
            <button
              onClick={() => setSidebarOpen(false)}
              className="p-2 rounded-lg text-theme-muted hover:text-theme-primary hover:bg-surface-elevated transition-colors md:hidden"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Primary Navigation */}
        <nav className="flex-1 px-3 py-4 overflow-y-auto">
          <ul className="space-y-1">
            {navigation.map((item) => (
              <li key={item.name}>
                <NavLink
                  to={item.href}
                  onClick={handleNavClick}
                  className={({ isActive }) => `
                    flex items-center gap-3 px-3 py-2.5 rounded-lg
                    text-sm font-medium transition-colors
                    ${isActive
                      ? 'bg-pilot-yellow text-neutral-900'
                      : 'text-theme-secondary hover:bg-surface-elevated hover:text-theme-primary'
                    }
                  `}
                >
                  <item.icon className="w-5 h-5 flex-shrink-0" />
                  <span className="truncate">{item.name}</span>
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        {/* Secondary Navigation */}
        <div className="px-3 py-4 border-t border-theme">
          <ul className="space-y-1">
            {secondaryNavigation.map((item) => (
              <li key={item.name}>
                <NavLink
                  to={item.href}
                  onClick={handleNavClick}
                  className={({ isActive }) => `
                    flex items-center gap-3 px-3 py-2.5 rounded-lg
                    text-sm font-medium transition-colors
                    ${isActive
                      ? 'bg-pilot-yellow text-neutral-900'
                      : 'text-theme-secondary hover:bg-surface-elevated hover:text-theme-primary'
                    }
                  `}
                >
                  <item.icon className="w-5 h-5 flex-shrink-0" />
                  <span className="truncate">{item.name}</span>
                </NavLink>
              </li>
            ))}
          </ul>
        </div>

        {/* Status Footer */}
        <div className="px-4 py-3 bg-surface-primary/50 border-t border-theme">
          <div className="flex items-center gap-2 text-xs">
            <span className="flex h-2 w-2 rounded-full bg-status-healthy animate-pulse-status" />
            <span className="text-theme-muted">Control Plane Connected</span>
          </div>
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col overflow-hidden bg-surface-primary w-full">
        {/* Mobile header with hamburger */}
        <div className="md:hidden flex items-center h-14 px-4 bg-surface-secondary border-b border-theme">
          <button
            onClick={() => setSidebarOpen(true)}
            className="p-2 -ml-2 rounded-lg text-theme-muted hover:text-theme-primary hover:bg-surface-elevated transition-colors"
          >
            <Menu className="w-6 h-6" />
          </button>
          <div className="ml-3">
            <PilotLogo dark={theme === 'dark'} />
          </div>
        </div>
        <Outlet />
      </main>
    </div>
  );
}

export function PageHeader({ title, description, actions, breadcrumbs }) {
  return (
    <header className="flex-shrink-0 min-h-14 md:h-16 bg-surface-secondary border-b border-theme px-4 md:px-6 py-3 md:py-0 flex flex-col md:flex-row md:items-center justify-between gap-3 md:gap-0">
      <div className="min-w-0">
        {breadcrumbs && (
          <nav className="flex items-center gap-1 text-xs md:text-sm text-theme-muted mb-0.5 overflow-x-auto">
            {breadcrumbs.map((crumb, index) => (
              <span key={index} className="flex items-center gap-1 whitespace-nowrap">
                {index > 0 && <ChevronRight className="w-3 h-3 flex-shrink-0" />}
                {crumb.href ? (
                  <NavLink to={crumb.href} className="hover:text-theme-primary transition-colors">
                    {crumb.label}
                  </NavLink>
                ) : (
                  <span className="text-theme-secondary">{crumb.label}</span>
                )}
              </span>
            ))}
          </nav>
        )}
        <h1 className="text-lg md:text-xl font-semibold text-theme-primary truncate">{title}</h1>
        {description && !breadcrumbs && (
          <p className="text-xs md:text-sm text-theme-muted">{description}</p>
        )}
      </div>
      {actions && <div className="flex items-center gap-2 md:gap-3 flex-shrink-0">{actions}</div>}
    </header>
  );
}

export function PageContent({ children, className = '' }) {
  return (
    <div className={`flex-1 overflow-auto p-4 md:p-6 ${className}`}>
      <div className="max-w-7xl mx-auto">
        {children}
      </div>
    </div>
  );
}
