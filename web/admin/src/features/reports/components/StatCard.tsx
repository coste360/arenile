import {
  StatCard as SharedStatCard,
  StatusBadge,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import {
  ArrowDownRightIcon,
  ArrowRightIcon,
  ArrowUpRightIcon,
  type LucideIcon,
} from 'lucide-react';
import type { ReactNode } from 'react';
import type { DeltaTone } from '../lib/delta';

export interface StatDelta {
  /** Change as shown ("+12,5%", "+4,5 p.p."). */
  text: string;
  tone: DeltaTone;
  /** Direction of the raw change, for the arrow. */
  direction: 'up' | 'down' | 'flat';
  /** "vs 2025: 1.234,00 €": the reference value, always as text. */
  reference: string;
}

export interface StatCardProps {
  label: string;
  value: ReactNode;
  icon?: LucideIcon;
  hint?: ReactNode;
  delta?: StatDelta;
}

const TONE: Record<DeltaTone, StatusTone> = {
  positive: 'success',
  negative: 'danger',
  neutral: 'neutral',
};

const ARROW = { up: ArrowUpRightIcon, down: ArrowDownRightIcon, flat: ArrowRightIcon };

/**
 * KPI tile of the reports page: the shared StatCard with the year-over-year
 * delta under the value, a StatusBadge with an arrow and text (colour is
 * never the only signal) followed by the reference value.
 */
export function StatCard({ label, value, icon, hint, delta }: StatCardProps) {
  const Arrow = delta ? ARROW[delta.direction] : null;
  return (
    <SharedStatCard label={label} value={value} icon={icon} hint={hint || undefined}>
      {delta && Arrow && (
        <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <StatusBadge tone={TONE[delta.tone]} dot={false}>
            <Arrow aria-hidden="true" />
            {delta.text}
          </StatusBadge>
          <span className="tabular-nums">{delta.reference}</span>
        </p>
      )}
    </SharedStatCard>
  );
}
