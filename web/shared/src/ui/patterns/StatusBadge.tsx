import type { ReactNode } from 'react';
import { Badge } from '@arenile/shared/ui/badge';
import { cn } from '@arenile/shared/lib/utils';
import type { SpotStatus } from '../../map/types';
import { STATUS_CLASSES } from '../../map/colors';

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger' | 'highlight';

const TONES: Record<StatusTone, { badge: string; dot: string }> = {
  neutral: { badge: 'bg-muted text-muted-foreground', dot: 'bg-muted-foreground' },
  info: { badge: 'bg-primary/10 text-primary-on-surface', dot: 'bg-primary' },
  success: { badge: 'bg-success/15 text-foreground', dot: 'bg-success' },
  warning: { badge: 'bg-warning/20 text-foreground', dot: 'bg-warning' },
  danger: { badge: 'bg-destructive/10 text-destructive', dot: 'bg-destructive' },
  highlight: { badge: 'bg-highlight text-highlight-foreground', dot: 'bg-highlight-foreground' },
};

export interface StatusBadgeProps {
  /** Semantic colour; ignored when `spotStatus` is set. */
  tone?: StatusTone;
  /** Beach-map status: uses the same colour as the map (--status-*). */
  spotStatus?: SpotStatus;
  /** Show a leading dot (default true). */
  dot?: boolean;
  children: ReactNode;
  className?: string;
}

/**
 * Pill with a coloured dot and a text label (colour is never the only
 * signal). Use for booking/payment states, online/offline, "presto"...
 */
export function StatusBadge({
  tone = 'neutral',
  spotStatus,
  dot = true,
  children,
  className,
}: StatusBadgeProps) {
  const dotClass = spotStatus ? STATUS_CLASSES[spotStatus].bg : TONES[tone].dot;
  return (
    <Badge
      variant="secondary"
      className={cn(
        'gap-1.5 font-medium',
        spotStatus ? 'bg-muted text-foreground' : TONES[tone].badge,
        className,
      )}
    >
      {dot && <span aria-hidden="true" className={cn('size-1.5 rounded-full', dotClass)} />}
      {children}
    </Badge>
  );
}
