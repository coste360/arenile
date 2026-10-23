import { STATUS_CLASSES, type SpotStatus } from '@arenile/shared/map';
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@arenile/shared/ui/chart';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Bar, BarChart, Cell, XAxis, YAxis } from 'recharts';
import { chartRows } from '../lib/kpis';

export interface StatusChartProps {
  byStatus: Partial<Record<SpotStatus, number>>;
}

/**
 * Spots per status of the day as horizontal bars in the map colours
 * (--status-* tokens, same as the legend), with the counts also listed
 * as text for screen readers.
 */
export function StatusChart({ byStatus }: StatusChartProps) {
  const { t } = useTranslation('operations');
  const { t: tc } = useTranslation();
  const rows = useMemo(
    () => chartRows(byStatus).map((r) => ({ ...r, label: tc(`map.status.${r.status}`) })),
    [byStatus, tc],
  );
  const config = useMemo(
    () =>
      Object.fromEntries(
        rows.map((r) => [
          r.status,
          { label: r.label, color: `var(${STATUS_CLASSES[r.status].cssVar})` },
        ]),
      ) satisfies ChartConfig,
    [rows],
  );
  if (rows.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('today.chartEmpty')}</p>;
  }
  return (
    <figure className="space-y-2">
      <ChartContainer
        config={{ ...config, count: { label: t('today.spots') } }}
        className="aspect-auto h-64 w-full"
        aria-hidden="true"
      >
        <BarChart data={rows} layout="vertical" margin={{ left: 8, right: 16 }}>
          <XAxis type="number" allowDecimals={false} hide />
          <YAxis type="category" dataKey="label" width={140} tickLine={false} axisLine={false} />
          <ChartTooltip cursor={false} content={<ChartTooltipContent hideLabel={false} />} />
          <Bar dataKey="count" radius={4}>
            {rows.map((r) => (
              <Cell key={r.status} fill={`var(${STATUS_CLASSES[r.status].cssVar})`} />
            ))}
          </Bar>
        </BarChart>
      </ChartContainer>
      <figcaption className="sr-only">
        <ul>
          {rows.map((r) => (
            <li key={r.status}>
              {r.label}: {r.count}
            </li>
          ))}
        </ul>
      </figcaption>
    </figure>
  );
}
