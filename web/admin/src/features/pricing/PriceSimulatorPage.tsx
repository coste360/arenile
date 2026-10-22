import { ApiProblem } from '@arenile/shared/api';
import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { Field, Select } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { formatCents } from '../configuration/lib/money';
import { agreements, priceRules, promoCodes, useSimulate, type SimulateResult } from './api';
import { PricingNav, SeasonSelect } from './components';
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
    <ConfigPage
      title={t('simulator.title')}
      description={t('simulator.description')}
      readOnly={!ctx.isLoading && !ctx.canConfigure}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      <PricingNav establishmentId={ctx.establishmentId} seasonId={sel.season?.id} />
      <QueryState
        isLoading={sel.isLoading || data.isLoading}
        error={sel.error ?? data.error}
        onRetry={sel.refetch}
      />
      {!sel.isLoading && sel.seasons.length === 0 && <EmptyState>{t('noSeason')}</EmptyState>}
      {ready && sel.season && (
        <Simulator
          key={sel.season.id}
          establishmentId={ctx.establishmentId}
          seasonId={sel.season.id}
          start={sel.season.opens_on}
          packs={data.packs
            .filter((p) => p.active)
            .map((p) => ({ id: p.id, name: data.name(p.name) || p.code }))}
          zones={data.rawZones.map((z) => ({
            id: z.id,
            name: `${data.name(z.name) || z.code} (${z.price_category})`,
          }))}
          extras={data.extras
            .filter((x) => x.active)
            .map((x) => ({ id: x.id, name: data.name(x.name) || x.code }))}
          locale={ctx.uiLocale}
          disabled={!ctx.canConfigure}
        />
      )}
    </ConfigPage>
  );
}

type Option = { id: string; name: string };

function Simulator(p: {
  establishmentId: string;
  seasonId: string;
  start: string;
  packs: Option[];
  zones: Option[];
  extras: Option[];
  locale: string;
  disabled: boolean;
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
      packId: p.packs[0]?.id ?? '',
      zoneId: p.zones[0]?.id ?? '',
    }),
  });
  const {
    register,
    formState: { errors },
  } = form;
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
  const select = (
    name: keyof SimulatorValues,
    label: string,
    options: { value: string; label: string }[],
    empty?: string,
  ) => (
    <Field id={`sim-${name}`} label={label} error={errors[name]}>
      <Select {...fieldAria(`sim-${name}`, errors[name])} {...register(name)}>
        {empty !== undefined && <option value="">{empty}</option>}
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </Select>
    </Field>
  );
  const input = (name: keyof SimulatorValues, label: string, type = 'text') => (
    <Field id={`sim-${name}`} label={label} error={errors[name]}>
      <Input {...fieldAria(`sim-${name}`, errors[name])} type={type} {...register(name)} />
    </Field>
  );
  const opts = (xs: Option[]) => xs.map((x) => ({ value: x.id, label: x.name }));

  if (p.packs.length === 0 || p.zones.length === 0) {
    return <EmptyState>{t('simulator.missingCatalog')}</EmptyState>;
  }
  return (
    <div className="grid gap-4 lg:grid-cols-[22rem_1fr]">
      <Card className="h-fit p-4">
        <form
          noValidate
          onSubmit={submit}
          className="space-y-3"
          aria-label={t('simulator.formLabel')}
        >
          <FormAlert state={alert} />
          {select('pack_id', t('fields.pack'), opts(p.packs))}
          {select('zone_id', t('fields.zone'), opts(p.zones))}
          <div className="grid grid-cols-2 gap-3">
            {input('starts_on', t('fields.starts_on'), 'date')}
            {input('ends_on', t('fields.ends_on'), 'date')}
            {select(
              'formula',
              t('fields.formula'),
              FORMULAS.map((f) => ({ value: f, label: t(`formulas.${f}`) })),
            )}
            {select(
              'slots',
              t('fields.slots'),
              (['FULL', 'AM', 'PM'] as const).map((s) => ({ value: s, label: t(`slots.${s}`) })),
            )}
            {select(
              'channel',
              t('fields.channel'),
              CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) })),
            )}
            {input('today', t('fields.today'), 'date')}
          </div>
          {p.extras.length > 0 && (
            <div className="grid grid-cols-[1fr_5rem] gap-3">
              {select('extra_id', t('fields.extra'), opts(p.extras), t('simulator.noExtra'))}
              {input('extra_qty', t('fields.quantity'))}
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <Field id="sim-promo_code" label={t('fields.promo_code')} error={errors.promo_code}>
              <Input
                {...fieldAria('sim-promo_code', errors.promo_code)}
                list="sim-promo-options"
                {...register('promo_code')}
              />
              <datalist id="sim-promo-options">
                {(promos.data ?? []).map((x) => (
                  <option key={x.id} value={x.code} />
                ))}
              </datalist>
            </Field>
            {select(
              'agreement_id',
              t('fields.agreement'),
              (agreementList.data ?? []).map((a) => ({ value: a.id, label: a.name })),
              t('simulator.noAgreement'),
            )}
          </div>
          {input('customer_tags', t('fields.customer_tags'))}
          <Button type="submit" className="w-full" disabled={p.disabled || simulate.isPending}>
            {t('simulator.run')}
          </Button>
        </form>
      </Card>
      <div aria-live="polite">
        {result ? (
          <Breakdown result={result} locale={p.locale} ruleName={ruleName} />
        ) : (
          <p className="text-sm text-slate-600">{t('simulator.hint')}</p>
        )}
      </div>
    </div>
  );
}

