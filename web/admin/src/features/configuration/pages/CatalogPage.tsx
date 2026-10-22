import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { ButtonGroup } from '@arenile/shared/ui/button-group';
import { FieldDescription, FieldLegend, FieldSeparator, FieldSet } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@arenile/shared/ui/tabs';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  FormActions,
  FormDialog,
  FormSheet,
  QueryState,
  RowActions,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  BoxIcon,
  MinusIcon,
  PackageIcon,
  PlusIcon,
  PuzzleIcon,
  Trash2Icon,
  type LucideIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
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
import { ComponentIcon } from '../components/ComponentIcon';
import { ConfigPage } from '../components/ConfigPage';
import {
  BooleanField,
  Field,
  FieldError,
  I18nTextField,
  MultiCheck,
  SelectField,
  SuffixInput,
} from '../components/fields';
import { FormAlert } from '../components/FormAlert';
import { errorAt, fieldAria } from '../lib/aria';
import { localize } from '../lib/i18nText';
import { basisPointsToPercentText as basisPointsToPercentInput } from '@arenile/shared/lib/money';
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

const TABS: { id: 'components' | 'packs' | 'extras'; icon: LucideIcon }[] = [
  { id: 'components', icon: BoxIcon },
  { id: 'packs', icon: PackageIcon },
  { id: 'extras', icon: PuzzleIcon },
];

