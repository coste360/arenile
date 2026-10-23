import type { Schemas } from '@arenile/shared/api';
import { cn } from '@arenile/shared/lib/utils';
import { Badge } from '@arenile/shared/ui/badge';
import { CheckIcon } from 'lucide-react';
import { useRef, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useBookingEnv } from '../../flow/context';
import { formatMoney, localize } from '../../flow/format';
import { PUBLIC_NS } from '../../i18n';
import { componentIcon } from './icons';

export interface PackChoiceProps {
  labelledBy: string;
  packs: Schemas['PublicPack'][];
  value: string | null;
  onChange: (packId: string) => void;
}

/**
 * Packs of one line as radio cards (one tab stop, arrows move): name,
 * composition with icons and quantities, "da" price, approval badge.
 */
export function PackChoice({ labelledBy, packs, value, onChange }: PackChoiceProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { establishment } = useBookingEnv();
  const ref = useRef<HTMLDivElement>(null);
  const lang = i18n.language;
  const fallback = establishment.default_locale;
  const current = packs.find((p) => p.id === value) ?? packs[0];

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const step =
      e.key === 'ArrowDown' || e.key === 'ArrowRight'
        ? 1
        : e.key === 'ArrowUp' || e.key === 'ArrowLeft'
          ? -1
          : 0;
    if (!step || packs.length === 0) return;
    e.preventDefault();
    const at = packs.findIndex((p) => p.id === current?.id);
    const next = packs[(at + step + packs.length) % packs.length];
    if (!next) return;
    onChange(next.id);
    ref.current?.querySelector<HTMLElement>(`[data-pack="${next.id}"]`)?.focus();
  };

  return (
    <div
      ref={ref}
      role="radiogroup"
      aria-labelledby={labelledBy}
      onKeyDown={onKeyDown}
      className="grid gap-3 sm:grid-cols-2"
    >
      {packs.map((pack) => {
        const checked = pack.id === value;
        const description = localize(pack.description, lang, fallback);
        return (
          <button
            key={pack.id}
            type="button"
            role="radio"
            aria-checked={checked}
            data-pack={pack.id}
            tabIndex={pack.id === current?.id ? 0 : -1}
            onClick={() => onChange(pack.id)}
            className={cn(
              'flex min-h-11 w-full flex-col gap-2 rounded-xl border bg-card p-4 text-left shadow-xs outline-none transition-colors',
              'focus-visible:ring-3 focus-visible:ring-ring/50',
              checked ? 'border-primary ring-2 ring-primary' : 'hover:bg-accent',
            )}
          >
            <span className="flex w-full items-start justify-between gap-2">
              <span className="font-semibold">{localize(pack.name, lang, fallback)}</span>
              <span
                aria-hidden="true"
                className={cn(
                  'flex size-5 shrink-0 items-center justify-center rounded-full border',
                  checked && 'border-primary bg-primary text-primary-foreground',
                )}
              >
                {checked && <CheckIcon className="size-3.5" />}
              </span>
            </span>
            {description && <span className="text-sm text-muted-foreground">{description}</span>}
            {pack.components.length > 0 && (
              <span
                className="flex flex-wrap gap-x-3 gap-y-1 text-sm"
                aria-label={t('what.composition')}
              >
                {pack.components.map((c) => {
                  const Icon = componentIcon(c.icon, c.code);
                  return (
                    <span key={c.code} className="flex items-center gap-1">
                      <Icon aria-hidden="true" className="size-4 text-muted-foreground" />
                      {c.quantity > 1 ? `${c.quantity} × ` : ''}
                      {localize(c.name, lang, fallback)}
                    </span>
                  );
                })}
              </span>
            )}
            <span className="flex w-full flex-wrap items-center justify-between gap-2">
              {pack.from_price !== undefined && (
                <span className="text-sm font-medium text-primary-on-surface">
                  {t('where.fromPrice', {
                    price: formatMoney(pack.from_price, establishment.currency, lang),
                  })}
                </span>
              )}
              {pack.requires_approval && (
                <Badge variant="secondary">{t('what.requiresApproval')}</Badge>
              )}
            </span>
          </button>
        );
      })}
    </div>
  );
}
