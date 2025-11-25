import { NavLink, Outlet } from 'react-router-dom';
import {
  Activity,
  Server,
  Target,
  AlertCircle,
  Camera,
  Bell,
  Settings,
  ChevronRight,
  Radio,
} from 'lucide-react';

const navigation = [
  { name: 'Fleet Overview', href: '/', icon: Activity },
  { name: 'Agents', href: '/agents', icon: Server },
  { name: 'Targets', href: '/targets', icon: Target },
  { name: 'Incidents', href: '/incidents', icon: AlertCircle },
  { name: 'Snapshots', href: '/snapshots', icon: Camera },
  { name: 'Alerts', href: '/alerts', icon: Bell },
];

const secondaryNavigation = [
  { name: 'Settings', href: '/settings', icon: Settings },
];

export function Layout() {
  return (
    <div className="flex h-screen bg-pilot-navy-dark">
      {/* Sidebar */}
      <aside className="w-64 bg-pilot-navy flex flex-col border-r border-pilot-navy-light">
        {/* Logo */}
        <div className="h-16 flex items-center gap-3 px-6 border-b border-pilot-navy-light">
          <div className="p-2 bg-pilot-yellow rounded-lg">
            <Radio className="w-5 h-5 text-pilot-navy" />
          </div>
          <div>
            <span className="font-bold text-white">ICMP-Mon</span>
            <span className="block text-xs text-gray-500">Network Monitoring</span>
          </div>
        </div>

        {/* Primary Navigation */}
        <nav className="flex-1 px-3 py-4">
          <ul className="space-y-1">
            {navigation.map((item) => (
              <li key={item.name}>
                <NavLink
                  to={item.href}
                  className={({ isActive }) => `
                    flex items-center gap-3 px-3 py-2.5 rounded-lg
                    text-sm font-medium transition-colors
                    ${isActive
                      ? 'bg-pilot-yellow text-pilot-navy'
                      : 'text-gray-300 hover:bg-pilot-navy-light hover:text-white'
                    }
                  `}
                >
                  <item.icon className="w-5 h-5" />
                  {item.name}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        {/* Secondary Navigation */}
        <div className="px-3 py-4 border-t border-pilot-navy-light">
          <ul className="space-y-1">
            {secondaryNavigation.map((item) => (
              <li key={item.name}>
                <NavLink
                  to={item.href}
                  className={({ isActive }) => `
                    flex items-center gap-3 px-3 py-2.5 rounded-lg
                    text-sm font-medium transition-colors
                    ${isActive
                      ? 'bg-pilot-yellow text-pilot-navy'
                      : 'text-gray-300 hover:bg-pilot-navy-light hover:text-white'
                    }
                  `}
                >
                  <item.icon className="w-5 h-5" />
                  {item.name}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>

        {/* Status Footer */}
        <div className="px-4 py-3 bg-pilot-navy-dark/50 border-t border-pilot-navy-light">
          <div className="flex items-center gap-2 text-xs">
            <span className="flex h-2 w-2 rounded-full bg-status-healthy animate-pulse-status" />
            <span className="text-gray-400">Control Plane Connected</span>
          </div>
        </div>
      </aside>

      {/* Main Content */}
      <main className="flex-1 flex flex-col overflow-hidden">
        <Outlet />
      </main>
    </div>
  );
}

export function PageHeader({ title, description, actions, breadcrumbs }) {
  return (
    <header className="flex-shrink-0 h-16 bg-pilot-navy border-b border-pilot-navy-light px-6 flex items-center justify-between">
      <div>
        {breadcrumbs && (
          <nav className="flex items-center gap-1 text-sm text-gray-400 mb-0.5">
            {breadcrumbs.map((crumb, index) => (
              <span key={index} className="flex items-center gap-1">
                {index > 0 && <ChevronRight className="w-3 h-3" />}
                {crumb.href ? (
                  <NavLink to={crumb.href} className="hover:text-white transition-colors">
                    {crumb.label}
                  </NavLink>
                ) : (
                  <span className="text-gray-300">{crumb.label}</span>
                )}
              </span>
            ))}
          </nav>
        )}
        <h1 className="text-xl font-semibold text-white">{title}</h1>
        {description && !breadcrumbs && (
          <p className="text-sm text-gray-400">{description}</p>
        )}
      </div>
      {actions && <div className="flex items-center gap-3">{actions}</div>}
    </header>
  );
}

export function PageContent({ children, className = '' }) {
  return (
    <div className={`flex-1 overflow-auto p-6 ${className}`}>
      <div className="max-w-7xl mx-auto">
        {children}
      </div>
    </div>
  );
}
