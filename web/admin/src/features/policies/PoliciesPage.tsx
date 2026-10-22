import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm, type Control, type FieldErrors, type UseFormRegister } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { createResource } from '../configuration/api/resource';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { useEstablishmentSection } from '../configuration/api/useEstablishmentSection';
import { Badge, ConfigPage, EmptyState, QueryState } from '../configuration/components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../configuration/components/dialogs';
import { Checkbox, Field } from '../configuration/components/fields';
import { FormAlert, SavedNotice } from '../configuration/components/FormAlert';
import { MultiCheck } from '../configuration/components/MultiCheck';
import { fieldAria } from '../configuration/lib/aria';
import { PricingNav, SeasonSelect } from '../pricing/components';
import { useSelectedSeason } from '../pricing/season';
import { usePricingData } from '../pricing/usePricingData';
import {
  defaultPolicySchema,
  policySchema,
  policyValues,
  rulesValues,
  toPolicyInput,
  toRules,
  type BookingPolicy,
  type BookingPolicyInput,
  type DefaultPolicyValues,
  type PolicyRules,
  type PolicyValues,
  type RulesValues,
} from './policy.schema';
import {
  POLICY_EXAMPLES,
  policyExample,
  type ExampleCatalog,
  type PolicyExampleId,
} from './examples';
import { RulesFields } from './RulesFields';

const policies = createResource<BookingPolicy, BookingPolicyInput, { season_id?: string }>({
  key: 'policies',
  collection: '/api/admin/v1/establishments/{establishmentId}/policies',
  item: '/api/admin/v1/establishments/{establishmentId}/policies/{policyId}',
  idParam: 'policyId',
});

const CHANNELS = ['desk', 'online', 'api'] as const;

