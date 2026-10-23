import {
  Badge,
  Button,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Textarea,
} from '@arenile/shared/ui';
import {
  type ColumnDef,
  DataTable,
  DataTableColumnHeader,
  DatePicker,
  DeleteButton,
  EditButton,
  EmptyState,
  FormSheet,
  QueryState,
  RowActions,
  SectionCard,
  SheetForm,
} from '@arenile/shared/ui/patterns';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { zodResolver } from '@hookform/resolvers/zod';
import { HandshakeIcon, PlusIcon, TicketPercentIcon } from 'lucide-react';
import { useForm, type Control, type FieldValues, type Path } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useCrudSubmit, useEditor } from '../configuration/api/useCrud';
import { agreements, promoCodes, type Agreement, type PromoCode } from './api';
import { ActionFields, ConditionFields } from './components';
import { describeAction } from './format';
import { EditorSection, FormErrorAlert, SwitchRow, TextRow } from './kit';
import type { PricingTabProps } from './RulesTab';
import {
  ACTIONS_BY_STAGE,
  FORM_ALIASES,
  agreementSchema,
  agreementValues,
  promoSchema,
  promoValues,
  toAgreementInput,
  toPromoInput,
  type AgreementValues,
  type PromoValues,
} from './rules.schema';

function validity(
  t: (k: string, o?: Record<string, unknown>) => string,
  from?: string,
  to?: string,
) {
  const f = from && formatIsoDate(from);
  const u = to && formatIsoDate(to);
  if (f && u) return t('validity.range', { from: f, to: u });
  if (f) return t('validity.from', { from: f });
  if (u) return t('validity.to', { to: u });
  return t('validity.always');
}

/** Promo codes (case-insensitive, unique) and agreements: discount-stage actions (spec 8.4). */
export function DiscountsTab(p: PricingTabProps) {
  return (
    <div className="space-y-6">
      <PromoSection {...p} />
      <AgreementSection {...p} />
    </div>
  );
}

function ActiveBadge({ active }: { active: boolean }) {
  const { t } = useTranslation('pricing');
  return active ? null : <Badge variant="outline">{t('inactive')}</Badge>;
}

function PromoSection(p: PricingTabProps) {
  const { t } = useTranslation('pricing');
  const list = promoCodes.useList(p.establishmentId);
  const remove = promoCodes.useRemove(p.establishmentId);
  const editor = useEditor<PromoCode>();
  const items = list.data ?? [];
  const columns: ColumnDef<PromoCode>[] = [
    {
      accessorKey: 'code',
      meta: { label: t('fields.code') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.code')} />,
      cell: ({ row }) => (
        <div className="flex items-center gap-1.5">
          <Badge variant="outline" className="font-mono">
            {row.original.code}
          </Badge>
          <ActiveBadge active={row.original.active} />
        </div>
      ),
    },
    {
      id: 'action',
      meta: { label: t('fields.action') },
      header: t('fields.action'),
      accessorFn: (x) => describeAction(t, x.action, p.locale),
    },
    {
      id: 'validity',
      meta: { label: t('fields.validity') },
      header: t('fields.validity'),
      accessorFn: (x) => validity(t, x.valid_from, x.valid_to),
    },
    {
      accessorKey: 'uses',
      meta: { label: t('promo.uses'), align: 'right' },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('promo.uses')} />,
      cell: ({ row }) => (
        <span className="tabular-nums">
          {row.original.max_uses
            ? `${row.original.uses} / ${row.original.max_uses}`
            : row.original.uses}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actionsColumn')}</span>,
      cell: ({ row }) =>
        p.readOnly ? null : (
          <RowActions>
            <EditButton
              label={t('promo.editAria', { name: row.original.code })}
              onClick={() => editor.edit(row.original)}
            />
            <DeleteButton
              label={t('promo.deleteAria', { name: row.original.code })}
              message={t('promo.deleteConfirm', { name: row.original.code })}
              onConfirm={() =>
                remove.mutateAsync({ id: row.original.id, version: row.original.version })
              }
            />
          </RowActions>
        ),
    },
  ];
  const add = !p.readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('promo.add')}
    </Button>
  );
  return (
    <section aria-labelledby="promo-title">
      <SectionCard
        titleId="promo-title"
        icon={TicketPercentIcon}
        title={t('promo.title')}
        description={t('promo.description')}
        flush
      >
        <ListQueryState query={list} />
        {list.data && (
          <DataTable
            embedded
            caption={t('promo.title')}
            columns={columns}
            data={items}
            getRowId={(x) => x.id}
            searchable={items.length > 5}
            toolbar={add}
            initialSorting={[{ id: 'code', desc: false }]}
            empty={
              <EmptyState icon={TicketPercentIcon} title={t('promo.empty')} bordered={false} />
            }
          />
        )}
      </SectionCard>
      <FormSheet
        size="lg"
        padded={false}
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={editor.item ? t('promo.editTitle', { name: editor.item.code }) : t('promo.add')}
      >
        <PromoForm key={editor.key} {...p} item={editor.item} onDone={editor.close} />
      </FormSheet>
    </section>
  );
}

