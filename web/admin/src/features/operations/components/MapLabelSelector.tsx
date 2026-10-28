import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { useTranslation } from 'react-i18next';
import { MAP_LABEL_PARTS, type MapLabelPart } from '../lib/mapLabels';

export interface MapLabelSelectorProps {
  value: readonly MapLabelPart[];
  onChange: (parts: MapLabelPart[]) => void;
  /** Without bookings.read the "Cliente" part is not offered. */
  canSeeCustomers: boolean;
}

/**
 * "Etichette: Codice / Cliente / Fine / Allestimento" (spec 30.4): parts
 * combinable, at least one kept (the last one cannot be switched off).
 * Toggle buttons wrap at phone width; each is a real button with
 * aria-pressed, so keyboard and screen readers work as for any toggle.
 */
export function MapLabelSelector({ value, onChange, canSeeCustomers }: MapLabelSelectorProps) {
  const { t } = useTranslation('operations');
  const parts = MAP_LABEL_PARTS.filter((p) => p !== 'customer' || canSeeCustomers);
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <span className="text-xs font-medium text-muted-foreground" aria-hidden="true">
        {t('labels.label')}
      </span>
      <ToggleGroup
        type="multiple"
        variant="outline"
        size="sm"
        aria-label={t('labels.label')}
        value={value.filter((p) => parts.includes(p))}
        onValueChange={(v) => {
          if (v.length === 0) return;
          onChange(v as MapLabelPart[]);
        }}
        className="flex-wrap"
      >
        {parts.map((p) => (
          <ToggleGroupItem key={p} value={p} aria-label={t(`labels.${p}`)}>
            {t(`labels.${p}`)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
