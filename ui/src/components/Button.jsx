import { forwardRef } from 'react';

const variants = {
  primary: 'bg-pilot-yellow text-neutral-900 hover:bg-pilot-yellow-light',
  secondary: 'bg-surface-tertiary text-theme-primary hover:bg-surface-elevated border border-theme',
  danger: 'bg-pilot-red text-white hover:bg-red-600',
  ghost: 'bg-transparent text-theme-primary hover:bg-surface-tertiary',
  outline: 'bg-transparent text-pilot-cyan border border-pilot-cyan hover:bg-pilot-cyan hover:text-neutral-900',
};

const sizes = {
  sm: 'px-3 py-1.5 text-sm',
  md: 'px-4 py-2 text-sm',
  lg: 'px-6 py-3 text-base',
};

export const Button = forwardRef(({
  children,
  variant = 'primary',
  size = 'md',
  disabled = false,
  loading = false,
  className = '',
  ...props
}, ref) => {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={`
        inline-flex items-center justify-center gap-2
        font-medium rounded-lg
        transition-all duration-200
        focus:outline-none focus:ring-2 focus:ring-pilot-cyan focus:ring-offset-2 focus:ring-offset-surface-primary
        disabled:opacity-50 disabled:cursor-not-allowed
        ${variants[variant]}
        ${sizes[size]}
        ${className}
      `}
      {...props}
    >
      {loading && (
        <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24">
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
            fill="none"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
      )}
      {children}
    </button>
  );
});

Button.displayName = 'Button';
