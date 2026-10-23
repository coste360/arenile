import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { StatusBadge, type StatusTone } from '@arenile/shared/ui/patterns';
import { ArrowDownRightIcon, ArrowRightIcon, ArrowUpRightIcon } from 'lucide-react';
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
  title: string;
  value: ReactNode;
  icon?: ReactNode;
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
 * KPI card of the reports page. Same anatomy as the "Oggi" KPI cards (Card,
 * description, tabular value, icon action) so the panel looks alike; the
 * delta is a StatusBadge with an arrow and text (colour is never the only
 * signal). INTEGRATOR: replace with the shared `StatCard` pattern when the
 * UI consistency branch lands.
 */
export function StatCard({ title, value, icon, hint, delta }: StatCardProps) {
  const Arrow = delta ? ARROW[delta.direction] : null;
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-2xl tabular-nums">{value}</CardTitle>
        {icon && <CardAction className="text-muted-foreground [&_svg]:size-5">{icon}</CardAction>}
      </CardHeader>
      {(hint || delta) && (
        <CardContent className="flex flex-wrap items-center gap-2 px-4 text-sm text-muted-foreground">
          {delta && Arrow && (
            <StatusBadge tone={TONE[delta.tone]} dot={false} className="tabular-nums">
              <Arrow aria-hidden="true" className="size-3" />
              {delta.text}
            </StatusBadge>
          )}
          {delta && <span className="tabular-nums">{delta.reference}</span>}
          {hint && <span>{hint}</span>}
        </CardContent>
      )}
    </Card>
  );
}
