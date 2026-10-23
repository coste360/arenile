import type { Schemas } from '@arenile/shared/api';
import { ExtraList, ExtraRow, useQuantityMaxText } from '@arenile/shared/ui/patterns/ExtraRow';
import { QuantityStepper } from '@arenile/shared/ui/patterns/QuantityStepper';
import { useTranslation } from 'react-i18next';
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

/**
 * Extras with the shared quantity stepper, their indicative unit price (the
 * quote is binding) and the catalog maximum, which the stepper cannot exceed.
 */
export function ExtrasList({ title, extras, value, onChange }: ExtrasListProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { establishment } = useBookingEnv();
  const maxText = useQuantityMaxText();
  if (extras.length === 0) return null;
  const lang = i18n.language;
  return (
    <ExtraList title={title}>
      {extras.map((extra) => {
        const name = localize(extra.name, lang, establishment.default_locale);
        const max = Math.max(1, extra.max_quantity);
        return (
          <ExtraRow
            key={extra.id}
            name={name}
            detail={
              extra.unit_price !== undefined
                ? t(UNIT_KEY[extra.pricing_unit], {
                    price: formatMoney(extra.unit_price, establishment.currency, lang),
                  })
                : undefined
            }
            hint={maxText(max)}
          >
            <QuantityStepper
              size="lg"
              value={value[extra.id] ?? 0}
              max={max}
              onChange={(q) => onChange(extra.id, q)}
              label={name}
              decreaseLabel={t('where.decrease', { name })}
              increaseLabel={t('where.increase', { name })}
            />
          </ExtraRow>
        );
      })}
    </ExtraList>
  );
}
