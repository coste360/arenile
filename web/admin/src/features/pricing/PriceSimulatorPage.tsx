import { ApiProblem } from '@arenile/shared/api';
import { formatIsoDate, isoToDate } from '@arenile/shared/lib/dates';
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Badge,
  Button,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Separator,
  Spinner,
  type ChartConfig,
} from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  DatePicker,
  EmptyState,
  type Option,
  OptionSelect,
  QueryState,
  SectionCard,
  StatCard,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  CalculatorIcon,
  CalendarXIcon,
  PackageXIcon,
  ReceiptTextIcon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useForm, type Control } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Bar, BarChart, CartesianGrid, XAxis } from 'recharts';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { formatCents } from '@arenile/shared/lib/money';
import { agreements, priceRules, promoCodes, useSimulate, type SimulateResult } from './api';
import { FormErrorAlert, PricingLayout, SeasonSelect, TextRow } from './kit';
import { formatEffect, STAGE_TONE } from './ruleNames';
import { useSelectedSeason } from './season';
import { CHANNELS, FORMULAS } from './rules.schema';
import {
  simulatorDefaults,
  simulatorSchema,
  toSimulateRequest,
  type SimulatorValues,
} from './simulator.schema';
import { usePricingData } from './usePricingData';

/** Simulatore prezzi (spec 8.7): any input, step-by-step breakdown, effective policy and deposit. */
export function PriceSimulatorPage() {
  const { t } = useTranslation('pricing');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const ready = !sel.isLoading && !data.isLoading && sel.season;
  return (
    <PricingLayout
      title={t('simulator.title')}
      description={t('simulator.description')}
      readOnly={!ctx.isLoading && !ctx.canConfigure}
      establishmentId={ctx.establishmentId}
      seasonId={sel.season?.id}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      <QueryState
        isLoading={sel.isLoading || data.isLoading}
        error={sel.error ?? data.error}
        onRetry={sel.refetch}
      />
      {!sel.isLoading && sel.seasons.length === 0 && (
        <EmptyState icon={CalendarXIcon} title={t('noSeason')} />
      )}
      {ready && sel.season && (
        <Simulator
          key={sel.season.id}
          establishmentId={ctx.establishmentId}
          seasonId={sel.season.id}
          start={sel.season.opens_on}
          packs={data.packs
            .filter((p) => p.active)
            .map((p) => ({ value: p.id, label: data.name(p.name) || p.code }))}
          zones={data.rawZones.map((z) => ({ value: z.id, label: data.name(z.name) || z.code }))}
          extras={data.extras
            .filter((x) => x.active)
            .map((x) => ({ value: x.id, label: data.name(x.name) || x.code }))}
          locale={ctx.uiLocale}
          disabled={!ctx.canConfigure}
          names={{
            // The engine reports packs, zones and periods by code; show their names.
            pack: (code) => {
              const p = data.packs.find((x) => x.code === code);
              return (p && data.name(p.name)) || code;
            },
            zone: (code) => {
              const z = data.rawZones.find((x) => x.code === code);
              return (z && data.name(z.name)) || code;
            },
            period: (code) => {
              const rp = sel.periods.find((x) => x.code === code);
              return (rp && data.name(rp.name)) || code;
            },
          }}
        />
      )}
    </PricingLayout>
  );
}

