import { ApiProblem } from '@arenile/shared/api';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { Switch } from '@arenile/shared/ui/switch';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  FieldRow,
  FormActions,
  FormDialog,
  I18nTextInput,
  OptionSelect,
  QueryState,
  RowActions,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { PlusIcon, TicketIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { packs, zones, type Pack, type Zone } from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useEditor } from '../configuration/api/useCrud';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { FormAlert } from '../configuration/components/FormAlert';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import { localize } from '../configuration/lib/i18nText';
import {
  useCreateEntryTicketType,
  useDeleteEntryTicketType,
  useEntryTicketTypes,
  useUpdateEntryTicketType,
  type EntryTicketType,
} from './api';
import {
  entryTypeSchema,
  toEntryTypeInput,
  toEntryTypeUpdate,
  toEntryTypeValues,
  type EntryTypeValues,
} from './entryType.schema';

/**
 * Configurazione → "Biglietti d'ingresso" (spec 14.3, ADR 0020): ticket types
 * (Adulto, Ridotto, Bambino, ...) for the equipped free beach or the pool.
 * Each type is a pack priced by the engine per rate period on a quantity
 * zone, whose pool is the daily capacity shared with online sales.
 */
export function EntryTicketTypesPage() {
  const { t } = useTranslation('entryTickets');
  const ctx = useConfigContext();
  const list = useEntryTicketTypes(ctx.establishmentId);
  const packList = packs.useList(ctx.establishmentId);
  const zoneList = zones.useList(ctx.establishmentId);
  const editor = useEditor<EntryTicketType>();
  const readOnly = !ctx.canConfigure;
  const loc = (text: Record<string, string> | undefined) =>
    localize(text, ctx.uiLocale, ctx.defaultLocale);
  const items = [...(list.data?.items ?? [])].sort(
    (a, b) => a.sort_order - b.sort_order || a.code.localeCompare(b.code),
  );
  const typeName = (e: EntryTicketType) => loc(e.name) || e.code;
  const packName = (id: string) => {
    const p = packList.data?.find((x) => x.id === id);
    return p ? loc(p.name) || p.code : '—';
  };
  const zoneName = (id: string) => {
    const z = zoneList.data?.find((x) => x.id === id);
    return z ? loc(z.name) || z.code : '—';
  };

  const columns: ColumnDef<EntryTicketType>[] = [
    {
      id: 'name',
      accessorFn: typeName,
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => <span className="font-medium">{typeName(row.original)}</span>,
    },
    {
      accessorKey: 'code',
      meta: { label: t('fields.code') },
      header: t('fields.code'),
      cell: ({ row }) => <Badge variant="outline">{row.original.code}</Badge>,
    },
    {
      id: 'pack',
      accessorFn: (e) => packName(e.pack_id),
      meta: { label: t('fields.pack') },
      header: t('fields.pack'),
    },
    {
      id: 'zone',
      accessorFn: (e) => zoneName(e.zone_id),
      meta: { label: t('fields.zone') },
      header: t('fields.zone'),
    },
    {
      accessorKey: 'sort_order',
      meta: { label: t('fields.sortOrder'), align: 'right' },
      header: t('fields.sortOrder'),
    },
    {
      accessorKey: 'active',
      meta: { label: t('fields.active') },
      header: t('fields.active'),
      cell: ({ row }) =>
        row.original.active ? (
          <StatusBadge tone="success">{t('types.active')}</StatusBadge>
        ) : (
          <StatusBadge tone="neutral">{t('types.inactive')}</StatusBadge>
        ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('types.actions')}</span>,
      cell: ({ row }) =>
        readOnly ? null : (
          <TypeRowActions
            establishmentId={ctx.establishmentId}
            name={typeName(row.original)}
            item={row.original}
            onEdit={editor.edit}
          />
        ),
    },
  ];

  const addButton = !readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('types.add')}
    </Button>
  );

  return (
    <ConfigPage
      title={t('types.title')}
      description={t('types.description')}
      readOnly={readOnly}
      actions={addButton}
    >
      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('types.title')}
          columns={columns}
          data={items}
          loading={list.isPending}
          getRowId={(e) => e.id}
          pageSize={0}
          empty={
            <EmptyState
              icon={TicketIcon}
              title={t('types.empty')}
              description={t('types.emptyHint')}
              bordered={false}
            />
          }
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('types.editTitle') : t('types.add')}
        description={t('types.formHint')}
      >
        <EntryTypeForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          item={editor.item}
          packs={(packList.data ?? []).filter((p) => p.active || p.id === editor.item?.pack_id)}
          zones={(zoneList.data ?? []).filter(
            (z) => z.assignment_mode === 'quantity' || z.id === editor.item?.zone_id,
          )}
          locales={ctx.locales}
          defaultLocale={ctx.defaultLocale}
          loc={loc}
          onDone={editor.close}
        />
      </FormDialog>
    </ConfigPage>
  );
}

