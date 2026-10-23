import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { Card } from '@arenile/shared/ui/card';
import { cn } from '@arenile/shared/lib/utils';

export type StatTone = 'default' | 'danger' | 'success';

const TONE: Record<StatTone, string> = {
  default: 'text-foreground',
  danger: 'text-destructive',
  success: 'text-success',
};

export interface StatCardProps {
  label: ReactNode;
  value: ReactNode;
  /** Line under the value (e.g. "0 partenze"). */
  hint?: ReactNode;
  icon?: LucideIcon;
  /** Colour of the value (danger: an amount still due). */
  tone?: StatTone;
  /** Extra content under the value (e.g. a Progress). */
  children?: ReactNode;
  className?: string;
}

/**
 * KPI tile: label, big tabular value, optional hint and icon. Lay them out in
 * a `grid grid-cols-2 gap-4 lg:grid-cols-4` (see README "KPI").
 */
export function StatCard({
  label,
  value,
  hint,
  icon: Icon,
  tone = 'default',
  children,
  className,
}: StatCardProps) {
  return (
    <Card className={cn('min-w-0 gap-2 px-4 py-4 sm:px-5', className)}>
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm text-muted-foreground">{label}</p>
        {Icon && <Icon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />}
      </div>
      <p
        className={cn(
          'text-lg font-semibold tracking-tight break-words tabular-nums sm:text-2xl',
          TONE[tone],
        )}
      >
        {value}
      </p>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </Card>
  );
}