function SelectRow({
  control,
  name,
  label,
  options,
}: {
  control: Control<SimulatorValues>;
  name: keyof SimulatorValues;
  label: string;
  options: Option[];
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <OptionSelect value={field.value} onChange={field.onChange} options={options} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function DateRow({
  control,
  name,
  label,
  clearable,
}: {
  control: Control<SimulatorValues>;
  name: 'starts_on' | 'ends_on' | 'today';
  label: string;
  clearable?: boolean;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <DatePicker
              value={field.value}
              onChange={(v) => field.onChange(v ?? '')}
              clearable={clearable}
              className="sm:[&>button]:w-full"
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

interface CodeNames {
  pack: (code: string) => string;
  zone: (code: string) => string;
  period: (code: string) => string;
}

function Simulator(p: {
  establishmentId: string;
  seasonId: string;
  start: string;
  packs: Option[];
  zones: Option[];
  extras: Option[];
  locale: string;
  disabled: boolean;
  names: CodeNames;
}) {
  const { t } = useTranslation('pricing');
  const simulate = useSimulate(p.establishmentId);
  const promos = promoCodes.useList(p.establishmentId);
  const agreementList = agreements.useList(p.establishmentId);
  const rules = priceRules.useList(p.establishmentId, { season_id: p.seasonId });
  // The engine reports applied rules by id; show the name the manager gave them.
  const ruleName = (id: string) => rules.data?.find((r) => r.id === id)?.name ?? id;
  const [result, setResult] = useState<SimulateResult | null>(null);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const form = useForm<SimulatorValues>({
    resolver: zodResolver(simulatorSchema),
    defaultValues: simulatorDefaults({
      start: p.start,
      packId: p.packs[0]?.value ?? '',
      zoneId: p.zones[0]?.value ?? '',
    }),
  });
  const { control } = form;
  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      setResult(await simulate.mutateAsync(toSimulateRequest(undefined, v)));
    } catch (err) {
      setResult(null);
      if (err instanceof ApiProblem && (err.status === 422 || err.type === 'no_price')) {
        setAlert({ kind: 'server', messages: [err.detail ?? t('simulator.noPrice')] });
      } else {
        setAlert(
          problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }),
        );
      }
    }
  });

  if (p.packs.length === 0 || p.zones.length === 0) {
    return <EmptyState icon={PackageXIcon} title={t('simulator.missingCatalog')} />;
  }
  return (
    <div className="grid items-start gap-6 lg:grid-cols-[22rem_1fr]">
      <Form {...form}>
        <form
          noValidate
          onSubmit={submit}
          aria-label={t('simulator.formLabel')}
          className="min-w-0 lg:sticky lg:top-18"
        >
          <SectionCard
            title={t('simulator.formLabel')}
            footer={
              <Button type="submit" className="w-full" disabled={p.disabled || simulate.isPending}>
                {simulate.isPending ? <Spinner /> : <CalculatorIcon aria-hidden="true" />}
                {t('simulator.run')}
              </Button>
            }
          >
            <div className="space-y-4">
              <FormErrorAlert state={alert} />
              <SelectRow
                control={control}
                name="pack_id"
                label={t('fields.pack')}
                options={p.packs}
              />
              <SelectRow
                control={control}
                name="zone_id"
                label={t('fields.zone')}
                options={p.zones}
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <DateRow control={control} name="starts_on" label={t('fields.starts_on')} />
                <DateRow control={control} name="ends_on" label={t('fields.ends_on')} />
                <SelectRow
                  control={control}
                  name="formula"
                  label={t('fields.formula')}
                  options={FORMULAS.map((f) => ({ value: f, label: t(`formulas.${f}`) }))}
                />
                <SelectRow
                  control={control}
                  name="slots"
                  label={t('fields.slots')}
                  options={(['FULL', 'AM', 'PM'] as const).map((s) => ({
                    value: s,
                    label: t(`slots.${s}`),
                  }))}
                />
                <SelectRow
                  control={control}
                  name="channel"
                  label={t('fields.channel')}
                  options={CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) }))}
                />
                <DateRow control={control} name="today" label={t('fields.today')} clearable />
              </div>
              <Separator />
              {p.extras.length > 0 && (
                <div className="grid grid-cols-[1fr_5rem] gap-3">
                  <SelectRow
                    control={control}
                    name="extra_id"
                    label={t('fields.extra')}
                    options={[{ value: '', label: t('simulator.noExtra') }, ...p.extras]}
                  />
                  <TextRow
                    control={control}
                    name="extra_qty"
                    label={t('fields.quantity')}
                    inputProps={{ inputMode: 'numeric' }}
                  />
                </div>
              )}
              <div className="grid gap-3 sm:grid-cols-2">
                <TextRow
                  control={control}
                  name="promo_code"
                  label={t('fields.promo_code')}
                  inputProps={{
                    list: 'sim-promo-options',
                    autoComplete: 'off',
                    className: 'font-mono',
                  }}
                />
                <datalist id="sim-promo-options">
                  {(promos.data ?? []).map((x) => (
                    <option key={x.id} value={x.code} />
                  ))}
                </datalist>
                <SelectRow
                  control={control}
                  name="agreement_id"
                  label={t('fields.agreement')}
                  options={[
                    { value: '', label: t('simulator.noAgreement') },
                    ...(agreementList.data ?? []).map((a) => ({ value: a.id, label: a.name })),
                  ]}
                />
              </div>
              <TextRow control={control} name="customer_tags" label={t('fields.customer_tags')} />
            </div>
          </SectionCard>
        </form>
      </Form>
      <div aria-live="polite">
        {result ? (
          <Breakdown result={result} locale={p.locale} ruleName={ruleName} names={p.names} />
        ) : (
          <EmptyState icon={ReceiptTextIcon} title={t('simulator.hint')} />
        )}
      </div>
    </div>
  );
}