/**
 * Edit/delete of one row. The delete mutation lives here, not in the page:
 * a page re-render rebuilds the column cells and would close the open
 * confirmation before it can show a 409.
 */
function TypeRowActions({
  establishmentId,
  name,
  item,
  onEdit,
}: {
  establishmentId: string;
  name: string;
  item: EntryTicketType;
  onEdit: (item: EntryTicketType) => void;
}) {
  const { t } = useTranslation('entryTickets');
  const remove = useDeleteEntryTicketType(establishmentId);
  const onDelete = async () => {
    try {
      await remove.mutateAsync(item.id);
    } catch (err) {
      // Tickets already sold: explain that the type must be deactivated.
      if (err instanceof ApiProblem && err.status === 409) {
        throw new ApiProblem({
          type: err.type,
          status: err.status,
          title: err.title,
          detail: t('types.inUse'),
        });
      }
      throw err;
    }
  };
  return (
    <RowActions
      editLabel={t('types.editAria', { name })}
      deleteLabel={t('types.deleteAria', { name })}
      deleteMessage={t('types.deleteConfirm', { name })}
      onEdit={() => onEdit(item)}
      onDelete={onDelete}
    />
  );
}

export interface EntryTypeFormProps {
  establishmentId: string;
  item?: EntryTicketType;
  packs: Pack[];
  zones: Zone[];
  locales: string[];
  defaultLocale: string;
  loc: (text: Record<string, string> | undefined) => string;
  onDone: () => void;
}

/** Create/edit form of one entry ticket type (shadcn Form + zod, server errors on fields). */
export function EntryTypeForm({
  establishmentId,
  item,
  packs: packItems,
  zones: zoneItems,
  locales,
  defaultLocale,
  loc,
  onDone,
}: EntryTypeFormProps) {
  const { t } = useTranslation('entryTickets');
  const form = useForm<EntryTypeValues>({
    resolver: zodResolver(entryTypeSchema(defaultLocale)),
    defaultValues: toEntryTypeValues(item, locales),
  });
  const create = useCreateEntryTicketType(establishmentId);
  const update = useUpdateEntryTicketType(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const pending = create.isPending || update.isPending;

  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      if (item) {
        await update.mutateAsync({
          id: item.id,
          version: item.version,
          body: toEntryTypeUpdate(values),
        });
      } else {
        await create.mutateAsync(toEntryTypeInput(values));
      }
      toast.success(t(item ? 'types.saved' : 'types.created'));
      onDone();
    } catch (err) {
      setAlert(
        problemToFormState(err, form.setError, {
          formKeys: Object.keys(form.getValues()),
          label: (path) => {
            const head = path.split('.')[0];
            return head === 'pack_id'
              ? t('fields.pack')
              : head === 'zone_id'
                ? t('fields.zone')
                : undefined;
          },
        }),
      );
    }
  });

  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
        <FormAlert state={alert} onReload={onDone} />
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow
            control={form.control}
            name="code"
            label={t('fields.code')}
            description={item ? t('types.codeFixed') : t('types.codeHint')}
          >
            {(field) => (
              <Input autoComplete="off" spellCheck={false} disabled={!!item} {...field} />
            )}
          </FieldRow>
          <FieldRow control={form.control} name="sort_order" label={t('fields.sortOrder')} optional>
            {(field) => <Input inputMode="numeric" autoComplete="off" {...field} />}
          </FieldRow>
        </div>
        <FieldRow control={form.control} name="name" label={t('fields.name')}>
          {(field) => (
            <I18nTextInput
              value={field.value}
              onChange={field.onChange}
              locales={locales}
              defaultLocale={defaultLocale}
              placeholder={t('types.namePlaceholder')}
            />
          )}
        </FieldRow>
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow
            control={form.control}
            name="pack_id"
            label={t('fields.pack')}
            description={t('types.packHint')}
          >
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                onBlur={field.onBlur}
                placeholder={t('types.choose')}
                options={packItems.map((p) => ({ value: p.id, label: loc(p.name) || p.code }))}
              />
            )}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="zone_id"
            label={t('fields.zone')}
            description={t('types.zoneHint')}
          >
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                onBlur={field.onBlur}
                placeholder={t('types.choose')}
                options={zoneItems.map((z) => ({ value: z.id, label: loc(z.name) || z.code }))}
              />
            )}
          </FieldRow>
        </div>
        <FieldRow
          control={form.control}
          name="description"
          label={t('fields.description')}
          optional
        >
          {(field) => (
            <I18nTextInput
              value={field.value}
              onChange={field.onChange}
              locales={locales}
              defaultLocale={defaultLocale}
              multiline
            />
          )}
        </FieldRow>
        <FieldRow
          control={form.control}
          name="active"
          label={t('fields.active')}
          description={t('types.activeHint')}
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} onBlur={field.onBlur} />
          )}
        </FieldRow>
        <FormActions onCancel={onDone} pending={pending} />
      </form>
    </Form>
  );
}
