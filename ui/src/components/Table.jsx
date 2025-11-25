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
    <thead className="bg-pilot-navy-dark border-b border-pilot-navy-light">
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
        ${onClick ? 'hover:bg-pilot-navy-light cursor-pointer' : ''}
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
        px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider
        ${className}
      `}
    >
      {children}
    </th>
  );
}

export function TableCell({ children, className = '' }) {
  return (
    <td className={`px-4 py-3 text-sm text-white ${className}`}>
      {children}
    </td>
  );
}
