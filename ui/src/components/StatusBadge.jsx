const statusConfig = {
  healthy: {
    bg: 'bg-status-healthy/20',
    text: 'text-status-healthy',
    dot: 'bg-status-healthy',
    label: 'Healthy',
  },
  degraded: {
    bg: 'bg-status-degraded/20',
    text: 'text-status-degraded',
    dot: 'bg-status-degraded',
    label: 'Degraded',
  },
  down: {
    bg: 'bg-status-down/20',
    text: 'text-status-down',
    dot: 'bg-status-down',
    label: 'Down',
  },
  unknown: {
    bg: 'bg-status-unknown/20',
    text: 'text-status-unknown',
    dot: 'bg-status-unknown',
    label: 'Unknown',
  },
  pending: {
    bg: 'bg-pilot-cyan/20',
    text: 'text-pilot-cyan',
    dot: 'bg-pilot-cyan',
    label: 'Pending',
  },
};

export function StatusBadge({
  status,
  label = null,
  pulse = false,
  size = 'md',
  className = ''
}) {
  const config = statusConfig[status] || statusConfig.unknown;
  const displayLabel = label || config.label;

  const sizes = {
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-1 text-sm',
    lg: 'px-3 py-1.5 text-sm',
  };

  const dotSizes = {
    sm: 'w-1.5 h-1.5',
    md: 'w-2 h-2',
    lg: 'w-2.5 h-2.5',
  };

  return (
    <span
      className={`
        inline-flex items-center gap-1.5 rounded-full font-medium
        ${config.bg} ${config.text}
        ${sizes[size]}
        ${className}
      `}
    >
      <span
        className={`
          ${dotSizes[size]} rounded-full ${config.dot}
          ${pulse ? 'animate-pulse-status' : ''}
        `}
      />
      {displayLabel}
    </span>
  );
}

export function StatusDot({ status, pulse = false, className = '' }) {
  const config = statusConfig[status] || statusConfig.unknown;

  return (
    <span
      className={`
        inline-block w-2.5 h-2.5 rounded-full
        ${config.dot}
        ${pulse ? 'animate-pulse-status' : ''}
        ${className}
      `}
    />
  );
}
