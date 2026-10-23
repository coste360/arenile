import { indicatorText } from './indicator';
import type { Schemas } from '@arenile/shared/api';
import { cn } from '@arenile/shared/lib/utils';
import { Badge } from '@arenile/shared/ui/badge';
import { CheckIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { PublicAvailability } from '../../api/public';
import { useBookingEnv } from '../../flow/context';
import { formatMoney, localize } from '../../flow/format';
import { PUBLIC_NS } from '../../i18n';

export interface ZoneCardsProps {
  zones: Schemas['PublicZone'][];
  availability: PublicAvailability;
  showPrices: boolean;
  selectedZoneIds: readonly string[];
  onToggle: (zoneId: string) => void;
}

/**
 * Cards of the `zone` zones: name, "da" price and indicative availability.
 * A card is a toggle button (aria-pressed); the spot inside the zone is
 * assigned automatically at confirmation (spec 7.5).
 */
export function ZoneCards({
  zones,
  availability,
  showPrices,
  selectedZoneIds,
  onToggle,
}: ZoneCardsProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { establishment } = useBookingEnv();
  const byZone = new Map(availability.zones.map((z) => [z.zone_id, z]));
  return (
    <div className="space-y-2">
      <ul className="grid gap-3 sm:grid-cols-2">
        {zones.map((zone) => {
          const a = byZone.get(zone.id);
          const soldOut = !a || a.indicator === 'none' || a.available <= 0;
          const pressed = selectedZoneIds.includes(zone.id);
          const name = localize(zone.name, i18n.language, establishment.default_locale);
          return (
            <li key={zone.id}>
              <button
                type="button"
                aria-pressed={pressed}
                disabled={soldOut}
                onClick={() => onToggle(zone.id)}
                className={cn(
                  'flex min-h-20 w-full flex-col items-start gap-2 rounded-xl border bg-card p-4 text-left shadow-xs transition-colors outline-none',
                  'focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-60',
                  pressed ? 'border-primary ring-2 ring-primary' : 'hover:bg-accent',
                )}
              >
                <span className="flex w-full items-start justify-between gap-2">
                  <span className="font-semibold">{name}</span>
                  {pressed && (
                    <span className="flex size-6 items-center justify-center rounded-full bg-primary text-primary-foreground">
                      <CheckIcon aria-hidden="true" className="size-4" />
                    </span>
                  )}
                </span>
                <span className="flex w-full flex-wrap items-center justify-between gap-2 text-sm">
                  {showPrices && a?.from_price !== undefined ? (
                    <span className="font-medium text-primary-on-surface">
                      {t('where.fromPrice', {
                        price: formatMoney(a.from_price, establishment.currency, i18n.language),
                      })}
                    </span>
                  ) : (
                    <span />
                  )}
                  <Badge
                    variant={soldOut ? 'secondary' : 'outline'}
                    className={cn(
                      a?.indicator === 'last' &&
                        'border-transparent bg-highlight text-highlight-foreground',
                    )}
                  >
                    {a ? indicatorText(t, a) : t('where.soldOut')}
                  </Badge>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      <p className="text-xs text-muted-foreground">{t('where.autoAssign')}</p>
    </div>
  );
}
