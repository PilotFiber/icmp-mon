export function Table({ children, className = '' }) {
  return (
    <div className={`overflow-x-auto ${className}`}>
      <table className="w-full">
        {children}
      </table>
    </div>
  );
}

export function TableHeader({ children }) {
  return (
    <thead className="bg-surface-primary border-b border-theme">
      {children}
    </thead>
  );
}

export function TableBody({ children }) {
  return (
    <tbody className="divide-y divide-pilot-navy-light">
      {children}
    </tbody>
  );
}

export function TableRow({ children, className = '', onClick = null }) {
  return (
    <tr
      className={`
        ${onClick ? 'hover:bg-surface-tertiary cursor-pointer' : ''}
        transition-colors
        ${className}
      `}
      onClick={onClick}
    >
      {children}
    </tr>
  );
}

export function TableHead({ children, className = '' }) {
  return (
    <th
      className={`
        px-4 py-3 text-left text-xs font-medium text-theme-muted uppercase tracking-wider
        ${className}
      `}
    >
      {children}
    </th>
  );
}

export function TableCell({ children, className = '' }) {
  return (
    <td className={`px-4 py-3 text-sm text-theme-primary ${className}`}>
      {children}
    </td>
  );
}
