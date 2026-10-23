import { Button } from '@arenile/shared/ui/button';
import { FieldDescription, FieldLegend, FieldSet } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  FormActions,
  FormDialog,
  QueryState,
  RowActions,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { BoxesIcon, LockIcon, PlusIcon, XIcon } from 'lucide-react';
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
import { ComponentIcon } from '../components/ComponentIcon';
import { ConfigPage } from '../components/ConfigPage';
import {
  DateRangeField,
  Field,
  FieldError,
  I18nTextField,
  SelectField,
} from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { errorAt, fieldAria } from '../lib/aria';
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
  const poolName = (p: Pool) => loc(p.name) || p.component_code;
  const zoneName = (id?: string) => {
    const z = zoneList.data?.find((x) => x.id === id);
    return z ? loc(z.name) || z.code : '—';
  };

  const columns: ColumnDef<Pool>[] = [
    {
      id: 'name',
      accessorFn: poolName,
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => (
        <span className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{poolName(row.original)}</span>
          {row.original.system_managed && (
            <StatusBadge tone="neutral">{t('pools.systemManaged')}</StatusBadge>
          )}
        </span>
      ),
    },
    {
      accessorKey: 'component_code',
      meta: { label: t('pools.fields.component_code') },
      header: t('pools.fields.component_code'),
      cell: ({ row }) => (
        <span className="inline-flex items-center gap-1.5">
          <ComponentIcon code={row.original.component_code} className="text-muted-foreground" />
          <span className="font-mono text-xs">{row.original.component_code}</span>
        </span>
      ),
    },
    {
      id: 'zone',
      accessorFn: (p) => zoneName(p.zone_id),
      meta: { label: t('pools.fields.zone_id') },
      header: t('pools.fields.zone_id'),
    },
    {
      accessorKey: 'capacity',
      meta: { label: t('pools.fields.capacity'), align: 'right' },
      header: t('pools.fields.capacity'),
    },
    {
      id: 'overrides',
      meta: { label: t('pools.overrides') },
      header: t('pools.overrides'),
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {(row.original.capacity_overrides ?? []).map((o) => (
            <span key={`${o.from}-${o.to}`} className="block">
              {formatIsoDate(o.from)} – {formatIsoDate(o.to)}: {o.capacity}
            </span>
          ))}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const p = row.original;
        if (p.system_managed) {
          return (
            <span className="flex items-center justify-end gap-1 text-xs text-muted-foreground">
              <LockIcon aria-hidden="true" className="size-3" />
              {t('pools.systemHint')}
            </span>
          );
        }
        return readOnly ? null : (
          <RowActions
            editLabel={t('pools.editAria', { name: poolName(p) })}
            deleteLabel={t('pools.deleteAria', { name: poolName(p) })}
            deleteMessage={t('pools.deleteConfirm', { name: poolName(p) })}
            onEdit={() => editor.edit(p)}
            onDelete={() => remove.mutateAsync({ id: p.id, version: p.version })}
          />
        );
      },
    },
  ];

  const addButton = !readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('pools.add')}
    </Button>
  );

  return (
    <ConfigPage
      title={t('pools.title')}
      description={t('pools.description')}
      readOnly={readOnly}
      actions={addButton}
    >
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('pools.title')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(p) => p.id}
          pageSize={0}
          empty={<EmptyState icon={BoxesIcon} title={t('pools.empty')} bordered={false} />}
        />
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
    control,
    formState: { errors },
  } = form;
  const loc = (text: Record<string, string>) => localize(text, uiLocale, defaultLocale);
  return (
    <form noValidate onSubmit={s.submit} className="space-y-6">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-5 sm:grid-cols-3">
        <SelectField
          control={control}
          name="component_code"
          id="pool-component"
          label={t('pools.fields.component_code')}
          placeholder={t('choose')}
          options={componentItems.map((c) => ({
            value: c.code,
            label: `${loc(c.name) || c.code} (${c.code})`,
            icon: <ComponentIcon code={c.code} />,
          }))}
        />
        <SelectField
          control={control}
          name="zone_id"
          id="pool-zone"
          label={t('pools.fields.zone_id')}
          options={[
            { value: '', label: t('pools.noZone') },
            ...zoneItems.map((z) => ({ value: z.id, label: loc(z.name) || z.code })),
          ]}
        />
        <Field id="pool-capacity" label={t('pools.fields.capacity')} error={errors.capacity}>
          <Input
            {...fieldAria('pool-capacity', errors.capacity)}
            inputMode="numeric"
            {...register('capacity')}
          />
        </Field>
      </div>
      <I18nTextField
        control={control}
        name="name"
        id="pool-name"
        label={t('fields.name')}
        locales={locales}
        defaultLocale={defaultLocale}
      />
      <FieldSet className="gap-3">
        <FieldLegend variant="label">{t('pools.overrides')}</FieldLegend>
        <FieldDescription>{t('pools.overridesHint')}</FieldDescription>
        {overrides.fields.map((f, i) => {
          const capErr = errorAt(errors, `capacity_overrides.${i}.capacity`);
          return (
            <div key={f.id} className="flex flex-wrap items-start gap-2 rounded-lg border p-3">
              <DateRangeField
                control={control}
                fromName={`capacity_overrides.${i}.from`}
                toName={`capacity_overrides.${i}.to`}
                id={`pool-o${i}`}
                label={t('pools.overrideRange', { index: i + 1 })}
              />
              <div className="w-28 space-y-1">
                <Input
                  {...fieldAria(`pool-o${i}-cap`, capErr)}
                  aria-label={t('pools.overrideCapacity', { index: i + 1 })}
                  placeholder={t('pools.fields.capacity')}
                  inputMode="numeric"
                  {...register(`capacity_overrides.${i}.capacity`)}
                />
                <FieldError id={`pool-o${i}-cap`} error={capErr} />
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="ml-auto"
                onClick={() => overrides.remove(i)}
                aria-label={t('pools.removeOverride', { index: i + 1 })}
              >
                <XIcon aria-hidden="true" />
              </Button>
            </div>
          );
        })}
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => overrides.append({ from: '', to: '', capacity: '' })}
          >
            <PlusIcon aria-hidden="true" />
            {t('pools.addOverride')}
          </Button>
        </div>
      </FieldSet>
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
