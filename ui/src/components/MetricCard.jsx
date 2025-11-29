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
        ${isSmall ? 'p-2.5 sm:p-3' : 'p-3 sm:p-4'}
        ${className}
      `}
    >
      {/* Status accent line */}
      {status && (
        <div className={`absolute left-0 top-2 bottom-2 w-0.5 rounded-full ${accentLine[status]}`} />
      )}

      <div className={`flex items-center justify-between ${status ? 'pl-2 sm:pl-3' : ''}`}>
        <div className="min-w-0 flex-1">
          <p className="text-[10px] sm:text-xs text-theme-muted mb-0.5 sm:mb-1 truncate">{title}</p>
          <div className="flex items-baseline gap-1">
            <span className={`${isSmall ? 'text-base sm:text-lg' : 'text-lg sm:text-xl'} font-semibold text-theme-secondary tabular-nums truncate`}>
              {value}
            </span>
            {unit && (
              <span className="text-[10px] sm:text-xs text-theme-muted">{unit}</span>
            )}
          </div>
          {subtitle && (
            <p className="text-[10px] sm:text-xs text-theme-muted mt-0.5 truncate">{subtitle}</p>
          )}
          {change !== null && (
            <div className={`flex items-center gap-1 mt-1 ${getTrendColor()}`}>
              {getTrendIcon()}
              <span className="text-[10px] sm:text-xs tabular-nums">
                {change > 0 ? '+' : ''}{change}%
              </span>
              {changeLabel && (
                <span className="text-theme-muted text-[10px] sm:text-xs hidden sm:inline">{changeLabel}</span>
              )}
            </div>
          )}
        </div>
        {Icon && (
          <Icon className={`${isSmall ? 'w-3.5 h-3.5 sm:w-4 sm:h-4' : 'w-4 h-4 sm:w-5 sm:h-5'} text-theme-muted flex-shrink-0 ml-2`} />
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
