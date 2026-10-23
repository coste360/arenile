import { useTranslation } from 'react-i18next';
import { NumberStepper } from '../../components/NumberStepper';
import { PUBLIC_NS } from '../../i18n';

export interface QuantityPickerProps {
  /** Localised name of the zone (e.g. "Lettini in riva"). */
  name: string;
  /** Maximum units available online for the whole interval. */
  max: number;
  value: number;
  onChange: (value: number) => void;
}

/** Quantity selector of a `quantity` zone (spec 12.3 passo 2). */
export function QuantityPicker({ name, max, value, onChange }: QuantityPickerProps) {
  const { t } = useTranslation(PUBLIC_NS);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border bg-card p-4">
      <div className="min-w-0">
        <p className="font-semibold">{name}</p>
        <p className="text-sm text-muted-foreground">
          {max > 0 ? t('where.available', { count: max }) : t('where.soldOut')}
        </p>
      </div>
      <NumberStepper
        value={value}
        max={Math.max(0, max)}
        onChange={onChange}
        label={t('where.units')}
        decreaseLabel={t('where.decrease', { name })}
        increaseLabel={t('where.increase', { name })}
      />
    </div>
  );
}
