export function Card({
  children,
  className = '',
  accent = null,
  hover = false,
  ...props
}) {
  const accentColors = {
    yellow: 'border-l-4 border-l-pilot-yellow',
    cyan: 'border-l-4 border-l-pilot-cyan',
    red: 'border-l-4 border-l-pilot-red',
    green: 'border-l-4 border-l-status-healthy',
    warning: 'border-l-4 border-l-warning',
  };

  return (
    <div
      className={`
        bg-surface-secondary rounded-lg p-6
        border border-theme
        ${accent ? accentColors[accent] : ''}
        ${hover ? 'hover:border-pilot-cyan transition-colors cursor-pointer' : ''}
        ${className}
      `}
      {...props}
    >
      {children}
    </div>
  );
}

export function CardHeader({ children, className = '' }) {
  return (
    <div className={`mb-4 ${className}`}>
      {children}
    </div>
  );
}

export function CardTitle({ children, className = '' }) {
  return (
    <h3 className={`text-lg font-semibold text-theme-primary ${className}`}>
      {children}
    </h3>
  );
}

export function CardDescription({ children, className = '' }) {
  return (
    <p className={`text-sm text-theme-muted mt-1 ${className}`}>
      {children}
    </p>
  );
}

export function CardContent({ children, className = '' }) {
  return (
    <div className={className}>
      {children}
    </div>
  );
}

export function CardFooter({ children, className = '' }) {
  return (
    <div className={`mt-4 pt-4 border-t border-theme ${className}`}>
      {children}
    </div>
  );
}
