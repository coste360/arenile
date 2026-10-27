import { formatIsoDate } from '@arenile/shared/lib/dates';
import { cn } from '@arenile/shared/lib/utils';
import {
  BeachMap,
  INDICATOR_CLASSES,
  INDICATOR_ICONS,
  MapLegend,
  type MapGeometry,
  type MapLabelMode,
  type SpotIndicator,
  type SpotStatus,
} from '@arenile/shared/map';
import { useTranslation } from 'react-i18next';
import { OPERATIONAL_LEGEND, type OperationsDay, type SlotChoice } from '../lib/status';

/** Paper of the printed day map (spec 30.10): A4 or A3, landscape. */
export type MapPaper = 'a4' | 'a3';

/** Height of the drawing on paper (landscape, header and legend included in the page). */
const PAPER_MAP_HEIGHT: Record<MapPaper, string> = { a4: '150mm', a3: '225mm' };

export interface DayMapPrintData {
  paper: MapPaper;
  geometry: MapGeometry;
  day: OperationsDay;
  date: string;
  slot: SlotChoice;
  statuses: Record<string, SpotStatus>;
  indicators: Record<string, readonly SpotIndicator[]>;
  captions: Record<string, string> | undefined;
  labelMode: MapLabelMode;
  establishmentName?: string;
}

/** The printed sheet of the day map (header, drawing, legend, totals). */
export function DayMapSheet({ data }: { data: DayMapPrintData }) {
  const { t } = useTranslation('operations');
  const totals = data.day.totals;
  const kinds = [...new Set(Object.values(data.indicators).flat())];
  return (
    <article className="space-y-3 text-sm print:text-black" data-paper={data.paper}>
      <header className="flex flex-wrap items-baseline justify-between gap-2 border-b pb-2">
        <h1 className="text-lg font-semibold">
          {t('print.mapTitle', { name: data.geometry.name ?? '' })}
        </h1>
        <p>
          {data.establishmentName ? `${data.establishmentName} · ` : ''}
          {formatIsoDate(data.date)} · {t(`slot.${data.slot}`)}
        </p>
      </header>
      {/* The map's own toolbar (zoom) is not printed. */}
      <div className="[&_[data-testid=beach-map]>div:first-child]:hidden">
        <BeachMap
          geometry={data.geometry}
          statuses={data.statuses}
          spotIndicators={data.indicators}
          spotCaptions={data.captions}
          labelMode={data.labelMode}
          showLegend={false}
          zoomSlider={false}
          title={t('print.mapTitle', { name: data.geometry.name ?? '' })}
          height={PAPER_MAP_HEIGHT[data.paper]}
        />
      </div>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-2">
          <MapLegend statuses={OPERATIONAL_LEGEND} />
          {kinds.length > 0 && (
            <ul className="flex flex-wrap gap-3 text-xs">
              {kinds.map((k) => {
                const Icon = INDICATOR_ICONS[k];
                return (
                  <li key={k} className="flex items-center gap-1.5">
                    <span
                      aria-hidden="true"
                      className={cn(
                        'flex size-4 items-center justify-center rounded-full',
                        INDICATOR_CLASSES[k].bg,
                        INDICATOR_CLASSES[k].fg,
                      )}
                    >
                      <Icon className="size-3" />
                    </span>
                    {t(`indicator.${k}`)}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
        <dl
          className="grid grid-cols-2 gap-x-4 gap-y-1 tabular-nums"
          aria-label={t('print.totals')}
        >
          <dt>{t('print.occupied')}</dt>
          <dd className="text-right font-medium">{totals.occupied}</dd>
          <dt>{t('print.free')}</dt>
          <dd className="text-right font-medium">{totals.free}</dd>
          <dt>{t('print.arrivals')}</dt>
          <dd className="text-right font-medium">{totals.arrivals}</dd>
          <dt>{t('print.departures')}</dt>
          <dd className="text-right font-medium">{totals.departures}</dd>
        </dl>
      </div>
    </article>
  );
}