function weekday(date: string, locale: string): string {
  const d = isoToDate(date);
  return d ? new Intl.DateTimeFormat(locale, { weekday: 'short' }).format(d) : '';
}

interface VatRow {
  rate: string;
  taxable: number;
  tax: number;
}

function Breakdown({
  result,
  locale,
  ruleName,
  names,
}: {
  result: SimulateResult;
  locale: string;
  ruleName: (id: string) => string;
  names: CodeNames;
}) {
  const { t } = useTranslation('pricing');
  const [showAll, setShowAll] = useState(false);
  const b = result.breakdown;
  const money = (c: number) => formatCents(c, b.currency);
  const allDays = b.lines.flatMap((l) => l.days);
  const chartData = allDays.map((d) => ({
    day: formatIsoDate(d.date).slice(0, 5),
    total: d.total / 100,
  }));
  const chartConfig = {
    total: { label: t('simulator.dayTotal'), color: 'var(--chart-1)' },
  } satisfies ChartConfig;
  const vatColumns: ColumnDef<VatRow>[] = [
    {
      id: 'rate',
      meta: { label: t('simulator.vatRate') },
      header: t('simulator.vatRate'),
      cell: ({ row }) =>
        t('simulator.vat', { rate: row.original.rate, taxable: money(row.original.taxable) }),
    },
    {
      id: 'tax',
      meta: { label: t('simulator.vatTax'), align: 'right' },
      header: t('simulator.vatTax'),
      cell: ({ row }) => money(row.original.tax),
    },
  ];
  const DAY_LIMIT = 12;
  return (
    <section aria-label={t('simulator.result')} className="space-y-4">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <StatCard
          label={t('simulator.total')}
          value={<span data-testid="simulator-total">{money(b.total)}</span>}
        />
        {/* With a policy refusal the booking cannot be made: no amount to pay. */}
        <StatCard
          label={t('simulator.deposit')}
          value={
            result.policy.violations.length > 0 ? (
              <span className="text-muted-foreground">—</span>
            ) : (
              money(result.deposit_due)
            )
          }
        />
        {b.rounding !== 0 && <StatCard label={t('simulator.rounding')} value={money(b.rounding)} />}
      </div>

      {result.policy.violations.length > 0 && (
        <Alert variant="warning">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertTitle>{t('simulator.violations')}</AlertTitle>
          <AlertDescription>
            <ul className="list-inside list-disc">
              {result.policy.violations.map((v) => (
                <li key={v.code}>
                  {t(`violations.${v.code}`, { defaultValue: v.code, ...(v.params ?? {}) })}
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {allDays.length > 1 && (
        <SectionCard title={t('simulator.chartTitle')} as="h3">
          <ChartContainer config={chartConfig} className="aspect-auto h-40 w-full">
            <BarChart accessibilityLayer data={chartData}>
              <CartesianGrid vertical={false} />
              <XAxis dataKey="day" tickLine={false} axisLine={false} tickMargin={8} />
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    formatter={(value) => money(Math.round(Number(value) * 100))}
                  />
                }
              />
              <Bar dataKey="total" fill="var(--color-total)" radius={4} />
            </BarChart>
          </ChartContainer>
        </SectionCard>
      )}

      {b.lines.map((line, i) => {
        const days = showAll ? line.days : line.days.slice(0, DAY_LIMIT);
        return (
          <SectionCard
            key={`${line.pack}-${i}`}
            as="h3"
            title={`${t('simulator.line', { pack: names.pack(line.pack), zone: names.zone(line.zone) })}${line.spot ? ` · ${line.spot}` : ''}`}
            description={
              <>
                {t('simulator.lineTotal')}:{' '}
                <span className="font-medium text-foreground tabular-nums">
                  {money(line.total)}
                </span>
              </>
            }
          >
            <div className="space-y-3">
              {line.fixed && (
                <div className="flex items-center justify-between rounded-lg border px-3 py-2 text-sm">
                  <span>
                    {t('simulator.fixed', { formula: t(`formulas.${line.fixed.formula}`) })}
                    {line.fixed.rate_period ? ` · ${names.period(line.fixed.rate_period)}` : ''}
                  </span>
                  <span className="tabular-nums">{money(line.fixed.amount)}</span>
                </div>
              )}
              <ul className="grid gap-3 sm:grid-cols-2">
                {days.map((d) => (
                  <li key={`${d.date}-${d.slots}`} className="space-y-3 rounded-lg border p-4">
                    <p className="flex items-center justify-between gap-2 text-sm font-semibold">
                      <span className="tabular-nums">
                        <span className="font-normal text-muted-foreground capitalize">
                          {weekday(d.date, locale)}
                        </span>{' '}
                        {formatIsoDate(d.date)}
                      </span>
                      {d.slots !== 'FULL' && (
                        <Badge variant="outline">{t(`slots.${d.slots}`)}</Badge>
                      )}
                    </p>
                    <ol
                      aria-label={t('simulator.applied')}
                      className="space-y-2 border-l pl-3 text-sm"
                    >
                      <li className="flex items-start gap-2">
                        <span
                          aria-hidden="true"
                          className="mt-1.5 size-2 shrink-0 rounded-full bg-muted-foreground"
                        />
                        <span className="flex flex-1 justify-between gap-2">
                          <span className="text-muted-foreground">{t('simulator.base')}</span>
                          <span className="tabular-nums">{money(d.base)}</span>
                        </span>
                      </li>
                      {d.applied.map((a, k) => (
                        <li key={`${a.rule}-${k}`} className="flex items-start gap-2">
                          <span
                            aria-hidden="true"
                            className="mt-1.5 size-2 shrink-0 rounded-full bg-primary"
                          />
                          <span className="flex flex-1 flex-wrap items-center justify-between gap-2">
                            <span className="flex flex-wrap items-center gap-1.5">
                              <StatusBadge tone={STAGE_TONE[a.stage]} dot={false}>
                                {t(`stages.${a.stage}`)}
                              </StatusBadge>
                              {ruleName(a.rule)}
                            </span>
                            <span className="text-muted-foreground tabular-nums">
                              {formatEffect(a.effect, money, locale)}
                            </span>
                          </span>
                        </li>
                      ))}
                    </ol>
                    <Separator />
                    <p className="flex justify-between text-sm font-medium">
                      <span>{t('simulator.dayTotal')}</span>
                      <span className="tabular-nums">{money(d.total)}</span>
                    </p>
                  </li>
                ))}
              </ul>
              {line.days.length > DAY_LIMIT && (
                <Button variant="outline" size="sm" onClick={() => setShowAll((v) => !v)}>
                  {showAll
                    ? t('simulator.showLess')
                    : t('simulator.showAll', { count: line.days.length })}
                </Button>
              )}
              {line.extras.map((x) => (
                <p key={x.code} className="flex justify-between text-sm">
                  <span>{t('simulator.extraLine', { code: x.code, qty: x.qty })}</span>
                  <span className="tabular-nums">{money(x.total)}</span>
                </p>
              ))}
            </div>
          </SectionCard>
        );
      })}

      {((b.extras ?? []).length > 0 || b.discounts.length > 0) && (
        <SectionCard title={t('simulator.adjustments')} as="h3">
          <div className="space-y-2 text-sm">
            {(b.extras ?? []).map((x) => (
              <p key={x.code} className="flex justify-between">
                <span>{t('simulator.extraLine', { code: x.code, qty: x.qty })}</span>
                <span className="tabular-nums">{money(x.total)}</span>
              </p>
            ))}
            {b.discounts.map((d) => (
              <p key={d.source} className="flex justify-between text-success">
                <span>{t('simulator.discount', { source: d.source })}</span>
                <span className="tabular-nums">−{money(d.amount)}</span>
              </p>
            ))}
          </div>
        </SectionCard>
      )}

      {b.vat.length > 0 && (
        <SectionCard title={t('simulator.vatTitle')} as="h3" flush>
          <DataTable
            embedded
            caption={t('simulator.vatTitle')}
            columns={vatColumns}
            data={b.vat}
            getRowId={(v) => v.rate}
            pageSize={0}
            columnToggle={false}
          />
        </SectionCard>
      )}
    </section>
  );
}