function Breakdown({
  result,
  locale,
  ruleName,
}: {
  result: SimulateResult;
  locale: string;
  ruleName: (id: string) => string;
}) {
  const { t } = useTranslation('pricing');
  const b = result.breakdown;
  const money = (c: number) => formatCents(c, locale, b.currency);
  return (
    <section aria-label={t('simulator.result')} className="space-y-4">
      <Card className="flex flex-wrap items-end justify-between gap-4 p-4">
        <div>
          <p className="text-sm text-slate-600">{t('simulator.total')}</p>
          <p className="text-3xl font-semibold tabular-nums" data-testid="simulator-total">
            {money(b.total)}
          </p>
        </div>
        <div className="text-right text-sm">
          <p>
            {t('simulator.deposit')}:{' '}
            <strong className="tabular-nums">{money(result.deposit_due)}</strong>
          </p>
          {b.rounding !== 0 && (
            <p className="text-slate-600">
              {t('simulator.rounding')}: {money(b.rounding)}
            </p>
          )}
        </div>
      </Card>
      {result.policy.violations.length > 0 && (
        <div
          role="alert"
          className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900"
        >
          <p className="font-medium">{t('simulator.violations')}</p>
          <ul className="list-inside list-disc">
            {result.policy.violations.map((v) => (
              <li key={v.code}>
                {t(`violations.${v.code}`, { defaultValue: v.code, ...(v.params ?? {}) })}
              </li>
            ))}
          </ul>
        </div>
      )}
      {b.lines.map((line, i) => (
        <Card key={`${line.pack}-${i}`} className="overflow-x-auto p-0">
          <table className="w-full min-w-[32rem] text-sm">
            <caption className="px-4 pt-3 text-left font-medium">
              {t('simulator.line', { pack: line.pack, zone: line.zone })}
              {line.spot ? ` · ${line.spot}` : ''}
            </caption>
            <thead className="text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('simulator.day')}
                </th>
                <th scope="col" className="px-4 py-2 text-right font-medium">
                  {t('simulator.base')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('simulator.applied')}
                </th>
                <th scope="col" className="px-4 py-2 text-right font-medium">
                  {t('simulator.dayTotal')}
                </th>
              </tr>
            </thead>
            <tbody>
              {line.fixed && (
                <tr className="border-t border-slate-100">
                  <th scope="row" className="px-4 py-2 text-left font-normal">
                    {t('simulator.fixed', { formula: t(`formulas.${line.fixed.formula}`) })}
                    {line.fixed.rate_period ? ` · ${line.fixed.rate_period}` : ''}
                  </th>
                  <td className="px-4 py-2" />
                  <td className="px-4 py-2" />
                  <td className="px-4 py-2 text-right tabular-nums">{money(line.fixed.amount)}</td>
                </tr>
              )}
              {line.days.map((d) => (
                <tr key={`${d.date}-${d.slots}`} className="border-t border-slate-100 align-top">
                  <th scope="row" className="px-4 py-2 text-left font-normal tabular-nums">
                    {d.date} <span className="text-xs text-slate-500">{d.slots}</span>
                  </th>
                  <td className="px-4 py-2 text-right tabular-nums">{money(d.base)}</td>
                  <td className="px-4 py-2">
                    {d.applied.length === 0 ? (
                      <span className="text-slate-500">—</span>
                    ) : (
                      <ul className="space-y-0.5">
                        {d.applied.map((a, k) => (
                          <li key={`${a.rule}-${k}`}>
                            <span className="text-xs text-slate-500">{t(`stages.${a.stage}`)}</span>{' '}
                            {ruleName(a.rule)}: {a.effect}
                          </li>
                        ))}
                      </ul>
                    )}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums">{money(d.total)}</td>
                </tr>
              ))}
              {line.extras.map((x) => (
                <tr key={x.code} className="border-t border-slate-100">
                  <th scope="row" className="px-4 py-2 text-left font-normal">
                    {t('simulator.extraLine', { code: x.code, qty: x.qty })}
                  </th>
                  <td className="px-4 py-2" />
                  <td className="px-4 py-2" />
                  <td className="px-4 py-2 text-right tabular-nums">{money(x.total)}</td>
                </tr>
              ))}
              <tr className="border-t border-slate-200 font-medium">
                <th scope="row" className="px-4 py-2 text-left">
                  {t('simulator.lineTotal')}
                </th>
                <td className="px-4 py-2" />
                <td className="px-4 py-2" />
                <td className="px-4 py-2 text-right tabular-nums">{money(line.total)}</td>
              </tr>
            </tbody>
          </table>
        </Card>
      ))}
      {((b.extras ?? []).length > 0 || b.discounts.length > 0 || b.vat.length > 0) && (
        <Card className="space-y-2 p-4 text-sm">
          {(b.extras ?? []).map((x) => (
            <p key={x.code} className="flex justify-between">
              <span>{t('simulator.extraLine', { code: x.code, qty: x.qty })}</span>
              <span className="tabular-nums">{money(x.total)}</span>
            </p>
          ))}
          {b.discounts.map((d) => (
            <p key={d.source} className="flex justify-between text-emerald-800">
              <span>{t('simulator.discount', { source: d.source })}</span>
              <span className="tabular-nums">−{money(d.amount)}</span>
            </p>
          ))}
          {b.vat.map((v) => (
            <p key={v.rate} className="flex justify-between text-slate-600">
              <span>{t('simulator.vat', { rate: v.rate, taxable: money(v.taxable) })}</span>
              <span className="tabular-nums">{money(v.tax)}</span>
            </p>
          ))}
        </Card>
      )}
    </section>
  );
}
