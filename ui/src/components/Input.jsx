import { forwardRef } from 'react';
import { Search } from 'lucide-react';

export const Input = forwardRef(({
  label,
  error,
  className = '',
  ...props
}, ref) => {
  return (
    <div className="w-full">
      {label && (
        <label className="block text-sm font-medium text-gray-300 mb-1.5">
          {label}
        </label>
      )}
      <input
        ref={ref}
        className={`
          w-full px-3 py-2
          bg-pilot-navy-dark border border-pilot-navy-light rounded-lg
          text-white placeholder-gray-500
          focus:outline-none focus:ring-2 focus:ring-pilot-yellow focus:border-transparent
          disabled:opacity-50 disabled:cursor-not-allowed
          ${error ? 'border-pilot-red focus:ring-pilot-red' : ''}
          ${className}
        `}
        {...props}
      />
      {error && (
        <p className="mt-1.5 text-sm text-pilot-red">{error}</p>
      )}
    </div>
  );
});

Input.displayName = 'Input';

export function SearchInput({ value, onChange, placeholder = 'Search...', className = '' }) {
  return (
    <div className={`relative ${className}`}>
      <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-500" />
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="
          w-full pl-10 pr-4 py-2
          bg-pilot-navy-dark border border-pilot-navy-light rounded-lg
          text-white placeholder-gray-500
          focus:outline-none focus:ring-2 focus:ring-pilot-yellow focus:border-transparent
        "
      />
    </div>
  );
}

export function Select({ label, options, value, onChange, className = '', ...props }) {
  return (
    <div className={`w-full ${className}`}>
      {label && (
        <label className="block text-sm font-medium text-gray-300 mb-1.5">
          {label}
        </label>
      )}
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="
          w-full px-3 py-2
          bg-pilot-navy-dark border border-pilot-navy-light rounded-lg
          text-white
          focus:outline-none focus:ring-2 focus:ring-pilot-yellow focus:border-transparent
        "
        {...props}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </div>
  );
}