/** Configurazione → Catalogo: components, packs and extras (spec 5.3). */
export function CatalogPage() {
  const { t } = useTranslation('configuration');
  const ctx = useConfigContext();
  const [tab, setTab] = useState<string>('packs');
  return (
    <ConfigPage
      title={t('catalog.title')}
      description={t('catalog.description')}
      readOnly={!ctx.canConfigure}
    >
      <Tabs value={tab} onValueChange={setTab} className="gap-4">
        <TabsList aria-label={t('catalog.title')}>
          {TABS.map(({ id, icon: Icon }) => (
            <TabsTrigger key={id} value={id}>
              <Icon aria-hidden="true" />
              {t(`catalog.tabs.${id}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="components">
          <ComponentsTab ctx={ctx} />
        </TabsContent>
        <TabsContent value="packs">
          <PacksTab ctx={ctx} />
        </TabsContent>
        <TabsContent value="extras">
          <ExtrasTab ctx={ctx} />
        </TabsContent>
      </Tabs>
    </ConfigPage>
  );
}

/** Hint + "Nuovo" button above each tab's table. */
function TabToolbar({ hint, action }: { hint: string; action: ReactNode }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <p className="max-w-2xl text-sm text-muted-foreground">{hint}</p>
      {action}
    </div>
  );
}

function NameCell({ name, code, icon }: { name: string; code: string; icon?: ReactNode }) {
  return (
    <span className="flex items-center gap-3">
      {icon && (
        <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          {icon}
        </span>
      )}
      <span className="flex min-w-0 flex-col">
        <span className="font-medium">{name}</span>
        <span className="font-mono text-xs text-muted-foreground">{code}</span>
      </span>
    </span>
  );
}

function actionsColumn<T extends { id: string; version: number }>(
  t: (k: string, o?: Record<string, unknown>) => string,
  readOnly: boolean,
  nameOf: (x: T) => string,
  onEdit: (x: T) => void,
  onDelete: (x: T) => Promise<unknown>,
): ColumnDef<T> {
  return {
    id: 'actions',
    enableHiding: false,
    header: () => <span className="sr-only">{t('actions')}</span>,
    cell: ({ row }) =>
      readOnly ? null : (
        <RowActions
          editLabel={t('catalog.editAria', { name: nameOf(row.original) })}
          deleteLabel={t('catalog.deleteAria', { name: nameOf(row.original) })}
          deleteMessage={t('catalog.deleteConfirm', { name: nameOf(row.original) })}
          onEdit={() => onEdit(row.original)}
          onDelete={() => onDelete(row.original)}
        />
      ),
  };
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
  const nameOf = (c: CatalogComponent) => loc(c.name) || c.code;
  const columns: ColumnDef<CatalogComponent>[] = [
    {
      id: 'name',
      accessorFn: nameOf,
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => (
        <NameCell
          name={nameOf(row.original)}
          code={row.original.code}
          icon={<ComponentIcon code={row.original.code} />}
        />
      ),
    },
    {
      id: 'tracked',
      meta: { label: t('catalog.fields.tracked') },
      header: t('catalog.fields.tracked'),
      cell: ({ row }) => {
        const c = row.original;
        if (!c.tracked) return <span className="text-muted-foreground">—</span>;
        const pool = poolList.data?.find((p) => p.id === c.pool_id);
        return (
          <StatusBadge tone="info">
            {pool
              ? t('catalog.trackedIn', { pool: loc(pool.name) || pool.component_code })
              : t('catalog.tracked')}
          </StatusBadge>
        );
      },
    },
    actionsColumn(t, readOnly, nameOf, editor.edit, (c) =>
      remove.mutateAsync({ id: c.id, version: c.version }),
    ),
  ];
  const add = !readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('catalog.addComponent')}
    </Button>
  );
  return (
    <div className="space-y-4">
      <TabToolbar hint={t('catalog.componentsHint')} action={add} />
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('catalog.tabs.components')}
          columns={columns}
          data={list.data ?? []}
          loading={list.isPending}
          getRowId={(c) => c.id}
          searchable
          pageSize={0}
          empty={
            <EmptyState icon={BoxIcon} title={t('catalog.emptyComponents')} bordered={false} />
          }
        />
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
    control,
    formState: { errors },
  } = form;
  const code = useWatch({ control, name: 'code' });
  const tracked = useWatch({ control, name: 'tracked' });
  return (
    <form noValidate onSubmit={s.submit} className="space-y-5">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="flex items-start gap-4">
        <span className="mt-6 flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary-on-surface">
          <ComponentIcon code={code} className="size-5" />
        </span>
        <Field
          id="cmp-code"
          label={t('fields.code')}
          error={errors.code}
          hint={t('catalog.componentCodeHint')}
          className="flex-1"
        >
          <Input
            {...fieldAria('cmp-code', errors.code, true)}
            className="font-mono"
            {...register('code')}
          />
        </Field>
      </div>
      <I18nTextField
        control={control}
        name="name"
        id="cmp-name"
        label={t('fields.name')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
      />
      <BooleanField
        control={control}
        name="tracked"
        id="cmp-tracked"
        variant="switch"
        label={t('catalog.fields.tracked')}
        description={t('catalog.trackedHint')}
      />
      {tracked && (
        <SelectField
          control={control}
          name="pool_id"
          id="cmp-pool"
          label={t('catalog.fields.pool_id')}
          placeholder={t('choose')}
          options={(poolList.data ?? []).map((p) => ({
            value: p.id,
            label: localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.component_code,
          }))}
        />
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
  const nameOf = (p: Pack) => loc(p.name) || p.code;
  const columns: ColumnDef<Pack>[] = [
    {
      id: 'name',
      accessorFn: nameOf,
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => (
        <NameCell
          name={nameOf(row.original)}
          code={row.original.code}
          icon={<PackageIcon aria-hidden="true" className="size-4" />}
        />
      ),
    },
    {
      id: 'composition',
      meta: { label: t('catalog.fields.composition') },
      header: t('catalog.fields.composition'),
      cell: ({ row }) => {
        const entries = Object.entries(row.original.composition);
        return (
          <span
            role="img"
            className="flex flex-wrap items-center gap-1.5"
            aria-label={t('catalog.compositionAria', {
              list: entries.map(([c, q]) => `${q} × ${c}`).join(', '),
            })}
          >
            {entries.map(([code, qty]) => (
              <Badge key={code} variant="secondary" className="gap-1 tabular-nums">
                {qty}×
                <ComponentIcon code={code} className="size-3.5" />
              </Badge>
            ))}
          </span>
        );
      },
    },
    {
      id: 'flags',
      meta: { label: t('catalog.flags') },
      header: t('catalog.flags'),
      cell: ({ row }) => {
        const p = row.original;
        return (
          <span className="flex flex-wrap gap-1.5">
            {p.active ? (
              <StatusBadge tone="success">{t('catalog.fields.active')}</StatusBadge>
            ) : (
              <StatusBadge tone="warning">{t('catalog.inactive')}</StatusBadge>
            )}
            {p.requires_approval && (
              <StatusBadge tone="highlight">{t('catalog.fields.requires_approval')}</StatusBadge>
            )}
            {p.vat_rate_bp !== undefined && (
              <Badge variant="outline">
                {t('catalog.vatBadge', {
                  rate: basisPointsToPercentInput(p.vat_rate_bp),
                })}
              </Badge>
            )}
          </span>
        );
      },
    },
    actionsColumn(t, readOnly, nameOf, editor.edit, (p) =>
      remove.mutateAsync({ id: p.id, version: p.version }),
    ),
  ];
  const add = !readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('catalog.addPack')}
    </Button>
  );
  return (
    <div className="space-y-4">
      <TabToolbar hint={t('catalog.packsHint')} action={add} />
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('catalog.tabs.packs')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(p) => p.id}
          searchable
          pageSize={0}
          empty={<EmptyState icon={PackageIcon} title={t('catalog.emptyPacks')} bordered={false} />}
        />
      )}
      <FormSheet
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('catalog.editPack') : t('catalog.addPack')}
        description={t('catalog.packsHint')}
      >
        <PackForm key={editor.key} ctx={ctx} item={editor.item} onDone={editor.close} />
      </FormSheet>
    </div>
  );
}

function PackForm({ ctx, item, onDone }: { ctx: ConfigContext; item?: Pack; onDone: () => void }) {
  const { t } = useTranslation('configuration');
  const componentList = components.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const form = useForm({
    resolver: zodResolver(packSchema(ctx.defaultLocale)),
    defaultValues: toPackValues(item, ctx.locales),
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
    setValue,
    formState: { errors },
  } = form;
  const loc = (x: Record<string, string>) => localize(x, ctx.uiLocale, ctx.defaultLocale);
  const compositionValues = useWatch({ control, name: 'composition' });
  const used = new Set(compositionValues.map((c) => c.component_code));
  const available = (componentList.data ?? []).filter((c) => !used.has(c.code));
  const step = (i: number, delta: number) => {
    const current = Number.parseInt(compositionValues[i]?.quantity ?? '0', 10) || 0;
    setValue(`composition.${i}.quantity`, String(Math.max(1, current + delta)), {
      shouldDirty: true,
      shouldValidate: true,
    });
  };
  return (
    <form noValidate onSubmit={s.submit} className="space-y-6">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-5 sm:grid-cols-3">
        <Field id="pack-code" label={t('fields.code')} error={errors.code}>
          <Input
            {...fieldAria('pack-code', errors.code)}
            className="font-mono"
            {...register('code')}
          />
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
          <SuffixInput
            {...fieldAria('pack-vat', errors.vat_rate, true)}
            suffix="%"
            inputMode="decimal"
            {...register('vat_rate')}
          />
        </Field>
      </div>
      <I18nTextField
        control={control}
        name="name"
        id="pack-name"
        label={t('fields.name')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
      />
      <I18nTextField
        control={control}
        name="description"
        id="pack-description"
        label={t('fields.description')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
        multiline
      />
      <FieldSet className="gap-3 rounded-lg border p-4">
        <FieldLegend variant="label" className="px-1">
          {t('catalog.fields.composition')}
        </FieldLegend>
        {composition.fields.length === 0 && (
          <FieldDescription>{t('catalog.compositionEmpty')}</FieldDescription>
        )}
        <ul className="divide-y">
          {composition.fields.map((f, i) => {
            const code = compositionValues[i]?.component_code ?? f.component_code;
            const cmp = componentList.data?.find((c) => c.code === code);
            const label = cmp ? loc(cmp.name) || code : code;
            const qtyErr = errorAt(errors, `composition.${i}.quantity`);
            const codeErr = errorAt(errors, `composition.${i}.component_code`);
            return (
              <li key={f.id} className="flex flex-wrap items-center gap-3 py-2">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
                  <ComponentIcon code={code} />
                </span>
                <span className="min-w-28 flex-1 text-sm font-medium">{label}</span>
                <ButtonGroup>
                  <Button
                    variant="outline"
                    size="icon-sm"
                    aria-label={t('catalog.decrease', { name: label })}
                    onClick={() => step(i, -1)}
                  >
                    <MinusIcon aria-hidden="true" />
                  </Button>
                  <Input
                    {...fieldAria(`pack-c${i}-qty`, qtyErr)}
                    aria-label={t('catalog.quantityOf', { name: label })}
                    className="h-8 w-14 text-center tabular-nums"
                    inputMode="numeric"
                    {...register(`composition.${i}.quantity`)}
                  />
                  <Button
                    variant="outline"
                    size="icon-sm"
                    aria-label={t('catalog.increase', { name: label })}
                    onClick={() => step(i, 1)}
                  >
                    <PlusIcon aria-hidden="true" />
                  </Button>
                </ButtonGroup>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => composition.remove(i)}
                  aria-label={t('catalog.removeComponent', { name: label })}
                >
                  <Trash2Icon aria-hidden="true" />
                </Button>
                <div className="w-full empty:hidden">
                  <FieldError id={`pack-c${i}-qty`} error={qtyErr} />
                  <FieldError id={`pack-c${i}-code`} error={codeErr} />
                </div>
              </li>
            );
          })}
        </ul>
        <FieldError id="pack-composition" error={errorAt(errors, 'composition')} />
        {available.length > 0 ? (
          <div
            className="flex flex-wrap gap-2"
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
            <FieldDescription>{t('catalog.noComponents')}</FieldDescription>
          )
        )}
      </FieldSet>
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
      <FieldSeparator />
      <div className="grid gap-5 sm:grid-cols-3">
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
      <div className="grid gap-3 sm:grid-cols-2">
        <BooleanField
          control={control}
          name="active"
          id="pack-active"
          variant="switch"
          label={t('catalog.fields.active')}
        />
        <BooleanField
          control={control}
          name="requires_approval"
          id="pack-approval"
          variant="switch"
          label={t('catalog.fields.requires_approval')}
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
  const nameOf = (x: Extra) => loc(x.name) || x.code;
  const columns: ColumnDef<Extra>[] = [
    {
      id: 'name',
      accessorFn: nameOf,
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => (
        <NameCell
          name={nameOf(row.original)}
          code={row.original.code}
          icon={
            row.original.component_code ? (
              <ComponentIcon code={row.original.component_code} />
            ) : (
              <PuzzleIcon aria-hidden="true" className="size-4" />
            )
          }
        />
      ),
    },
    {
      accessorKey: 'pricing_unit',
      meta: { label: t('catalog.fields.pricing_unit') },
      header: t('catalog.fields.pricing_unit'),
      cell: ({ row }) => (
        <Badge variant="outline">{t(`catalog.pricingUnits.${row.original.pricing_unit}`)}</Badge>
      ),
    },
    {
      accessorKey: 'attach_to',
      meta: { label: t('catalog.fields.attach_to') },
      header: t('catalog.fields.attach_to'),
      cell: ({ row }) => (
        <StatusBadge tone="info">{t(`catalog.attachTo.${row.original.attach_to}`)}</StatusBadge>
      ),
    },
    {
      accessorKey: 'active',
      meta: { label: t('team.status') },
      header: t('team.status'),
      cell: ({ row }) =>
        row.original.active ? (
          <StatusBadge tone="success">{t('catalog.fields.active')}</StatusBadge>
        ) : (
          <StatusBadge tone="warning">{t('catalog.inactive')}</StatusBadge>
        ),
    },
    actionsColumn(t, readOnly, nameOf, editor.edit, (x) =>
      remove.mutateAsync({ id: x.id, version: x.version }),
    ),
  ];
  const add = !readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('catalog.addExtra')}
    </Button>
  );
  return (
    <div className="space-y-4">
      <TabToolbar hint={t('catalog.extrasHint')} action={add} />
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('catalog.tabs.extras')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(x) => x.id}
          searchable
          pageSize={0}
          empty={<EmptyState icon={PuzzleIcon} title={t('catalog.emptyExtras')} bordered={false} />}
        />
      )}
      <FormSheet
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('catalog.editExtra') : t('catalog.addExtra')}
        description={t('catalog.extrasHint')}
      >
        <ExtraForm key={editor.key} ctx={ctx} item={editor.item} onDone={editor.close} />
      </FormSheet>
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
    defaultValues: toExtraValues(item, ctx.locales),
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
    <form noValidate onSubmit={s.submit} className="space-y-6">
      <FormAlert state={s.alert} onReload={onDone} />
      <div className="grid gap-5 sm:grid-cols-2">
        <Field id="ex-code" label={t('fields.code')} error={errors.code}>
          <Input
            {...fieldAria('ex-code', errors.code)}
            className="font-mono"
            {...register('code')}
          />
        </Field>
        <Field id="ex-sort" label={t('fields.sort_order')} error={errors.sort_order}>
          <Input
            {...fieldAria('ex-sort', errors.sort_order)}
            inputMode="numeric"
            {...register('sort_order')}
          />
        </Field>
        <SelectField
          control={control}
          name="pricing_unit"
          id="ex-unit"
          label={t('catalog.fields.pricing_unit')}
          options={PRICING_UNITS.map((u) => ({ value: u, label: t(`catalog.pricingUnits.${u}`) }))}
        />
        <SelectField
          control={control}
          name="attach_to"
          id="ex-attach"
          label={t('catalog.fields.attach_to')}
          options={ATTACH_TO.map((a) => ({ value: a, label: t(`catalog.attachTo.${a}`) }))}
        />
        <SelectField
          control={control}
          name="component_code"
          id="ex-component"
          label={t('catalog.fields.component_code')}
          hint={t('catalog.extraComponentHint')}
          options={[
            { value: '', label: t('catalog.noComponent') },
            ...(componentList.data ?? [])
              .filter((c) => c.tracked)
              .map((c) => ({
                value: c.code,
                label: loc(c.name) || c.code,
                icon: <ComponentIcon code={c.code} />,
              })),
          ]}
        />
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
          <SuffixInput
            {...fieldAria('ex-vat', errors.vat_rate, true)}
            suffix="%"
            inputMode="decimal"
            {...register('vat_rate')}
          />
        </Field>
      </div>
      <I18nTextField
        control={control}
        name="name"
        id="ex-name"
        label={t('fields.name')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
      />
      <I18nTextField
        control={control}
        name="description"
        id="ex-description"
        label={t('fields.description')}
        locales={ctx.locales}
        defaultLocale={ctx.defaultLocale}
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
      <BooleanField
        control={control}
        name="active"
        id="ex-active"
        variant="switch"
        label={t('catalog.fields.active')}
      />
      <FormActions onCancel={onDone} pending={s.pending} />
    </form>
  );
}
