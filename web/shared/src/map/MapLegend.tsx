import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { cn } from '../lib/utils';
import { Badge } from '../ui/badge';
import { STATUS_CLASSES } from './colors';
import { BADGE_GLYPHS, STATUS_BADGES, badgeGlyphStroke } from './kit/status';
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
 * Colours are always paired with the status name, never conveyed alone; the
 * swatch of a status that carries a corner badge on the map repeats its glyph
 * (lock for unavailable, cross for blocked...), so the legend explains it.
 * The list is named by the visible heading (or by "Legenda" when hidden).
 */
export function MapLegend({ statuses, statusColors, id, title, className }: MapLegendProps) {
  const { t } = useTranslation();
  const headingId = useId();
  const heading = title === undefined ? t('map.legend') : title;
  return (
    <div id={id} data-slot="map-legend" className={cn('space-y-1.5', className)}>
      {heading && (
        <p id={headingId} className="text-xs font-medium text-muted-foreground">
          {heading}
        </p>
      )}
      <ul
        className="flex flex-wrap gap-1.5"
        aria-labelledby={heading ? headingId : undefined}
        aria-label={heading ? undefined : t('map.legend')}
      >
        {statuses.map((s) => {
          const glyph = STATUS_BADGES[s];
          const color = statusColors?.[s];
          return (
            <li key={s} data-legend-status={s}>
              <Badge variant="outline" className="gap-1.5 bg-background/80 font-normal">
                {glyph ? (
                  <svg
                    aria-hidden="true"
                    viewBox="0 0 24 24"
                    data-legend-glyph={glyph}
                    className={cn(
                      'size-4 shrink-0 rounded-full ring-1 ring-foreground/20',
                      !color && STATUS_CLASSES[s].bg,
                      color ? 'text-white' : STATUS_CLASSES[s].fg,
                    )}
                    // Only a caller-provided hex override needs an inline colour.
                    style={color ? { background: color } : undefined}
                  >
                    <path
                      d={BADGE_GLYPHS[glyph]}
                      transform="translate(3.6 3.6) scale(0.7)"
                      fill={glyph === 'star' ? 'currentColor' : 'none'}
                      stroke="currentColor"
                      strokeWidth={badgeGlyphStroke(glyph)}
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    />
                  </svg>
                ) : (
                  <span
                    aria-hidden="true"
                    className={cn(
                      'size-2.5 rounded-full ring-1 ring-foreground/20',
                      !color && STATUS_CLASSES[s].bg,
                    )}
                    // Only a caller-provided hex override needs an inline colour.
                    style={color ? { background: color } : undefined}
                  />
                )}
                {t(`map.status.${s}`)}
              </Badge>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
