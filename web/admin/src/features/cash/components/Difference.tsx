import { StatusBadge } from '@arenile/shared/ui/patterns';
import { cn } from '@arenile/shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { differenceTone } from '../lib/totals';

const TONE = { balanced: 'success', surplus: 'warning', shortfall: 'danger' } as const;

/** Signed difference (counted - expected): colour plus a text label, never colour alone. */
export function DifferenceAmount({
  cents,
  money,
  className,
  badge = true,
}: {
  cents: number;
  money: (cents: number) => string;
  className?: string;
  badge?: boolean;
}) {
  const { t } = useTranslation('cash');
  const tone = differenceTone(cents);
  const sign = cents > 0 ? '+' : '';
  return (
    <span className={cn('inline-flex flex-wrap items-center justify-end gap-2', className)}>
      <span
        data-testid="difference-amount"
        className={cn(
          'font-semibold tabular-nums',
          tone === 'shortfall' && 'text-destructive',
          tone === 'balanced' && 'text-muted-foreground',
        )}
      >
        {sign}
        {money(cents)}
      </span>
      {badge && <StatusBadge tone={TONE[tone]}>{t(`close.${tone}`)}</StatusBadge>}
    </span>
  );
}