/** Loading/error state of a list inside a flush SectionCard (padded like the card body). */
function ListQueryState({
  query,
}: {
  query: { isPending: boolean; error: unknown; refetch: () => unknown };
}) {
  if (!query.isPending && !query.error) return null;
  return (
    <div className="px-6 pb-6">
      <QueryState
        isLoading={query.isPending}
        error={query.error}
        onRetry={() => void query.refetch()}
      />
    </div>
  );
}

/** "Valido dal" / "Valido fino al" date pickers. */
function ValidityFields<T extends FieldValues>({ control }: { control: Control<T> }) {
  const { t } = useTranslation('pricing');
  return (
    <>
      {(['valid_from', 'valid_to'] as const).map((name) => (
        <FormField
          key={name}
          control={control}
          name={name as Path<T>}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t(`fields.${name}`)}</FormLabel>
              <FormControl>
                <DatePicker
                  value={field.value as string}
                  onChange={(v) => field.onChange(v ?? '')}
                  clearable
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      ))}
    </>
  );
}

function PromoForm(p: PricingTabProps & { item?: PromoCode; onDone: () => void }) {
  const { t } = useTranslation('pricing');
  const form = useForm<PromoValues>({
    resolver: zodResolver(promoSchema),
    defaultValues: promoValues(p.item),
  });
  const s = useCrudSubmit({
    form,
    item: p.item,
    create: promoCodes.useCreate(p.establishmentId),
    update: promoCodes.useUpdate(p.establishmentId),
    toInput: toPromoInput,
    onDone: p.onDone,
    aliases: FORM_ALIASES,
  });
  const { control } = form;
  const numeric = { inputMode: 'numeric' as const, className: 'max-w-32' };
  return (
    <Form {...form}>
      <SheetForm onSubmit={s.submit} onCancel={p.onDone} pending={s.pending}>
        <FormErrorAlert state={s.alert} onReload={p.onDone} />
        <EditorSection title={t('sections.general')}>
          <div className="grid gap-4 sm:grid-cols-3">
            <TextRow
              control={control}
              name="code"
              label={t('fields.code')}
              inputProps={{ className: 'font-mono', autoComplete: 'off' }}
            />
            <TextRow
              control={control}
              name="max_uses"
              label={t('fields.max_uses')}
              inputProps={numeric}
            />
            <TextRow
              control={control}
              name="max_uses_per_customer"
              label={t('fields.max_uses_per_customer')}
              inputProps={numeric}
            />
            <ValidityFields control={control} />
          </div>
          <DescriptionField control={control} name="description" label={t('fields.description')} />
        </EditorSection>
        <ActionFields control={control} allowed={ACTIONS_BY_STAGE.discount} />
        <ConditionFields control={control} periods={p.periods} zones={p.zones} />
        <OptionsSection control={control} />
      </SheetForm>
    </Form>
  );
}

