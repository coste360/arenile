import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useFieldArray, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  components,
  pools,
  zones,
  type CatalogComponent,
  type Pool,
  type Zone,
} from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { Badge, ConfigPage, EmptyState, QueryState } from '../components/ConfigPage';
import { ComponentIcon } from '../components/ComponentIcon';
import { ConfirmDelete, FormActions, FormDialog } from '../components/dialogs';
import { Field, FieldError, Select } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { I18nTextInput } from '../components/I18nTextInput';
import { errorAt, fieldAria } from '../lib/aria';
import { formatDate } from '../lib/dates';
import { localize } from '../lib/i18nText';
import { poolSchema, toPoolInput, toPoolValues } from './inventory.schema';

/**
 * Configurazione → Pool: position-less quantity inventory (spec 7.5). Pools
 * managed by the system (implicit pool of a `zone` zone) are read-only.
 */
export function PoolsPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const list = pools.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const componentList = components.useList(ctx.establishmentId);
  const remove = pools.useRemove(ctx.establishmentId);
  const editor = useEditor<Pool>();
  const readOnly = !ctx.canConfigure;
  const items = list.data ?? [];
  const loc = (text: Record<string, string>) => localize(text, ctx.uiLocale, ctx.defaultLocale);
  const zoneName = (id?: string) => {
    const z = zoneList.data?.find((x) => x.id === id);
    return z ? loc(z.name) || z.code : '—';
  };

  return (
    <ConfigPage
      title={t('pools.title')}
      description={t('pools.description')}
      readOnly={readOnly}
      actions={!readOnly && <Button onClick={editor.create}>{t('pools.add')}</Button>}
    >
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('pools.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[40rem] text-sm">
            <caption className="sr-only">{t('pools.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('fields.name')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('pools.fields.component_code')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('pools.fields.zone_id')}
                </th>
                <th scope="col" className="px-4 py-2 text-right font-medium">
                  {t('pools.fields.capacity')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('pools.overrides')}
                </th>
                <th scope="col" className="px-4 py-2">
                  <span className="sr-only">{t('actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((p) => {
                const name = loc(p.name) || p.component_code;
                return (
                  <tr key={p.id} className="border-t border-slate-100">
                    <th scope="row" className="px-4 py-2 text-left font-medium">
                      {name}
                      {p.system_managed && (
                        <span className="ml-2">
                          <Badge>{t('pools.systemManaged')}</Badge>
                        </span>
                      )}
                    </th>
                    <td className="px-4 py-2">
                      <span className="inline-flex items-center gap-1">
                        <ComponentIcon code={p.component_code} />
                        <span className="font-mono text-xs">{p.component_code}</span>
                      </span>
                    </td>
                    <td className="px-4 py-2">{zoneName(p.zone_id)}</td>
                    <td className="px-4 py-2 text-right tabular-nums">{p.capacity}</td>
                    <td className="px-4 py-2 text-xs text-slate-700">
                      {(p.capacity_overrides ?? []).map((o) => (
                        <span key={`${o.from}-${o.to}`} className="block">
                          {formatDate(o.from, ctx.uiLocale)} – {formatDate(o.to, ctx.uiLocale)}:{' '}
                          {o.capacity}
                        </span>
                      ))}
                    </td>
                    <td className="px-4 py-2 text-right">
                      {!readOnly && !p.system_managed && (
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => editor.edit(p)}
                            aria-label={t('pools.editAria', { name })}
                          >
                            {t('common.actions.edit', { ns: 'translation' })}
                          </Button>
                          <ConfirmDelete
                            label={t('pools.deleteAria', { name })}
                            message={t('pools.deleteConfirm', { name })}
                            onConfirm={() => remove.mutateAsync({ id: p.id, version: p.version })}
                          />
                        </div>
                      )}
                      {p.system_managed && (
                        <span className="text-xs text-slate-500">{t('pools.systemHint')}</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </Card>
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('pools.editTitle') : t('pools.add')}
      >
        <PoolForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          item={editor.item}
          zones={(zoneList.data ?? []).filter((z) => z.assignment_mode === 'quantity')}
          components={componentList.data ?? []}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          uiLocale={ctx.uiLocale}
          onDone={editor.close}
        />
      </FormDialog>
    </ConfigPage>
  );
}

function PoolForm({
  establishmentId,
  item,
  zones: zoneItems,
  components: componentItems,
  locales,
  defaultLocale,
  uiLocale,
  onDone,
}: {
  establishmentId: string;
  item?: Pool;
  zones: Zone[];
  components: CatalogComponent[];
  locales: string[];
  defaultLocale: string;
  uiLocale: string;
  onDone: () => void;
}) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(poolSchema(defaultLocale)),
    defaultValues: toPoolValues(item, locales),
  });
  const overrides = useFieldArray({ control: form.control, name: 'capacity_overrides' });
  const create = pools.useCreate(establishmentId);
  const update = pools.useUpdate(establishmentId);
  const s = useCrudSubmit({ form, item, create, update, toInput: toPoolInput, onDone });
  const {
    register,
    formState: { errors },
  } = form;
  const loc = (text: Record<string, string>) => localize(text, uiLocale, defaultLocale);
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field
          id="pool-component"
          label={t('pools.fields.component_code')}
          error={errors.component_code}
        >
          <Select
            {...fieldAria('pool-component', errors.component_code)}
            {...register('component_code')}
          >
            <option value="">{t('choose')}</option>
            {componentItems.map((c) => (
              <option key={c.id} value={c.code}>
                {loc(c.name) || c.code} ({c.code})
              </option>
            ))}
          </Select>
        </Field>
        <Field id="pool-zone" label={t('pools.fields.zone_id')} error={errors.zone_id}>
          <Select {...fieldAria('pool-zone', errors.zone_id)} {...register('zone_id')}>
            <option value="">{t('pools.noZone')}</option>
            {zoneItems.map((z) => (
              <option key={z.id} value={z.id}>
                {loc(z.name) || z.code}
              </option>
            ))}
          </Select>
        </Field>
        <Field id="pool-capacity" label={t('pools.fields.capacity')} error={errors.capacity}>
          <Input
            {...fieldAria('pool-capacity', errors.capacity)}
            inputMode="numeric"
            {...register('capacity')}
          />
        </Field>
      </div>
      <I18nTextInput
        name="name"
        idPrefix="pool"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
        register={register}
        errors={errors}
      />
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-slate-800">{t('pools.overrides')}</legend>
        <p className="text-xs text-slate-600">{t('pools.overridesHint')}</p>
        {overrides.fields.map((f, i) => {
          const errs = ['from', 'to', 'capacity'].map((k) =>
            errorAt(errors, `capacity_overrides.${i}.${k}`),
          );
          return (
            <div key={f.id} className="flex flex-wrap items-end gap-2">
              <div className="space-y-1">
                <label htmlFor={`pool-o${i}-from`} className="text-xs text-slate-600">
                  {t('periods.from')}
                </label>
                <Input
                  {...fieldAria(`pool-o${i}-from`, errs[0])}
                  type="date"
                  {...register(`capacity_overrides.${i}.from`)}
                />
              </div>
              <div className="space-y-1">
                <label htmlFor={`pool-o${i}-to`} className="text-xs text-slate-600">
                  {t('periods.to')}
                </label>
                <Input
                  {...fieldAria(`pool-o${i}-to`, errs[1])}
                  type="date"
                  {...register(`capacity_overrides.${i}.to`)}
                />
              </div>
              <div className="w-28 space-y-1">
                <label htmlFor={`pool-o${i}-cap`} className="text-xs text-slate-600">
                  {t('pools.fields.capacity')}
                </label>
                <Input
                  {...fieldAria(`pool-o${i}-cap`, errs[2])}
                  inputMode="numeric"
                  {...register(`capacity_overrides.${i}.capacity`)}
                />
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => overrides.remove(i)}
                aria-label={t('pools.removeOverride', { index: i + 1 })}
              >
                {t('common.actions.delete', { ns: 'translation' })}
              </Button>
              <div className="w-full">
                <FieldError id={`pool-o${i}-from`} error={errs[0]} />
                <FieldError id={`pool-o${i}-to`} error={errs[1]} />
                <FieldError id={`pool-o${i}-cap`} error={errs[2]} />
              </div>
            </div>
          );
        })}
        <Button
          variant="outline"
          size="sm"
          onClick={() => overrides.append({ from: '', to: '', capacity: '' })}
        >
          {t('pools.addOverride')}
        </Button>
      </fieldset>
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
