import type { Schemas } from '@arenile/shared/api';
import { useTranslation } from 'react-i18next';
import { NumberStepper } from '../../components/NumberStepper';
import { useBookingEnv } from '../../flow/context';
import { formatMoney, localize } from '../../flow/format';
import { PUBLIC_NS } from '../../i18n';

export interface ExtrasListProps {
  title: string;
  extras: Schemas['PublicExtra'][];
  value: Record<string, number>;
  onChange: (extraId: string, quantity: number) => void;
}

const UNIT_KEY: Record<Schemas['PricingUnit'], string> = {
  per_booking: 'what.perUnit',
  per_day: 'what.perDay',
  per_slot: 'what.perSlot',
};

/** Extras with a quantity stepper and their indicative unit price (the quote is binding). */
export function ExtrasList({ title, extras, value, onChange }: ExtrasListProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { establishment } = useBookingEnv();
  if (extras.length === 0) return null;
  const lang = i18n.language;
  return (
    <fieldset className="space-y-2">
      <legend className="mb-2 text-sm font-medium">{title}</legend>
      <ul className="divide-y rounded-xl border bg-card">
        {extras.map((extra) => {
          const name = localize(extra.name, lang, establishment.default_locale);
          return (
            <li key={extra.id} className="flex flex-wrap items-center justify-between gap-3 p-3">
              <div className="min-w-0">
                <p className="font-medium">{name}</p>
                {extra.unit_price !== undefined && (
                  <p className="text-sm text-muted-foreground">
                    {t(UNIT_KEY[extra.pricing_unit], {
                      price: formatMoney(extra.unit_price, establishment.currency, lang),
                    })}
                  </p>
                )}
              </div>
              <NumberStepper
                value={value[extra.id] ?? 0}
                max={Math.max(1, extra.max_quantity)}
                onChange={(q) => onChange(extra.id, q)}
                label={name}
                decreaseLabel={t('where.decrease', { name })}
                increaseLabel={t('where.increase', { name })}
              />
            </li>
          );
        })}
      </ul>
    </fieldset>
  );
}
