import { Spinner } from '@arenile/shared/ui/spinner';
import { useTranslation } from 'react-i18next';
import type { PublicQuote } from '../api/public';
import { formatMoney } from '../flow/format';
import { PUBLIC_NS } from '../i18n';

export interface PriceBarProps {
  quote: PublicQuote | undefined;
  updating: boolean;
  currency: string;
}

/**
 * Compact total next to the step buttons, for phones and embeds where the
 * sticky summary column is not on screen (spec 12.3 "riepilogo sempre
 * visibile"). Works inside a resized iframe, where `position: fixed` would not.
 */
export function PriceBar({ quote, updating, currency }: PriceBarProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  if (!quote && !updating) return null;
  return (
    <div className="flex min-w-0 flex-col" data-testid="price-bar">
      <span className="text-xs text-muted-foreground">
        {quote && quote.due_now !== quote.total ? t('summary.dueNow') : t('summary.total')}
      </span>
      <span
        className="flex items-center gap-1.5 text-lg font-semibold tabular-nums"
        aria-live="polite"
        aria-atomic="true"
      >
        {quote
          ? formatMoney(
              quote.due_now !== quote.total ? quote.due_now : quote.total,
              quote.currency || currency,
              i18n.language,
            )
          : null}
        {updating && (
          <>
            <Spinner className="size-3.5" />
            <span className="sr-only">{t('summary.updating')}</span>
          </>
        )}
      </span>
    </div>
  );
}
