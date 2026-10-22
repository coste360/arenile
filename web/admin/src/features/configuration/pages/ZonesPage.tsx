import { Button, Card, Input } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { maps, zones, type BeachMap, type Zone } from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { Badge, ConfigPage, EmptyState, QueryState } from '../components/ConfigPage';
import { ConfirmDelete, FormActions, FormDialog } from '../components/dialogs';
import { Checkbox, Field, Select } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { I18nTextInput } from '../components/I18nTextInput';
import { fieldAria } from '../lib/aria';
import { localize } from '../lib/i18nText';
import {
  ASSIGNMENT_MODES,
  QUOTA_TYPES,
  toZoneInput,
  toZoneValues,
  zoneAliases,
  zoneSchema,
} from './inventory.schema';

/** Configurazione → Zone: assignment mode, online visibility and quota, price category. */
export function ZonesPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const list = zones.useList(ctx.establishmentId);
  const mapList = maps.useList(ctx.establishmentId);
  const remove = zones.useRemove(ctx.establishmentId);
  const editor = useEditor<Zone>();
  const readOnly = !ctx.canConfigure;
  const items = [...(list.data ?? [])].sort((a, b) => a.sort_order - b.sort_order);
  const name = (z: Zone) => localize(z.name, ctx.uiLocale, ctx.defaultLocale) || z.code;
  const mapName = (id?: string) => {
    const m = mapList.data?.find((x) => x.id === id);
    return m ? localize(m.name, ctx.uiLocale, ctx.defaultLocale) : '—';
  };

  return (
    <ConfigPage
      title={t('zones.title')}
      description={t('zones.description')}
      readOnly={readOnly}
      actions={!readOnly && <Button onClick={editor.create}>{t('zones.add')}</Button>}
    >
      <QueryState
        isLoading={list.isPending}
        error={list.error}
        onRetry={() => void list.refetch()}
      />
      {list.data && items.length === 0 && <EmptyState>{t('zones.empty')}</EmptyState>}
      {items.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[44rem] text-sm">
            <caption className="sr-only">{t('zones.title')}</caption>
            <thead className="bg-slate-50 text-left text-slate-600">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('fields.name')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('zones.fields.assignment_mode')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('zones.fields.price_category')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('zones.online')}
                </th>
                <th scope="col" className="px-4 py-2 font-medium">
                  {t('zones.fields.map_id')}
                </th>
                <th scope="col" className="px-4 py-2 text-right font-medium">
                  {t('zones.spots')}
                </th>
                <th scope="col" className="px-4 py-2">
                  <span className="sr-only">{t('actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((z) => (
                <tr key={z.id} className="border-t border-slate-100">
                  <th scope="row" className="px-4 py-2 text-left font-medium">
                    {name(z)}
                    <span className="ml-2 font-mono text-xs text-slate-500">{z.code}</span>
                  </th>
                  <td className="px-4 py-2">
                    <Badge tone="blue">{t(`zones.modes.${z.assignment_mode}`)}</Badge>
                    {z.auto_assign && (
                      <span className="ml-1 text-xs text-slate-600">
                        {t('zones.autoAssignShort')}
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-2 font-mono text-xs">{z.price_category}</td>
                  <td className="px-4 py-2">
                    {z.online_visible ? (
                      <Badge tone="green">
                        {z.online_quota
                          ? t(`zones.quota.${z.online_quota.type}`, { value: z.online_quota.value })
                          : t('zones.onlineAll')}
                      </Badge>
                    ) : (
                      <Badge>{t('zones.deskOnly')}</Badge>
                    )}
                  </td>
                  <td className="px-4 py-2">{mapName(z.map_id)}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{z.spot_count}</td>
                  <td className="px-4 py-2 text-right">
                    {!readOnly && (
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => editor.edit(z)}
                          aria-label={t('zones.editAria', { name: name(z) })}
                        >
                          {t('common.actions.edit', { ns: 'translation' })}
                        </Button>
                        <ConfirmDelete
                          label={t('zones.deleteAria', { name: name(z) })}
                          message={t('zones.deleteConfirm', { name: name(z) })}
                          onConfirm={() => remove.mutateAsync({ id: z.id, version: z.version })}
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
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('zones.editTitle', { name: name(editor.item) }) : t('zones.add')}
      >
        <ZoneForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          item={editor.item}
          maps={mapList.data ?? []}
          count={items.length}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          uiLocale={ctx.uiLocale}
          onDone={editor.close}
        />
      </FormDialog>
    </ConfigPage>
  );
}

function ZoneForm({
  establishmentId,
  item,
  maps: mapItems,
  count,
  locales,
  defaultLocale,
  uiLocale,
  onDone,
}: {
  establishmentId: string;
  item?: Zone;
  maps: BeachMap[];
  count: number;
  locales: string[];
  defaultLocale: string;
  uiLocale: string;
  onDone: () => void;
}) {
  const { t } = useTranslation('configuration');
  const form = useForm({
    resolver: zodResolver(zoneSchema(defaultLocale)),
    defaultValues: toZoneValues(item, locales, count),
  });
  const create = zones.useCreate(establishmentId);
  const update = zones.useUpdate(establishmentId);
  const s = useCrudSubmit({
    form,
    item,
    create,
    update,
    toInput: toZoneInput,
    onDone,
    aliases: zoneAliases,
  });
  const {
    register,
    formState: { errors },
  } = form;
  const mode = useWatch({ control: form.control, name: 'assignment_mode' });
  const online = useWatch({ control: form.control, name: 'online_visible' });
  const quotaType = useWatch({ control: form.control, name: 'quota_type' });
  return (
    <form noValidate onSubmit={s.submit} className="space-y-4">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field id="zone-code" label={t('fields.code')} error={errors.code}>
          <Input {...fieldAria('zone-code', errors.code)} {...register('code')} />
        </Field>
        <Field
          id="zone-price_category"
          label={t('zones.fields.price_category')}
          error={errors.price_category}
          hint={t('zones.priceCategoryHint')}
        >
          <Input
            {...fieldAria('zone-price_category', errors.price_category, true)}
            {...register('price_category')}
          />
        </Field>
        <Field id="zone-sort" label={t('fields.sort_order')} error={errors.sort_order}>
          <Input
            {...fieldAria('zone-sort', errors.sort_order)}
            inputMode="numeric"
            {...register('sort_order')}
          />
        </Field>
      </div>
      <I18nTextInput
        name="name"
        idPrefix="zone"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
        register={register}
        errors={errors}
      />
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-slate-800">
          {t('zones.fields.assignment_mode')}
        </legend>
        {ASSIGNMENT_MODES.map((m) => (
          <label key={m} htmlFor={`zone-mode-${m}`} className="flex items-start gap-2 text-sm">
            <input
              id={`zone-mode-${m}`}
              type="radio"
              value={m}
              className="mt-0.5 accent-[var(--ar-primary)]"
              {...register('assignment_mode')}
            />
            <span>
              <span className="font-medium">{t(`zones.modes.${m}`)}</span>
              <span className="block text-xs text-slate-600">{t(`zones.modeHints.${m}`)}</span>
            </span>
          </label>
        ))}
      </fieldset>
      {mode !== 'quantity' && (
        <Field id="zone-map" label={t('zones.fields.map_id')} error={errors.map_id}>
          <Select {...fieldAria('zone-map', errors.map_id)} {...register('map_id')}>
            <option value="">{t('zones.noMap')}</option>
            {mapItems.map((m) => (
              <option key={m.id} value={m.id}>
                {localize(m.name, uiLocale, defaultLocale)}
              </option>
            ))}
          </Select>
        </Field>
      )}
      {mode === 'zone' && (
        <Checkbox
          id="zone-auto_assign"
          label={t('zones.fields.auto_assign')}
          description={t('zones.autoAssignHint')}
          {...register('auto_assign')}
        />
      )}
      <fieldset className="space-y-3 rounded-md border border-slate-200 p-3">
        <legend className="px-1 text-sm font-medium text-slate-800">{t('zones.online')}</legend>
        <Checkbox
          id="zone-online_visible"
          label={t('zones.fields.online_visible')}
          {...register('online_visible')}
        />
        {online && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              id="zone-quota_type"
              label={t('zones.fields.quota_type')}
              error={errors.quota_type}
            >
              <Select
                {...fieldAria('zone-quota_type', errors.quota_type)}
                {...register('quota_type')}
              >
                {QUOTA_TYPES.map((q) => (
                  <option key={q} value={q}>
                    {t(`zones.quotaTypes.${q}`)}
                  </option>
                ))}
              </Select>
            </Field>
            {quotaType !== 'none' && (
              <Field
                id="zone-quota_value"
                label={t('zones.fields.quota_value')}
                error={errors.quota_value}
                hint={t(`zones.quotaHints.${quotaType}`)}
              >
                <Input
                  {...fieldAria('zone-quota_value', errors.quota_value, true)}
                  inputMode="numeric"
                  {...register('quota_value')}
                />
              </Field>
            )}
          </div>
        )}
      </fieldset>
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
