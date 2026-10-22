import { useTranslation } from 'react-i18next';
import { cn } from '../lib/utils';
import { Badge } from '../ui/badge';
import { STATUS_CLASSES } from './colors';
import type { SpotStatus } from './types';

export interface MapLegendProps {
  statuses: readonly SpotStatus[];
  /** Hex overrides (e.g. from the panel theme); default: the --status-* theme tokens. */
  statusColors?: Partial<Record<SpotStatus, string>>;
  /** Referenced by the map's aria-describedby. */
  id?: string;
  /** Visible heading (default "Legenda"); `null` hides it. */
  title?: string | null;
  className?: string;
}

/**
 * Status legend of the map as a list of outline Badges with a colour swatch.
 * Colours are always paired with the status name, never conveyed alone.
 */
export function MapLegend({ statuses, statusColors, id, title, className }: MapLegendProps) {
  const { t } = useTranslation();
  const heading = title === undefined ? t('map.legend') : title;
  return (
    <div id={id} data-slot="map-legend" className={cn('space-y-1.5', className)}>
      {heading && <p className="text-xs font-medium text-muted-foreground">{heading}</p>}
      <ul className="flex flex-wrap gap-1.5">
        {statuses.map((s) => (
          <li key={s} data-legend-status={s}>
            <Badge variant="outline" className="gap-1.5 bg-background/80 font-normal">
              <span
                aria-hidden="true"
                className={cn(
                  'size-2.5 rounded-full ring-1 ring-foreground/20',
                  !statusColors?.[s] && STATUS_CLASSES[s].bg,
                )}
                // Only a caller-provided hex override needs an inline colour.
                style={statusColors?.[s] ? { background: statusColors[s] } : undefined}
              />
              {t(`map.status.${s}`)}
            </Badge>
          </li>
        ))}
      </ul>
    </div>
  );
}
