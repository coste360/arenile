import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
  Field as UiField,
} from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { RadioGroup, RadioGroupItem } from '@arenile/shared/ui/radio-group';
import {
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { LayersIcon, MapIcon, PlusIcon } from 'lucide-react';
import { useController, useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { maps, zones, type BeachMap, type Zone } from '../api/resources';
import { useConfigContext } from '../api/useConfigContext';
import { useCrudSubmit, useEditor } from '../api/useCrud';
import { ConfigPage, QueryState } from '../components/ConfigPage';
import { FormActions, FormDialog, RowActions } from '../components/dialogs';
import { BooleanField, Field, I18nTextField, SelectField } from '../components/fields';
import { FormAlert } from '../components/FormAlert';
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

  const columns: ColumnDef<Zone>[] = [
    {
      id: 'name',
      accessorFn: (z) => name(z),
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => (
        <span className="flex flex-col">
          <span className="font-medium">{name(row.original)}</span>
          <span className="font-mono text-xs text-muted-foreground">{row.original.code}</span>
        </span>
      ),
    },
    {
      accessorKey: 'assignment_mode',
      meta: { label: t('zones.fields.assignment_mode') },
      header: t('zones.fields.assignment_mode'),
      cell: ({ row }) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <StatusBadge tone="info">{t(`zones.modes.${row.original.assignment_mode}`)}</StatusBadge>
          {row.original.auto_assign && (
            <span className="text-xs text-muted-foreground">{t('zones.autoAssignShort')}</span>
          )}
        </span>
      ),
    },
    {
      accessorKey: 'price_category',
      meta: { label: t('zones.fields.price_category') },
      header: t('zones.fields.price_category'),
      cell: ({ row }) => (
        <Badge variant="outline" className="font-mono">
          {row.original.price_category}
        </Badge>
      ),
    },
    {
      id: 'online',
      meta: { label: t('zones.online') },
      header: t('zones.online'),
      cell: ({ row }) => {
        const z = row.original;
        return z.online_visible ? (
          <StatusBadge tone="success">
            {z.online_quota
              ? t(`zones.quota.${z.online_quota.type}`, { value: z.online_quota.value })
              : t('zones.onlineAll')}
          </StatusBadge>
        ) : (
          <StatusBadge>{t('zones.deskOnly')}</StatusBadge>
        );
      },
    },
    {
      id: 'map',
      accessorFn: (z) => mapName(z.map_id),
      meta: { label: t('zones.fields.map_id') },
      header: t('zones.fields.map_id'),
    },
    {
      accessorKey: 'spot_count',
      meta: { label: t('zones.spots') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('zones.spots')} className="justify-end" />
      ),
      cell: ({ row }) => (
        <span className="block text-right tabular-nums">{row.original.spot_count}</span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) =>
        readOnly ? null : (
          <RowActions
            editLabel={t('zones.editAria', { name: name(row.original) })}
            deleteLabel={t('zones.deleteAria', { name: name(row.original) })}
            deleteMessage={t('zones.deleteConfirm', { name: name(row.original) })}
            onEdit={() => editor.edit(row.original)}
            onDelete={() =>
              remove.mutateAsync({ id: row.original.id, version: row.original.version })
            }
          />
        ),
    },
  ];

  const addButton = !readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('zones.add')}
    </Button>
  );

  return (
    <ConfigPage
      title={t('zones.title')}
      description={t('zones.description')}
      readOnly={readOnly}
      actions={addButton}
    >
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('zones.title')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(z) => z.id}
          searchable
          pageSize={0}
          empty={<EmptyState icon={LayersIcon} title={t('zones.empty')} bordered={false} />}
        />
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
    control,
    formState: { errors },
  } = form;
  const modeField = useController({ control, name: 'assignment_mode' }).field;
  const mode = useWatch({ control, name: 'assignment_mode' });
  const online = useWatch({ control, name: 'online_visible' });
  const quotaType = useWatch({ control, name: 'quota_type' });
  return (
    <form noValidate onSubmit={s.submit} className="space-y-6">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-5 sm:grid-cols-3">
        <Field id="zone-code" label={t('fields.code')} error={errors.code}>
          <Input
            {...fieldAria('zone-code', errors.code)}
            className="font-mono"
            {...register('code')}
          />
        </Field>
        <Field
          id="zone-price_category"
          label={t('zones.fields.price_category')}
          error={errors.price_category}
          hint={t('zones.priceCategoryHint')}
        >
          <Input
            {...fieldAria('zone-price_category', errors.price_category, true)}
            className="font-mono"
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
      <I18nTextField
        control={control}
        name="name"
        id="zone-name"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
      />
      <FieldSet>
        <FieldLegend variant="label">{t('zones.fields.assignment_mode')}</FieldLegend>
        <RadioGroup
          value={modeField.value}
          onValueChange={modeField.onChange}
          className="grid gap-3 sm:grid-cols-3"
        >
          {ASSIGNMENT_MODES.map((m) => (
            <FieldLabel key={m} htmlFor={`zone-mode-${m}`}>
              <UiField orientation="horizontal">
                <FieldContent>
                  <FieldTitle>{t(`zones.modes.${m}`)}</FieldTitle>
                  <FieldDescription>{t(`zones.modeHints.${m}`)}</FieldDescription>
                </FieldContent>
                <RadioGroupItem value={m} id={`zone-mode-${m}`} />
              </UiField>
            </FieldLabel>
          ))}
        </RadioGroup>
      </FieldSet>
      {mode !== 'quantity' && (
        <SelectField
          control={control}
          name="map_id"
          id="zone-map"
          label={t('zones.fields.map_id')}
          className="sm:max-w-sm"
          options={[
            { value: '', label: t('zones.noMap') },
            ...mapItems.map((m) => ({
              value: m.id,
              label: localize(m.name, uiLocale, defaultLocale),
              icon: <MapIcon aria-hidden="true" />,
            })),
          ]}
        />
      )}
      {mode === 'zone' && (
        <BooleanField
          control={control}
          name="auto_assign"
          id="zone-auto_assign"
          variant="switch"
          label={t('zones.fields.auto_assign')}
          description={t('zones.autoAssignHint')}
        />
      )}
      <FieldSet className="gap-4 rounded-lg border p-4">
        <FieldLegend variant="label" className="px-1">
          {t('zones.online')}
        </FieldLegend>
        <BooleanField
          control={control}
          name="online_visible"
          id="zone-online_visible"
          variant="switch"
          className="border-0 p-0"
          label={t('zones.fields.online_visible')}
        />
        {online && (
          <div className="grid gap-5 sm:grid-cols-2">
            <SelectField
              control={control}
              name="quota_type"
              id="zone-quota_type"
              label={t('zones.fields.quota_type')}
              options={QUOTA_TYPES.map((q) => ({ value: q, label: t(`zones.quotaTypes.${q}`) }))}
            />
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
      </FieldSet>
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
