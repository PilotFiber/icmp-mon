import { AlertTriangle, CheckCircle, Info, XCircle, X } from 'lucide-react';

const alertConfig = {
  info: {
    bg: 'bg-pilot-cyan/10 border-pilot-cyan',
    icon: Info,
    iconColor: 'text-pilot-cyan',
  },
  success: {
    bg: 'bg-status-healthy/10 border-status-healthy',
    icon: CheckCircle,
    iconColor: 'text-status-healthy',
  },
  warning: {
    bg: 'bg-warning/10 border-warning',
    icon: AlertTriangle,
    iconColor: 'text-warning',
  },
  error: {
    bg: 'bg-pilot-red/10 border-pilot-red',
    icon: XCircle,
    iconColor: 'text-pilot-red',
  },
};

export function Alert({
  type = 'info',
  title,
  children,
  onDismiss,
  className = '',
}) {
  const config = alertConfig[type];
  const Icon = config.icon;

  return (
    <div
      className={`
        rounded-lg border-l-4 p-4 animate-fade-in
        ${config.bg}
        ${className}
      `}
    >
      <div className="flex items-start gap-3">
        <Icon className={`w-5 h-5 mt-0.5 ${config.iconColor}`} />
        <div className="flex-1">
          {title && (
            <h4 className="font-medium text-theme-primary mb-1">{title}</h4>
          )}
          <div className="text-sm text-theme-secondary">{children}</div>
        </div>
        {onDismiss && (
          <button
            onClick={onDismiss}
            className="text-theme-muted hover:text-theme-primary transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        )}
      </div>
    </div>
  );
}

export function AlertCard({
  severity = 'warning',
  title,
  message,
  timestamp,
  target,
  agent,
  onAcknowledge,
  className = '',
}) {
  const severityConfig = {
    critical: {
      bg: 'bg-pilot-red/10',
      border: 'border-l-pilot-red',
      badge: 'bg-pilot-red text-theme-primary',
    },
    warning: {
      bg: 'bg-warning/10',
      border: 'border-l-warning',
      badge: 'bg-warning text-neutral-900',
    },
    info: {
      bg: 'bg-pilot-cyan/10',
      border: 'border-l-pilot-cyan',
      badge: 'bg-pilot-cyan text-neutral-900',
    },
  };

  const config = severityConfig[severity] || severityConfig.warning;

  return (
    <div
      className={`
        rounded-lg border-l-4 p-4
        ${config.bg} ${config.border}
        ${className}
      `}
    >
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1">
          <div className="flex items-center gap-2 mb-2">
            <span className={`px-2 py-0.5 text-xs font-medium rounded ${config.badge}`}>
              {severity.toUpperCase()}
            </span>
            {timestamp && (
              <span className="text-xs text-theme-muted">{timestamp}</span>
            )}
          </div>
          <h4 className="font-medium text-theme-primary mb-1">{title}</h4>
          <p className="text-sm text-theme-muted">{message}</p>
          {(target || agent) && (
            <div className="flex gap-4 mt-2 text-xs text-theme-muted">
              {target && <span>Target: {target}</span>}
              {agent && <span>Agent: {agent}</span>}
            </div>
          )}
        </div>
        {onAcknowledge && (
          <button
            onClick={onAcknowledge}
            className="text-sm text-pilot-cyan hover:text-pilot-cyan-light transition-colors"
          >
            Acknowledge
          </button>
        )}
      </div>
    </div>
  );
}
