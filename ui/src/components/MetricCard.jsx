import { TrendingUp, TrendingDown, Minus } from 'lucide-react';

export function MetricCard({
  title,
  value,
  unit = '',
  subtitle = '',
  change = null,
  changeLabel = '',
  icon: Icon = null,
  status = null,
  size = 'default',
  className = '',
}) {
  const getTrendIcon = () => {
    if (change === null || change === 0) {
      return <Minus className="w-3 h-3" />;
    }
    if (change > 0) {
      return <TrendingUp className="w-3 h-3" />;
    }
    return <TrendingDown className="w-3 h-3" />;
  };

  const getTrendColor = () => {
    if (change === null || change === 0) return 'text-theme-muted';
    if (change > 0) return 'text-status-healthy';
    return 'text-status-down';
  };

  const accentLine = {
    healthy: 'bg-emerald-500',
    degraded: 'bg-amber-500',
    down: 'bg-red-500',
  };

  const isSmall = size === 'sm';

  return (
    <div
      className={`
        relative rounded-lg bg-surface-secondary border border-theme
        ${isSmall ? 'p-3' : 'p-4'}
        ${className}
      `}
    >
      {/* Status accent line */}
      {status && (
        <div className={`absolute left-0 top-2 bottom-2 w-0.5 rounded-full ${accentLine[status]}`} />
      )}

      <div className={`flex items-center justify-between ${status ? 'pl-3' : ''}`}>
        <div className="min-w-0">
          <p className="text-xs text-theme-muted mb-1">{title}</p>
          <div className="flex items-baseline gap-1">
            <span className={`${isSmall ? 'text-lg' : 'text-xl'} font-semibold text-theme-secondary tabular-nums`}>
              {value}
            </span>
            {unit && (
              <span className="text-xs text-theme-muted">{unit}</span>
            )}
          </div>
          {subtitle && (
            <p className="text-xs text-theme-muted mt-0.5">{subtitle}</p>
          )}
          {change !== null && (
            <div className={`flex items-center gap-1 mt-1 ${getTrendColor()}`}>
              {getTrendIcon()}
              <span className="text-xs tabular-nums">
                {change > 0 ? '+' : ''}{change}%
              </span>
              {changeLabel && (
                <span className="text-theme-muted text-xs">{changeLabel}</span>
              )}
            </div>
          )}
        </div>
        {Icon && (
          <Icon className={`${isSmall ? 'w-4 h-4' : 'w-5 h-5'} text-theme-muted`} />
        )}
      </div>
    </div>
  );
}

export function MetricCardCompact({ title, value, unit = '', className = '' }) {
  return (
    <div className={`text-center ${className}`}>
      <p className="text-xs text-theme-muted uppercase tracking-wide">{title}</p>
      <div className="flex items-baseline justify-center gap-0.5 mt-1">
        <span className="text-xl font-bold text-theme-primary">{value}</span>
        {unit && <span className="text-sm text-theme-muted">{unit}</span>}
      </div>
    </div>
  );
}
