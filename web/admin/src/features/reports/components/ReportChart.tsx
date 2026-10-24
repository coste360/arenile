import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@arenile/shared/ui/chart';
import { cn } from '@arenile/shared/lib/utils';
import { useMemo } from 'react';
import { Bar, BarChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from 'recharts';

export type ChartTone = 1 | 2 | 3 | 4 | 5;

/** Static class names so Tailwind generates them (tooltip swatches). */
const SWATCH: Record<ChartTone, string> = {
  1: 'bg-chart-1',
  2: 'bg-chart-2',
  3: 'bg-chart-3',
  4: 'bg-chart-4',
  5: 'bg-chart-5',
};

export interface ChartSeries {
  /** Data key of the value in each datum. */
  key: string;
  label: string;
  /** Theme chart colour token (--chart-1 … --chart-5). */
  tone: ChartTone;
  /** Dashed line: the reference series (previous year). */
  dashed?: boolean;
}

export interface ReportChartProps {
  data: readonly Record<string, string | number>[];
  /** Data key of the category (x axis) label. */
  categoryKey: string;
  series: readonly ChartSeries[];
  /** Bars for categories, lines for time series. */
  kind: 'bar' | 'line';
  /** Formats a value for the axis and the tooltip (money, percentage, count). */
  format: (value: number) => string;
  /** Visually hidden caption: tells screen readers where the same data is as a table. */
  caption: string;
}

/**
 * Chart of a report on the shadcn chart component (Recharts, theme tokens
 * --chart-n, dark mode included). Decorative for assistive technology: the
 * section's DataTable carries the same numbers, the caption says so. One
 * y axis only; a legend appears with two or more series; the reference
 * series is dashed so it is not told apart by colour alone.
 */
export function ReportChart({
  data,
  categoryKey,
  series,
  kind,
  format,
  caption,
}: ReportChartProps) {
  const config = useMemo(
    () =>
      Object.fromEntries(
        series.map((s) => [s.key, { label: s.label, color: `var(--chart-${s.tone})` }]),
      ) satisfies ChartConfig,
    [series],
  );
  const rows = data as Record<string, string | number>[];
  const axes = (
    <>
      <CartesianGrid vertical={false} />
      <XAxis
        dataKey={categoryKey}
        tickLine={false}
        axisLine={false}
        tickMargin={8}
        minTickGap={16}
      />
      <YAxis
        tickLine={false}
        axisLine={false}
        width={72}
        tickFormatter={(v: number) => format(v)}
      />
      <ChartTooltip
        content={
          <ChartTooltipContent
            formatter={(value, name) => {
              const s = series.find((x) => x.key === name);
              return (
                <span className="flex w-full items-center gap-2">
                  <span
                    aria-hidden="true"
                    className={cn('size-2.5 shrink-0 rounded-sm', s ? SWATCH[s.tone] : 'bg-muted')}
                  />
                  <span className="flex-1 text-muted-foreground">{s?.label ?? String(name)}</span>
                  <span className="font-medium text-foreground tabular-nums">
                    {format(Number(value))}
                  </span>
                </span>
              );
            }}
          />
        }
      />
      {series.length > 1 && <ChartLegend content={<ChartLegendContent />} />}
    </>
  );
  return (
    <figure className="space-y-2">
      <ChartContainer config={config} className="aspect-auto h-64 w-full" aria-hidden="true">
        {kind === 'bar' ? (
          <BarChart accessibilityLayer={false} data={rows} margin={{ left: 0, right: 8 }}>
            {axes}
            {series.map((s) => (
              <Bar
                key={s.key}
                dataKey={s.key}
                fill={`var(--color-${s.key})`}
                radius={[4, 4, 0, 0]}
              />
            ))}
          </BarChart>
        ) : (
          <LineChart accessibilityLayer={false} data={rows} margin={{ left: 0, right: 8 }}>
            {axes}
            {series.map((s) => (
              <Line
                key={s.key}
                dataKey={s.key}
                type="monotone"
                stroke={`var(--color-${s.key})`}
                strokeWidth={2}
                strokeDasharray={s.dashed ? '6 4' : undefined}
                dot={false}
              />
            ))}
          </LineChart>
        )}
      </ChartContainer>
      <figcaption className="sr-only">{caption}</figcaption>
    </figure>
  );
}