/** Configurazione → Policy (spec 9): establishment default policy + season overrides by scope. */
export function PoliciesPage() {
  const { t } = useTranslation('policies');
  const ctx = useConfigContext();
  const sel = useSelectedSeason(ctx.establishmentId);
  const data = usePricingData(ctx);
  const readOnly = !ctx.isLoading && !ctx.canConfigure;
  const list = policies.useList(
    ctx.establishmentId,
    { season_id: sel.season?.id },
    { enabled: !!sel.season },
  );
  const remove = policies.useRemove(ctx.establishmentId);
  const editor = useEditor<BookingPolicy>();
  const items = [...(list.data ?? [])].sort((a, b) => b.priority - a.priority);
  const [tab, setTab] = useState<'season' | 'default'>('season');

  const scopeText = (p: BookingPolicy) => {
    const parts: string[] = [];
    if (p.scope.channels?.length)
      parts.push(p.scope.channels.map((c) => t(`pricing:channels.${c}`)).join(', '));
    if (p.scope.zones?.length) {
      parts.push(
        p.scope.zones
          .map((z) => data.zones.find((x) => x.id === z || x.code === z)?.name ?? z)
          .join(', '),
      );
    }
    if (p.scope.packs?.length) {
      parts.push(
        p.scope.packs
          .map((id) => {
            const pk = data.packs.find((x) => x.id === id || x.code === id);
            return pk ? data.name(pk.name) || pk.code : id;
          })
          .join(', '),
      );
    }
    if (p.scope.rate_periods?.length) parts.push(p.scope.rate_periods.join(', '));
    return parts.length ? parts.join(' · ') : t('scopeAll');
  };

  return (
    <ConfigPage
      title={t('title')}
      description={t('description')}
      readOnly={readOnly}
      actions={<SeasonSelect seasons={sel.seasons} season={sel.season} onSelect={sel.select} />}
    >
      <PricingNav establishmentId={ctx.establishmentId} seasonId={sel.season?.id} />
      <div
        role="tablist"
        aria-label={t('tabs.label')}
        className="flex gap-1 border-b border-slate-200"
      >
        {(['season', 'default'] as const).map((id) => (
          <button
            key={id}
            id={`policy-tab-${id}`}
            type="button"
            role="tab"
            aria-selected={tab === id}
            aria-controls={`policy-panel-${id}`}
            onClick={() => setTab(id)}
            className={
              tab === id
                ? '-mb-px border-b-2 border-[var(--ar-primary)] px-3 py-2 text-sm font-semibold'
                : 'px-3 py-2 text-sm text-slate-600 hover:text-slate-900'
            }
          >
            {t(`tabs.${id}`)}
          </button>
        ))}
      </div>
      {tab === 'default' && (
        <div role="tabpanel" id="policy-panel-default" aria-labelledby="policy-tab-default">
          {ctx.establishment ? (
            <DefaultPolicyForm
              key={ctx.establishment.id}
              establishmentId={ctx.establishmentId}
              rules={ctx.establishment.settings.default_policy}
              startVersion={ctx.establishment.version}
              locale={ctx.uiLocale}
              readOnly={readOnly}
            />
          ) : (
            <QueryState isLoading={ctx.isLoading} error={ctx.error} onRetry={ctx.refetch} />
          )}
        </div>
      )}
      {tab === 'season' && (
        <div
          role="tabpanel"
          id="policy-panel-season"
          aria-labelledby="policy-tab-season"
          className="space-y-4"
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm text-slate-600">{t('seasonHint')}</p>
            {!readOnly && sel.season && <Button onClick={editor.create}>{t('add')}</Button>}
          </div>
          <QueryState
            isLoading={sel.isLoading || (!!sel.season && list.isPending)}
            error={sel.error ?? list.error}
            onRetry={() => void list.refetch()}
          />
          {!sel.isLoading && sel.seasons.length === 0 && (
            <EmptyState>{t('pricing:noSeason')}</EmptyState>
          )}
          {list.data && items.length === 0 && <EmptyState>{t('empty')}</EmptyState>}
          {items.length > 0 && (
            <Card className="overflow-x-auto">
              <table className="w-full min-w-[36rem] text-sm">
                <caption className="sr-only">{t('title')}</caption>
                <thead className="bg-slate-50 text-left text-slate-600">
                  <tr>
                    <th scope="col" className="px-3 py-2 font-medium">
                      {t('fields.name')}
                    </th>
                    <th scope="col" className="px-3 py-2 font-medium">
                      {t('fields.scope')}
                    </th>
                    <th scope="col" className="px-3 py-2 text-right font-medium">
                      {t('fields.priority')}
                    </th>
                    <th scope="col" className="px-3 py-2">
                      <span className="sr-only">{t('actions')}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((p) => (
                    <tr key={p.id} className="border-t border-slate-100">
                      <th scope="row" className="px-3 py-2 text-left font-medium">
                        {p.name}
                        {!p.active && (
                          <span className="ml-2">
                            <Badge>{t('inactive')}</Badge>
                          </span>
                        )}
                      </th>
                      <td className="px-3 py-2">{scopeText(p)}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{p.priority}</td>
                      <td className="px-3 py-2 text-right">
                        {!readOnly && (
                          <div className="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => editor.edit(p)}
                              aria-label={t('editAria', { name: p.name })}
                            >
                              {t('common.actions.edit', { ns: 'translation' })}
                            </Button>
                            <ConfirmDelete
                              label={t('deleteAria', { name: p.name })}
                              message={t('deleteConfirm', { name: p.name })}
                              onConfirm={() => remove.mutateAsync({ id: p.id, version: p.version })}
                            />
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>
          )}
        </div>
      )}
      {sel.season && (
        <FormDialog
          open={editor.open}
          onOpenChange={editor.setOpen}
          wide
          title={editor.item ? t('editTitle', { name: editor.item.name }) : t('add')}
        >
          <PolicyForm
            key={editor.key}
            establishmentId={ctx.establishmentId}
            seasonId={sel.season.id}
            item={editor.item}
            locale={ctx.uiLocale}
            zones={data.zones}
            packs={data.packs.map((p) => ({ id: p.id, name: data.name(p.name) || p.code }))}
            periods={sel.periods.map((p) => p.code)}
            catalog={{
              packs: data.packs.map((p) => ({
                id: p.id,
                code: p.code,
                composition: p.composition as Record<string, number>,
              })),
              periods: sel.periods.map((p) => ({ code: p.code, ranges: p.ranges })),
            }}
            onDone={editor.close}
          />
        </FormDialog>
      )}
    </ConfigPage>
  );
}

function PolicyForm(p: {
  establishmentId: string;
  seasonId: string;
  item?: BookingPolicy;
  locale: string;
  zones: { id: string; name: string }[];
  packs: { id: string; name: string }[];
  periods: string[];
  catalog: ExampleCatalog;
  onDone: () => void;
}) {
  const { t } = useTranslation('policies');
  const [exampleWarnings, setExampleWarnings] = useState<string[]>([]);
  const form = useForm<PolicyValues>({
    resolver: zodResolver(policySchema),
    defaultValues: policyValues(p.item, p.locale),
  });
  const s = useCrudSubmit({
    form,
    item: p.item,
    create: policies.useCreate(p.establishmentId),
    update: policies.useUpdate(p.establishmentId),
    toInput: (v: PolicyValues) => toPolicyInput(p.seasonId, v),
    onDone: p.onDone,
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const applyExample = (id: PolicyExampleId) => {
    const fill = policyExample(id, p.catalog);
    const values = { name: form.getValues('name') || t(`examples.${id}`), ...fill.values };
    for (const [key, value] of Object.entries(values)) {
      form.setValue(key as keyof PolicyValues, value as never, {
        shouldDirty: true,
        shouldValidate: form.formState.isSubmitted,
      });
    }
    setExampleWarnings(fill.warnings);
  };
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={p.onDone} />
      <div className="space-y-2 rounded-md bg-slate-50 p-3">
        <p className="text-sm font-medium text-slate-800">{t('examples.title')}</p>
        <div className="flex flex-wrap gap-2">
          {POLICY_EXAMPLES.map((id) => (
            <Button key={id} variant="outline" size="sm" onClick={() => applyExample(id)}>
              {t(`examples.${id}`)}
            </Button>
          ))}
        </div>
        <p className="text-xs text-slate-600">{t('examples.hint')}</p>
        {exampleWarnings.length > 0 && (
          <ul role="status" className="list-inside list-disc text-xs text-amber-800">
            {exampleWarnings.map((w) => (
              <li key={w}>{t(w)}</li>
            ))}
          </ul>
        )}
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="pol-name" label={t('fields.name')} error={errors.name} className="sm:col-span-2">
          <Input {...fieldAria('pol-name', errors.name)} {...register('name')} />
        </Field>
        <Field id="pol-priority" label={t('fields.priority')} error={errors.priority}>
          <Input
            {...fieldAria('pol-priority', errors.priority)}
            inputMode="numeric"
            {...register('priority')}
          />
        </Field>
      </div>
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium">{t('fields.scope')}</legend>
        <p className="text-xs text-slate-600">{t('hints.scope')}</p>
        <MultiCheck
          control={control}
          name="scope_channels"
          id="pol-scope-channels"
          legend={t('scope.channels')}
          options={CHANNELS.map((c) => ({ value: c, label: t(`pricing:channels.${c}`) }))}
        />
        {p.zones.length > 0 && (
          <MultiCheck
            control={control}
            name="scope_zones"
            id="pol-scope-zones"
            legend={t('scope.zones')}
            options={p.zones.map((z) => ({ value: z.id, label: z.name }))}
          />
        )}
        {p.packs.length > 0 && (
          <MultiCheck
            control={control}
            name="scope_packs"
            id="pol-scope-packs"
            legend={t('scope.packs')}
            options={p.packs.map((x) => ({ value: x.id, label: x.name }))}
          />
        )}
        {p.periods.length > 0 && (
          <MultiCheck
            control={control}
            name="scope_periods"
            id="pol-scope-periods"
            legend={t('scope.periods')}
            options={p.periods.map((c) => ({ value: c, label: c }))}
          />
        )}
      </fieldset>
      <RulesFields
        control={control as unknown as Control<RulesValues>}
        register={register as unknown as UseFormRegister<RulesValues>}
        errors={errors as FieldErrors<RulesValues>}
        idPrefix="pol"
      />
      <Checkbox id="pol-active" label={t('fields.active')} {...register('active')} />
      <FormActions onCancel={p.onDone} pending={s.pending} />
    </form>
  );
}

function DefaultPolicyForm(p: {
  establishmentId: string;
  rules: PolicyRules;
  startVersion: number;
  locale: string;
  readOnly: boolean;
}) {
  const { t } = useTranslation('policies');
  const [version, setVersion] = useState(p.startVersion);
  const form = useForm<DefaultPolicyValues>({
    resolver: zodResolver(defaultPolicySchema),
    defaultValues: rulesValues(p.rules, p.locale),
  });
  const section = useEstablishmentSection({
    establishmentId: p.establishmentId,
    form,
    version,
    onVersion: setVersion,
    toPatch: (v, current) => ({ settings: { ...current.settings, default_policy: toRules(v) } }),
    toValues: (est) => rulesValues(est.settings.default_policy, p.locale),
  });
  return (
    <Card className="p-4">
      <form noValidate onSubmit={section.submit} className="space-y-4">
        <p className="text-sm text-slate-600">{t('defaultHint')}</p>
        <FormAlert state={section.alert} onReload={section.reload} />
        <fieldset disabled={p.readOnly} className="space-y-4">
          <RulesFields
            control={form.control}
            register={form.register}
            errors={form.formState.errors}
            idPrefix="def"
            isDefault
          />
        </fieldset>
        {!p.readOnly && (
          <div className="flex items-center justify-end gap-3">
            <SavedNotice show={section.saved} />
            <Button type="submit" disabled={section.pending}>
              {t('common.actions.save', { ns: 'translation' })}
            </Button>
          </div>
        )}
      </form>
    </Card>
  );
}
