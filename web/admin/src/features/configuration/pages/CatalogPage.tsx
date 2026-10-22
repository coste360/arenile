import { Button, Card, Input, cn } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useRef, useState, type KeyboardEvent } from 'react';
import { useFieldArray, useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  components,
  extras,
  packs,
  pools,
  zones,
  type CatalogComponent,
  type Extra,
  type Pack,
} from '../api/resources';
import { useConfigContext, type ConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { Badge, EmptyState, ConfigPage, QueryState } from '../components/ConfigPage';
import { ComponentIcon } from '../components/ComponentIcon';
import { ConfirmDelete, FormActions, FormDialog } from '../components/dialogs';
import { Checkbox, Field, FieldError, Select } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { I18nTextInput } from '../components/I18nTextInput';
import { MultiCheck } from '../components/MultiCheck';
import { errorAt, fieldAria } from '../lib/aria';
import { localize } from '../lib/i18nText';
import { basisPointsToPercentInput } from '../lib/money';
import {
  ATTACH_TO,
  CHANNELS,
  FORMULAS,
  PRICING_UNITS,
  SPOT_KINDS,
  componentSchema,
  extraAliases,
  extraSchema,
  packAliases,
  packSchema,
  toComponentInput,
  toComponentValues,
  toExtraInput,
  toExtraValues,
  toPackInput,
  toPackValues,
} from './catalog.schema';

const TABS = ['components', 'packs', 'extras'] as const;
type Tab = (typeof TABS)[number];

/** Configurazione → Catalogo: components, packs and extras (spec 5.3). */
export function CatalogPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const [tab, setTab] = useState<Tab>('packs');
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});

  const onKey = (e: KeyboardEvent<HTMLButtonElement>) => {
    const i = TABS.indexOf(tab);
    let next: Tab | undefined;
    if (e.key === 'ArrowRight') next = TABS[(i + 1) % TABS.length];
    if (e.key === 'ArrowLeft') next = TABS[(i + TABS.length - 1) % TABS.length];
    if (e.key === 'Home') next = TABS[0];
    if (e.key === 'End') next = TABS[TABS.length - 1];
    if (next) {
      e.preventDefault();
      setTab(next);
      refs.current[next]?.focus();
    }
  };

  return (
    <ConfigPage
      title={t('catalog.title')}
      description={t('catalog.description')}
      readOnly={!ctx.canConfigure}
    >
      <div
        role="tablist"
        aria-label={t('catalog.title')}
        className="flex gap-1 border-b border-slate-200"
      >
        {TABS.map((id) => (
          <button
            key={id}
            ref={(el) => {
              refs.current[id] = el;
            }}
            id={`tab-${id}`}
            type="button"
            role="tab"
            aria-selected={tab === id}
            aria-controls={`panel-${id}`}
            tabIndex={tab === id ? 0 : -1}
            onClick={() => setTab(id)}
            onKeyDown={onKey}
            className={cn(
              '-mb-px border-b-2 px-4 py-2 text-sm font-medium',
              tab === id
                ? 'border-[var(--ar-primary)] text-[var(--ar-primary-on-surface)]'
                : 'border-transparent text-slate-600 hover:text-slate-900',
            )}
          >
            {t(`catalog.tabs.${id}`)}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`} tabIndex={0}>
        {tab === 'components' && <ComponentsTab ctx={ctx} />}
        {tab === 'packs' && <PacksTab ctx={ctx} />}
        {tab === 'extras' && <ExtrasTab ctx={ctx} />}
      </div>
    </ConfigPage>
  );
}

function RowActions({
  readOnly,
  name,
  onEdit,
  onDelete,
}: {
  readOnly: boolean;
  name: string;
  onEdit: () => void;
  onDelete: () => Promise<unknown>;
}) {
  const { t } = useTranslation('configuration');
  if (readOnly) return null;
  return (
    <div className="flex justify-end gap-1">
      <Button
        variant="ghost"
        size="sm"
        onClick={onEdit}
        aria-label={t('catalog.editAria', { name })}
      >
        {t('common.actions.edit', { ns: 'translation' })}
      </Button>
      <ConfirmDelete
        label={t('catalog.deleteAria', { name })}
        message={t('catalog.deleteConfirm', { name })}
        onConfirm={onDelete}
      />
    </div>
  );
}

// ---- Components ---------------------------------------------------------------------

function ComponentsTab({ ctx }: { ctx: ConfigContext }) {
  const { t } = useTranslation('configuration');
  const list = components.useList(ctx.establishmentId);
  const poolList = pools.useList(ctx.establishmentId);
  const remove = components.useRemove(ctx.establishmentId);
  const editor = useEditor<CatalogComponent>();
  const readOnly = !ctx.canConfigure;
  const loc = (x: Record<string, string>) => localize(x, ctx.uiLocale, ctx.defaultLocale);
  const items = list.data ?? [];
  return (
    <div className="space-y-4 pt-4">
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm text-slate-600">{t('catalog.componentsHint')}</p>
        {!readOnly && <Button onClick={editor.create}>{t('catalog.addComponent')}</Button>}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('catalog.emptyComponents')}</EmptyState>}
      {items.length > 0 && (
        <Card>
          <ul className="divide-y divide-slate-100">
            {items.map((c) => {
              const name = loc(c.name) || c.code;
              const pool = poolList.data?.find((p) => p.id === c.pool_id);
              return (
                <li key={c.id} className="flex flex-wrap items-center gap-3 px-4 py-2">
                  <ComponentIcon code={c.code} className="text-slate-700" />
                  <span className="font-medium">{name}</span>
                  <span className="font-mono text-xs text-slate-500">{c.code}</span>
                  {c.tracked && (
                    <Badge tone="blue">
                      {pool
                        ? t('catalog.trackedIn', { pool: loc(pool.name) || pool.component_code })
                        : t('catalog.tracked')}
                    </Badge>
                  )}
                  <span className="ml-auto">
                    <RowActions
                      readOnly={readOnly}
                      name={name}
                      onEdit={() => editor.edit(c)}
                      onDelete={() => remove.mutateAsync({ id: c.id, version: c.version })}
                    />
                  </span>
                </li>
              );
            })}
          </ul>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('catalog.editComponent') : t('catalog.addComponent')}
      >
        <ComponentForm key={editor.key} ctx={ctx} item={editor.item} onDone={editor.close} />
      </FormDialog>
    </div>
  );
}

function ComponentForm({
  ctx,
  item,
  onDone,
}: {
  ctx: ConfigContext;
  item?: CatalogComponent;
  onDone: () => void;
}) {
  const { t } = useTranslation('configuration');
  const poolList = pools.useList(ctx.establishmentId);
  const form = useForm({
    resolver: zodResolver(componentSchema(ctx.defaultLocale)),
    defaultValues: toComponentValues(item, ctx.locales),
  });
  const create = components.useCreate(ctx.establishmentId);
  const update = components.useUpdate(ctx.establishmentId);
  const s = useCrudSubmit({ form, item, create, update, toInput: toComponentInput, onDone });
  const {
    register,
    formState: { errors },
  } = form;
  const code = useWatch({ control: form.control, name: 'code' });
  const tracked = useWatch({ control: form.control, name: 'tracked' });
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onDone} />
      <Field
        id="cmp-code"
        label={t('fields.code')}
        error={errors.code}
        hint={t('catalog.componentCodeHint')}
      >
        <div className="flex items-center gap-2">
          <ComponentIcon code={code} className="text-slate-700" />
          <Input {...fieldAria('cmp-code', errors.code, true)} {...register('code')} />
        </div>
      </Field>
      <I18nTextInput
        name="name"
        idPrefix="cmp"
        label={t('fields.name')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
        register={register}
        errors={errors}
      />
      <Checkbox
        id="cmp-tracked"
        label={t('catalog.fields.tracked')}
        description={t('catalog.trackedHint')}
        {...register('tracked')}
      />
      {tracked && (
        <Field id="cmp-pool" label={t('catalog.fields.pool_id')} error={errors.pool_id}>
          <Select {...fieldAria('cmp-pool', errors.pool_id)} {...register('pool_id')}>
            <option value="">{t('choose')}</option>
            {(poolList.data ?? []).map((p) => (
              <option key={p.id} value={p.id}>
                {localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.component_code}
              </option>
            ))}
          </Select>
        </Field>
      )}
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}

// ---- Packs ---------------------------------------------------------------------------

function PacksTab({ ctx }: { ctx: ConfigContext }) {
  const { t } = useTranslation('configuration');
  const list = packs.useList(ctx.establishmentId);
  const remove = packs.useRemove(ctx.establishmentId);
  const editor = useEditor<Pack>();
  const readOnly = !ctx.canConfigure;
  const items = [...(list.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const loc = (x: Record<string, string>) => localize(x, ctx.uiLocale, ctx.defaultLocale);
  return (
    <div className="space-y-4 pt-4">
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm text-slate-600">{t('catalog.packsHint')}</p>
        {!readOnly && <Button onClick={editor.create}>{t('catalog.addPack')}</Button>}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('catalog.emptyPacks')}</EmptyState>}
      {items.length > 0 && (
        <Card>
          <ul className="divide-y divide-slate-100">
            {items.map((p) => {
              const name = loc(p.name) || p.code;
              return (
                <li key={p.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                  <span className="font-medium">{name}</span>
                  <span className="font-mono text-xs text-slate-500">{p.code}</span>
                  <span
                    className="flex flex-wrap items-center gap-2 text-sm text-slate-700"
                    aria-label={t('catalog.compositionAria', {
                      list: Object.entries(p.composition)
                        .map(([c, q]) => `${q} × ${c}`)
                        .join(', '),
                    })}
                  >
                    {Object.entries(p.composition).map(([code, qty]) => (
                      <span
                        key={code}
                        className="inline-flex items-center gap-1 rounded bg-slate-100 px-1.5 py-0.5"
                        aria-hidden="true"
                      >
                        {qty}× <ComponentIcon code={code} />
                      </span>
                    ))}
                  </span>
                  {!p.active && <Badge tone="amber">{t('catalog.inactive')}</Badge>}
                  {p.requires_approval && <Badge>{t('catalog.fields.requires_approval')}</Badge>}
                  {p.vat_rate_bp !== undefined && (
                    <Badge>
                      {t('catalog.vatBadge', {
                        rate: basisPointsToPercentInput(p.vat_rate_bp, ctx.uiLocale),
                      })}
                    </Badge>
                  )}
                  <span className="ml-auto">
                    <RowActions
                      readOnly={readOnly}
                      name={name}
                      onEdit={() => editor.edit(p)}
                      onDelete={() => remove.mutateAsync({ id: p.id, version: p.version })}
                    />
                  </span>
                </li>
              );
            })}
          </ul>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('catalog.editPack') : t('catalog.addPack')}
      >
        <PackForm key={editor.key} ctx={ctx} item={editor.item} onDone={editor.close} />
      </FormDialog>
    </div>
  );
}

function PackForm({ ctx, item, onDone }: { ctx: ConfigContext; item?: Pack; onDone: () => void }) {
  const { t } = useTranslation('configuration');
  const componentList = components.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const form = useForm({
    resolver: zodResolver(packSchema(ctx.defaultLocale)),
    defaultValues: toPackValues(item, ctx.locales, ctx.uiLocale),
  });
  const composition = useFieldArray({ control: form.control, name: 'composition' });
  const create = packs.useCreate(ctx.establishmentId);
  const update = packs.useUpdate(ctx.establishmentId);
  const s = useCrudSubmit({
    form,
    item,
    create,
    update,
    toInput: toPackInput,
    onDone,
    aliases: packAliases,
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const loc = (x: Record<string, string>) => localize(x, ctx.uiLocale, ctx.defaultLocale);
  const compositionValues = useWatch({ control, name: 'composition' });
  const used = new Set(compositionValues.map((c) => c.component_code));
  const available = (componentList.data ?? []).filter((c) => !used.has(c.code));
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="pack-code" label={t('fields.code')} error={errors.code}>
          <Input {...fieldAria('pack-code', errors.code)} {...register('code')} />
        </Field>
        <Field id="pack-sort" label={t('fields.sort_order')} error={errors.sort_order}>
          <Input
            {...fieldAria('pack-sort', errors.sort_order)}
            inputMode="numeric"
            {...register('sort_order')}
          />
        </Field>
        <Field
          id="pack-vat"
          label={t('catalog.fields.vat_rate')}
          error={errors.vat_rate}
          hint={t('catalog.vatHint')}
        >
          <Input
            {...fieldAria('pack-vat', errors.vat_rate, true)}
            inputMode="decimal"
            {...register('vat_rate')}
          />
        </Field>
      </div>
      <I18nTextInput
        name="name"
        idPrefix="pack"
        label={t('fields.name')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
        register={register}
        errors={errors}
      />
      <I18nTextInput
        name="description"
        idPrefix="pack"
        label={t('fields.description')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
        register={register}
        errors={errors}
        multiline
      />
      <fieldset className="space-y-2 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium text-slate-800">
          {t('catalog.fields.composition')}
        </legend>
        {composition.fields.length === 0 && (
          <p className="text-sm text-slate-600">{t('catalog.compositionEmpty')}</p>
        )}
        <ul className="space-y-2">
          {composition.fields.map((f, i) => {
            const code = compositionValues[i]?.component_code ?? f.component_code;
            const cmp = componentList.data?.find((c) => c.code === code);
            const qtyErr = errorAt(errors, `composition.${i}.quantity`);
            const codeErr = errorAt(errors, `composition.${i}.component_code`);
            return (
              <li key={f.id} className="flex flex-wrap items-center gap-2">
                <ComponentIcon code={code} className="text-slate-700" />
                <span className="min-w-32 text-sm font-medium">
                  {cmp ? loc(cmp.name) || code : code}
                </span>
                <label htmlFor={`pack-c${i}-qty`} className="sr-only">
                  {t('catalog.quantityOf', { name: cmp ? loc(cmp.name) || code : code })}
                </label>
                <Input
                  {...fieldAria(`pack-c${i}-qty`, qtyErr)}
                  className="w-20"
                  inputMode="numeric"
                  {...register(`composition.${i}.quantity`)}
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => composition.remove(i)}
                  aria-label={t('catalog.removeComponent', { name: code })}
                >
                  ×
                </Button>
                <FieldError id={`pack-c${i}-qty`} error={qtyErr} />
                <FieldError id={`pack-c${i}-code`} error={codeErr} />
              </li>
            );
          })}
        </ul>
        <FieldError id="pack-composition" error={errorAt(errors, 'composition')} />
        {available.length > 0 ? (
          <div
            className="flex flex-wrap gap-2 pt-1"
            role="group"
            aria-label={t('catalog.addToComposition')}
          >
            {available.map((c) => (
              <Button
                key={c.id}
                variant="outline"
                size="sm"
                onClick={() => composition.append({ component_code: c.code, quantity: '1' })}
              >
                <ComponentIcon code={c.code} />
                {loc(c.name) || c.code}
              </Button>
            ))}
          </div>
        ) : (
          componentList.data?.length === 0 && (
            <p className="text-sm text-amber-800">{t('catalog.noComponents')}</p>
          )
        )}
      </fieldset>
      <MultiCheck
        control={control}
        name="allowed_formulas"
        id="pack-formulas"
        legend={t('catalog.fields.allowed_formulas')}
        options={FORMULAS.map((f) => ({ value: f, label: t(`formulas.${f}`) }))}
      />
      <MultiCheck
        control={control}
        name="channels"
        id="pack-channels"
        legend={t('catalog.fields.channels')}
        options={CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) }))}
      />
      <MultiCheck
        control={control}
        name="requires_spot_kind"
        id="pack-spot-kinds"
        legend={t('catalog.fields.requires_spot_kind')}
        hint={t('catalog.allIfEmpty')}
        options={SPOT_KINDS.map((k) => ({ value: k, label: t(`spotKinds.${k}`) }))}
      />
      {(zoneList.data?.length ?? 0) > 0 && (
        <MultiCheck
          control={control}
          name="allowed_zone_ids"
          id="pack-zones"
          legend={t('catalog.fields.allowed_zone_ids')}
          hint={t('catalog.allIfEmpty')}
          options={(zoneList.data ?? []).map((z) => ({
            value: z.id,
            label: loc(z.name) || z.code,
          }))}
        />
      )}
      <div className="grid gap-4 sm:grid-cols-3">
        <Field
          id="pack-max"
          label={t('catalog.fields.max_per_booking')}
          error={errors.max_per_booking}
        >
          <Input
            {...fieldAria('pack-max', errors.max_per_booking)}
            inputMode="numeric"
            {...register('max_per_booking')}
          />
        </Field>
        <Field
          id="pack-image"
          label={t('catalog.fields.image')}
          error={errors.image}
          className="sm:col-span-2"
        >
          <Input
            {...fieldAria('pack-image', errors.image)}
            type="url"
            placeholder="https://"
            {...register('image')}
          />
        </Field>
      </div>
      <div className="flex flex-wrap gap-6">
        <Checkbox id="pack-active" label={t('catalog.fields.active')} {...register('active')} />
        <Checkbox
          id="pack-approval"
          label={t('catalog.fields.requires_approval')}
          {...register('requires_approval')}
        />
      </div>
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}

// ---- Extras --------------------------------------------------------------------------

function ExtrasTab({ ctx }: { ctx: ConfigContext }) {
  const { t } = useTranslation('configuration');
  const list = extras.useList(ctx.establishmentId);
  const remove = extras.useRemove(ctx.establishmentId);
  const editor = useEditor<Extra>();
  const readOnly = !ctx.canConfigure;
  const items = [...(list.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const loc = (x: Record<string, string>) => localize(x, ctx.uiLocale, ctx.defaultLocale);
  return (
    <div className="space-y-4 pt-4">
      <div className="flex items-start justify-between gap-2">
        <p className="text-sm text-slate-600">{t('catalog.extrasHint')}</p>
        {!readOnly && <Button onClick={editor.create}>{t('catalog.addExtra')}</Button>}
      </div>
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('catalog.emptyExtras')}</EmptyState>}
      {items.length > 0 && (
        <Card>
          <ul className="divide-y divide-slate-100">
            {items.map((x) => {
              const name = loc(x.name) || x.code;
              return (
                <li key={x.id} className="flex flex-wrap items-center gap-3 px-4 py-2">
                  {x.component_code && (
                    <ComponentIcon code={x.component_code} className="text-slate-700" />
                  )}
                  <span className="font-medium">{name}</span>
                  <span className="font-mono text-xs text-slate-500">{x.code}</span>
                  <Badge>{t(`catalog.pricingUnits.${x.pricing_unit}`)}</Badge>
                  <Badge tone="blue">{t(`catalog.attachTo.${x.attach_to}`)}</Badge>
                  {!x.active && <Badge tone="amber">{t('catalog.inactive')}</Badge>}
                  <span className="ml-auto">
                    <RowActions
                      readOnly={readOnly}
                      name={name}
                      onEdit={() => editor.edit(x)}
                      onDelete={() => remove.mutateAsync({ id: x.id, version: x.version })}
                    />
                  </span>
                </li>
              );
            })}
          </ul>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('catalog.editExtra') : t('catalog.addExtra')}
      >
        <ExtraForm key={editor.key} ctx={ctx} item={editor.item} onDone={editor.close} />
      </FormDialog>
    </div>
  );
}

function ExtraForm({
  ctx,
  item,
  onDone,
}: {
  ctx: ConfigContext;
  item?: Extra;
  onDone: () => void;
}) {
  const { t } = useTranslation('configuration');
  const componentList = components.useList(ctx.establishmentId);
  const packList = packs.useList(ctx.establishmentId);
  const form = useForm({
    resolver: zodResolver(extraSchema(ctx.defaultLocale)),
    defaultValues: toExtraValues(item, ctx.locales, ctx.uiLocale),
  });
  const create = extras.useCreate(ctx.establishmentId);
  const update = extras.useUpdate(ctx.establishmentId);
  const s = useCrudSubmit({
    form,
    item,
    create,
    update,
    toInput: toExtraInput,
    onDone,
    aliases: extraAliases,
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const attachTo = useWatch({ control, name: 'attach_to' });
  const loc = (x: Record<string, string>) => localize(x, ctx.uiLocale, ctx.defaultLocale);
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="ex-code" label={t('fields.code')} error={errors.code}>
          <Input {...fieldAria('ex-code', errors.code)} {...register('code')} />
        </Field>
        <Field id="ex-unit" label={t('catalog.fields.pricing_unit')} error={errors.pricing_unit}>
          <Select {...fieldAria('ex-unit', errors.pricing_unit)} {...register('pricing_unit')}>
            {PRICING_UNITS.map((u) => (
              <option key={u} value={u}>
                {t(`catalog.pricingUnits.${u}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="ex-attach" label={t('catalog.fields.attach_to')} error={errors.attach_to}>
          <Select {...fieldAria('ex-attach', errors.attach_to)} {...register('attach_to')}>
            {ATTACH_TO.map((a) => (
              <option key={a} value={a}>
                {t(`catalog.attachTo.${a}`)}
              </option>
            ))}
          </Select>
        </Field>
        <Field
          id="ex-component"
          label={t('catalog.fields.component_code')}
          error={errors.component_code}
          hint={t('catalog.extraComponentHint')}
        >
          <Select
            {...fieldAria('ex-component', errors.component_code, true)}
            {...register('component_code')}
          >
            <option value="">{t('catalog.noComponent')}</option>
            {(componentList.data ?? [])
              .filter((c) => c.tracked)
              .map((c) => (
                <option key={c.id} value={c.code}>
                  {loc(c.name) || c.code}
                </option>
              ))}
          </Select>
        </Field>
        <Field id="ex-max" label={t('catalog.fields.max_quantity')} error={errors.max_quantity}>
          <Input
            {...fieldAria('ex-max', errors.max_quantity)}
            inputMode="numeric"
            {...register('max_quantity')}
          />
        </Field>
        <Field
          id="ex-vat"
          label={t('catalog.fields.vat_rate')}
          error={errors.vat_rate}
          hint={t('catalog.vatHint')}
        >
          <Input
            {...fieldAria('ex-vat', errors.vat_rate, true)}
            inputMode="decimal"
            {...register('vat_rate')}
          />
        </Field>
      </div>
      <I18nTextInput
        name="name"
        idPrefix="ex"
        label={t('fields.name')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
        register={register}
        errors={errors}
      />
      <I18nTextInput
        name="description"
        idPrefix="ex"
        label={t('fields.description')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
        register={register}
        errors={errors}
        multiline
      />
      <MultiCheck
        control={control}
        name="channels"
        id="ex-channels"
        legend={t('catalog.fields.channels')}
        options={CHANNELS.map((c) => ({ value: c, label: t(`channels.${c}`) }))}
      />
      {attachTo === 'line' && (packList.data?.length ?? 0) > 0 && (
        <MultiCheck
          control={control}
          name="allowed_pack_ids"
          id="ex-packs"
          legend={t('catalog.fields.allowed_pack_ids')}
          hint={t('catalog.allIfEmpty')}
          options={(packList.data ?? []).map((p) => ({
            value: p.id,
            label: loc(p.name) || p.code,
          }))}
        />
      )}
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="ex-sort" label={t('fields.sort_order')} error={errors.sort_order}>
          <Input
            {...fieldAria('ex-sort', errors.sort_order)}
            inputMode="numeric"
            {...register('sort_order')}
          />
        </Field>
      </div>
      <Checkbox id="ex-active" label={t('catalog.fields.active')} {...register('active')} />
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
