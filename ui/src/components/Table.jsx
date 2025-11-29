export function Table({ children, className = '' }) {
  return (
    <div className={`overflow-x-auto -mx-4 md:mx-0 ${className}`}>
      <table className="w-full min-w-[600px] md:min-w-0">
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

export function TableHead({ children, className = '', hideOnMobile = false }) {
  return (
    <th
      className={`
        px-3 md:px-4 py-2.5 md:py-3 text-left text-xs font-medium text-theme-muted uppercase tracking-wider
        ${hideOnMobile ? 'hidden md:table-cell' : ''}
        ${className}
      `}
    >
      {children}
    </th>
  );
}

export function TableCell({ children, className = '', hideOnMobile = false }) {
  return (
    <td className={`
      px-3 md:px-4 py-2.5 md:py-3 text-sm text-theme-primary
      ${hideOnMobile ? 'hidden md:table-cell' : ''}
      ${className}
    `}>
      {children}
    </td>
  );
}

// Mobile-friendly card list alternative to tables
export function MobileCardList({ children, className = '' }) {
  return (
    <div className={`md:hidden space-y-3 ${className}`}>
      {children}
    </div>
  );
}

export function MobileCard({ children, onClick, className = '' }) {
  return (
    <div
      className={`
        bg-surface-secondary rounded-lg p-3 space-y-2
        ${onClick ? 'cursor-pointer active:bg-surface-tertiary' : ''}
        ${className}
      `}
      onClick={onClick}
    >
      {children}
    </div>
  );
}

export function MobileCardRow({ label, children, className = '' }) {
  return (
    <div className={`flex justify-between items-center text-sm ${className}`}>
      <span className="text-theme-muted">{label}</span>
      <span className="text-theme-primary text-right">{children}</span>
    </div>
  );
}

// Wrapper that shows table on desktop, cards on mobile
export function ResponsiveTable({ children, mobileContent, className = '' }) {
  return (
    <>
      <div className={`hidden md:block ${className}`}>
        {children}
      </div>
      {mobileContent && (
        <div className="md:hidden">
          {mobileContent}
        </div>
      )}
    </>
  );
}
