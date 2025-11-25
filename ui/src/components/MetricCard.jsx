import { TrendingUp, TrendingDown, Minus } from 'lucide-react';
import { Card } from './Card';

export function MetricCard({
  title,
  value,
  unit = '',
  change = null,
  changeLabel = '',
  icon: Icon = null,
  status = null,
  className = '',
}) {
  const getTrendIcon = () => {
    if (change === null || change === 0) {
      return <Minus className="w-4 h-4 text-gray-400" />;
    }
    if (change > 0) {
      return <TrendingUp className="w-4 h-4 text-status-healthy" />;
    }
    return <TrendingDown className="w-4 h-4 text-status-down" />;
  };

  const getTrendColor = () => {
    if (change === null || change === 0) return 'text-gray-400';
    if (change > 0) return 'text-status-healthy';
    return 'text-status-down';
  };

  const statusColors = {
    healthy: 'border-l-status-healthy',
    degraded: 'border-l-status-degraded',
    down: 'border-l-status-down',
  };

  return (
    <Card
      className={`
        ${status ? `border-l-4 ${statusColors[status]}` : ''}
        ${className}
      `}
    >
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <p className="text-sm text-gray-400 mb-1">{title}</p>
          <div className="flex items-baseline gap-1">
            <span className="text-3xl font-bold text-white">{value}</span>
            {unit && <span className="text-lg text-gray-400">{unit}</span>}
          </div>
          {change !== null && (
            <div className={`flex items-center gap-1 mt-2 ${getTrendColor()}`}>
              {getTrendIcon()}
              <span className="text-sm font-medium">
                {change > 0 ? '+' : ''}{change}%
              </span>
              {changeLabel && (
                <span className="text-gray-500 text-sm">{changeLabel}</span>
              )}
            </div>
          )}
        </div>
        {Icon && (
          <div className="p-3 bg-pilot-navy-light rounded-lg">
            <Icon className="w-6 h-6 text-pilot-cyan" />
          </div>
        )}
      </div>
    </Card>
  );
}

export function MetricCardCompact({ title, value, unit = '', className = '' }) {
  return (
    <div className={`text-center ${className}`}>
      <p className="text-xs text-gray-400 uppercase tracking-wide">{title}</p>
      <div className="flex items-baseline justify-center gap-0.5 mt-1">
        <span className="text-xl font-bold text-white">{value}</span>
        {unit && <span className="text-sm text-gray-400">{unit}</span>}
      </div>
    </div>
  );
}