function DescriptionField<T extends FieldValues>({
  control,
  name,
  label,
}: {
  control: Control<T>;
  name: Path<T>;
  label: string;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Textarea {...field} value={field.value as string} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function OptionsSection<T extends FieldValues>({ control }: { control: Control<T> }) {
  const { t } = useTranslation('pricing');
  return (
    <EditorSection title={t('sections.options')}>
      <div className="grid gap-3 sm:grid-cols-2">
        <SwitchRow
          control={control}
          name={'stackable' as Path<T>}
          label={t('fields.stackable')}
          description={t('hints.stackable')}
        />
        <SwitchRow control={control} name={'active' as Path<T>} label={t('fields.active')} />
      </div>
    </EditorSection>
  );
}

function AgreementSection(p: PricingTabProps) {
  const { t } = useTranslation('pricing');
  const list = agreements.useList(p.establishmentId);
  const remove = agreements.useRemove(p.establishmentId);
  const editor = useEditor<Agreement>();
  const items = list.data ?? [];
  const columns: ColumnDef<Agreement>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('fields.name') },
      header: ({ column }) => <DataTableColumnHeader column={column} title={t('fields.name')} />,
      cell: ({ row }) => (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium">{row.original.name}</span>
          {row.original.code && (
            <Badge variant="outline" className="font-mono">
              {row.original.code}
            </Badge>
          )}
          <ActiveBadge active={row.original.active} />
        </div>
      ),
    },
    {
      accessorKey: 'partner_name',
      meta: { label: t('fields.partner_name') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('fields.partner_name')} />
      ),
      cell: ({ row }) => row.original.partner_name || '—',
    },
    {
      id: 'action',
      meta: { label: t('fields.action') },
      header: t('fields.action'),
      accessorFn: (x) => describeAction(t, x.action, p.locale),
    },
    {
      id: 'validity',
      meta: { label: t('fields.validity') },
      header: t('fields.validity'),
      accessorFn: (x) => validity(t, x.valid_from, x.valid_to),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actionsColumn')}</span>,
      cell: ({ row }) =>
        p.readOnly ? null : (
          <RowActions>
            <EditButton
              label={t('agreements.editAria', { name: row.original.name })}
              onClick={() => editor.edit(row.original)}
            />
            <DeleteButton
              label={t('agreements.deleteAria', { name: row.original.name })}
              message={t('agreements.deleteConfirm', { name: row.original.name })}
              onConfirm={() =>
                remove.mutateAsync({ id: row.original.id, version: row.original.version })
              }
            />
          </RowActions>
        ),
    },
  ];
  const add = !p.readOnly && (
    <Button onClick={editor.create}>
      <PlusIcon aria-hidden="true" />
      {t('agreements.add')}
    </Button>
  );
  return (
    <section aria-labelledby="agreements-title">
      <SectionCard
        titleId="agreements-title"
        icon={HandshakeIcon}
        title={t('agreements.title')}
        description={t('agreements.description')}
        flush
      >
        <ListQueryState query={list} />
        {list.data && (
          <DataTable
            embedded
            caption={t('agreements.title')}
            columns={columns}
            data={items}
            getRowId={(x) => x.id}
            searchable={items.length > 5}
            toolbar={add}
            initialSorting={[{ id: 'name', desc: false }]}
            empty={
              <EmptyState icon={HandshakeIcon} title={t('agreements.empty')} bordered={false} />
            }
          />
        )}
      </SectionCard>
      <FormSheet
        size="lg"
        padded={false}
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={
          editor.item ? t('agreements.editTitle', { name: editor.item.name }) : t('agreements.add')
        }
      >
        <AgreementForm key={editor.key} {...p} item={editor.item} onDone={editor.close} />
      </FormSheet>
    </section>
  );
}

function AgreementForm(p: PricingTabProps & { item?: Agreement; onDone: () => void }) {
  const { t } = useTranslation('pricing');
  const form = useForm<AgreementValues>({
    resolver: zodResolver(agreementSchema),
    defaultValues: agreementValues(p.item),
  });
  const s = useCrudSubmit({
    form,
    item: p.item,
    create: agreements.useCreate(p.establishmentId),
    update: agreements.useUpdate(p.establishmentId),
    toInput: toAgreementInput,
    onDone: p.onDone,
    aliases: FORM_ALIASES,
  });
  const { control } = form;
  return (
    <Form {...form}>
      <SheetForm onSubmit={s.submit} onCancel={p.onDone} pending={s.pending}>
        <FormErrorAlert state={s.alert} onReload={p.onDone} />
        <EditorSection title={t('sections.general')}>
          <div className="grid gap-4 sm:grid-cols-3">
            <TextRow control={control} name="name" label={t('fields.name')} />
            <TextRow control={control} name="partner_name" label={t('fields.partner_name')} />
            <TextRow
              control={control}
              name="code"
              label={t('fields.code')}
              description={t('hints.agreementCode')}
              inputProps={{ className: 'font-mono', autoComplete: 'off' }}
            />
            <ValidityFields control={control} />
          </div>
          <DescriptionField control={control} name="notes" label={t('fields.notes')} />
        </EditorSection>
        <ActionFields control={control} allowed={ACTIONS_BY_STAGE.discount} />
        <ConditionFields control={control} periods={p.periods} zones={p.zones} />
        <OptionsSection control={control} />
      </SheetForm>
    </Form>
  );
}
