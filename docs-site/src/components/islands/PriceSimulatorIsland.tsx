import { useMemo, useState } from 'react';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Separator } from '@arenile/shared/ui/separator';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@arenile/shared/ui/table';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import { StatusBadge, type StatusTone } from '@arenile/shared/ui/patterns/StatusBadge';
import demoPricing from '../../data/demo-pricing.json';
import type { DemoPricingSnapshot, I18nText, PriceStage } from '../../data/types';
import type { DocsLocale } from './i18n';

export interface PriceSimulatorLabels {
  title: string;
  live: string;
  pack: string;
  zone: string;
  period: string;
  date: string;
  duration: string;
  durations: Record<string, string>;
  total: string;
  deposit: string;
  vat: string;
  days: string;
  base: string;
  discounts: string;
  rounding: string;
  notSellable: string;
  stages: Record<PriceStage, string>;
  source: string;
}

export interface PriceSimulatorIslandProps {
  locale: DocsLocale;
  labels: PriceSimulatorLabels;
  /** Initial selection (catalogue codes); defaults to the first of each. */
  pack?: string;
  period?: string;
  duration?: string;
}

/** Real pricing engine output on `seed demo` (src/data/demo-pricing.json, generated). */
const data = demoPricing as unknown as DemoPricingSnapshot;

/** Badge tone of a pricing stage, as in the panel's simulator timeline. */
const STAGE_TONE: Record<PriceStage, StatusTone> = {
  base: 'info',
  modifier: 'highlight',
  discount: 'success',
};

const localize = (t: I18nText, locale: DocsLocale) => t[locale] ?? t.it ?? Object.values(t)[0] ?? '';

/**
 * Price simulator on the demo price list. Nothing is computed here: every
 * figure is a breakdown returned by the real engine (POST /pricing/simulate)
 * for each combination, captured by the docs pipeline; the island only picks
 * the scenario and renders it with the shared components.
 */
export default function PriceSimulatorIsland({ locale, labels, ...initial }: PriceSimulatorIslandProps) {
  const [pack, setPack] = useState(initial.pack ?? data.packs[0]?.code ?? '');
  const [category, setCategory] = useState(data.categories[0]?.code ?? '');
  const [period, setPeriod] = useState(initial.period ?? data.periods[0]?.code ?? '');
  const [duration, setDuration] = useState(initial.duration ?? data.durations[1]?.code ?? data.durations[0]?.code ?? '');

  const scenario = useMemo(
    () =>
      data.scenarios.find(
        (s) => s.pack === pack && s.category === category && s.period === period && s.duration === duration,
      ),
    [pack, category, period, duration],
  );
  const money = (cents: number) => formatCents(cents, data.currency, locale === 'it' ? 'it-IT' : 'en-GB');
  const date = (iso: string) => (locale === 'it' ? formatIsoDate(iso) : iso);
  const vat = scenario?.vat.reduce((s, v) => s + v.tax, 0) ?? 0;

  const choice = (
    label: string,
    value: string,
    onChange: (v: string) => void,
    options: { value: string; label: string }[],
  ) => (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        value={value}
        onValueChange={(v) => v && onChange(v)}
        aria-label={label}
        className="flex flex-wrap justify-start"
      >
        {options.map((o) => (
          <ToggleGroupItem key={o.value} value={o.value} className="px-3">
            {o.label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );

  return (
    <Card className="gap-4">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{labels.title}</CardTitle>
          <Badge variant="secondary">{labels.live}</Badge>
        </div>
        <CardDescription>{labels.source.replace('{{version}}', String(data.engine_version))}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="grid gap-4 sm:grid-cols-2">
          {choice(labels.pack, pack, setPack, data.packs.map((p) => ({ value: p.code, label: localize(p.name, locale) })))}
          {choice(
            labels.zone,
            category,
            setCategory,
            data.categories.map((c) => ({ value: c.code, label: localize(c.zone, locale) })),
          )}
          {choice(
            labels.period,
            period,
            setPeriod,
            data.periods.map((p) => ({ value: p.code, label: localize(p.name, locale) })),
          )}
          {choice(
            labels.duration,
            duration,
            setDuration,
            data.durations.map((d) => ({ value: d.code, label: labels.durations[d.code] ?? d.code })),
          )}
        </div>

        <Separator />

        <div aria-live="polite">
          {!scenario ? (
            <p className="text-sm text-muted-foreground">{labels.notSellable}</p>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap items-end justify-between gap-3">
                <div>
                  <p className="text-sm text-muted-foreground">
                    {labels.total} · {date(scenario.starts_on)}
                    {scenario.ends_on !== scenario.starts_on && ` – ${date(scenario.ends_on)}`}
                  </p>
                  <p className="text-3xl font-semibold tabular-nums">{money(scenario.total)}</p>
                </div>
                <div className="text-right text-sm text-muted-foreground">
                  <p>
                    {labels.vat}: <span className="tabular-nums">{money(vat)}</span>
                  </p>
                  {scenario.deposit_due > 0 && (
                    <p>
                      {labels.deposit}: <span className="tabular-nums">{money(scenario.deposit_due)}</span>
                    </p>
                  )}
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-sm font-medium">{labels.days}</p>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{labels.date}</TableHead>
                      <TableHead className="text-right">{labels.base}</TableHead>
                      <TableHead className="hidden sm:table-cell" />
                      <TableHead className="text-right">{labels.total}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {scenario.days.map((d) => (
                      <TableRow key={`${d.date}-${d.slots}`}>
                        <TableCell className="tabular-nums">
                          {date(d.date)}
                          {d.slots !== 'FULL' && <span className="text-muted-foreground"> · {d.slots}</span>}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">{money(d.base)}</TableCell>
                        <TableCell className="hidden sm:table-cell">
                          <span className="flex flex-wrap gap-1">
                            {d.applied.map((a, i) => (
                              <StatusBadge key={i} tone={STAGE_TONE[a.stage]} dot={false}>
                                {labels.stages[a.stage]}: {localize(a.rule, locale)} ({a.effect})
                              </StatusBadge>
                            ))}
                          </span>
                        </TableCell>
                        <TableCell className="text-right font-medium tabular-nums">{money(d.total)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>

              {(scenario.discounts.length > 0 || scenario.rounding !== 0) && (
                <ul className="space-y-1 text-sm">
                  {scenario.discounts.map((x, i) => (
                    <li key={i} className="flex justify-between">
                      <span>
                        {labels.discounts}: {x.source}
                      </span>
                      <span className="tabular-nums">−{money(x.amount)}</span>
                    </li>
                  ))}
                  {scenario.rounding !== 0 && (
                    <li className="flex justify-between">
                      <span>{labels.rounding}</span>
                      <span className="tabular-nums">{money(scenario.rounding)}</span>
                    </li>
                  )}
                </ul>
              )}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
