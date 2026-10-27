import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { Switch } from '@arenile/shared/ui/switch';
import {
  ConfirmDialog,
  DataTable,
  DataTableColumnHeader,
  EditButton,
  EmptyState,
  FieldRow,
  FormActions,
  FormDialog,
  OptionSelect,
  QueryState,
  RowActions,
  SectionCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { PlusIcon, Trash2Icon, UsersRoundIcon } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useEditor } from '../configuration/api/useCrud';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { customerCategories, REGISTER_CATEGORIES, type CustomerCategory } from './api';
import { describeM8Problem } from './lib';
import {
  categorySchema,
  categoryValues,
  toCategoryInput,
  type CategoryValues,
} from './partner.schema';

/**
 * Customer categories (spec 30.2.2): residents, over 65, people with
 * disabilities... target of agreements, with the requirement checked at the
 * desk and the category of the agreements register.
 */
export function CustomerCategoriesTab({
  establishmentId,
  canConfigure,
}: {
  establishmentId: string;
  canConfigure: boolean;
}) {
  const { t } = useTranslation('partners');
  const list = customerCategories.useList(establishmentId);
  const editor = useEditor<CustomerCategory>();
  const items = list.data ?? [];
  const columns: ColumnDef<CustomerCategory>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('categories.name') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('categories.name')} />
      ),
      cell: ({ row: { original: c } }) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium">{c.name}</span>
          <Badge variant="outline" className="font-mono">
            {c.code}
          </Badge>
          {!c.active && <StatusBadge tone="neutral">{t('inactive')}</StatusBadge>}
        </span>
      ),
    },
    {
      id: 'register',
      meta: { label: t('categories.register') },
      header: t('categories.register'),
      accessorFn: (c) => t(`register.${c.register_category}`),
    },
    {
      id: 'requirement',
      meta: { label: t('categories.requirement') },
      header: t('categories.requirement'),
      cell: ({ row }) => (
        <span className="text-muted-foreground">{row.original.requirement || '—'}</span>
      ),
    },
    {
      id: 'members',
      meta: { label: t('categories.members'), align: 'right' },
      header: t('categories.members'),
      accessorFn: (c) => c.member_count ?? 0,
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actionsColumn')}</span>,
      cell: ({ row: { original: c } }) =>
        canConfigure ? (
          <RowActions>
            <EditButton
              label={t('categories.editAria', { name: c.name })}
              onClick={() => editor.edit(c)}
            />
            <DeleteCategory establishmentId={establishmentId} category={c} />
          </RowActions>
        ) : null,
    },
  ];
  return (
    <section aria-labelledby="categories-title">
      <SectionCard
        titleId="categories-title"
        icon={UsersRoundIcon}
        title={t('categories.title')}
        description={t('categories.description')}
        flush
      >
        <DataTable
          embedded
          caption={t('categories.title')}
          loading={list.isPending}
          error={
            list.error ? (
              <QueryState
                isLoading={false}
                error={list.error}
                onRetry={() => void list.refetch()}
              />
            ) : undefined
          }
          columns={columns}
          data={items}
          getRowId={(c) => c.id}
          searchable={items.length > 5}
          toolbar={
            canConfigure && (
              <Button onClick={editor.create}>
                <PlusIcon aria-hidden="true" />
                {t('categories.add')}
              </Button>
            )
          }
          initialSorting={[{ id: 'name', desc: false }]}
          empty={
            <EmptyState
              icon={UsersRoundIcon}
              bordered={false}
              title={t('categories.empty')}
              description={t('categories.emptyHint')}
            />
          }
        />
      </SectionCard>
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        title={
          editor.item ? t('categories.editTitle', { name: editor.item.name }) : t('categories.add')
        }
        description={t('categories.formDescription')}
        wide
      >
        {editor.open && (
          <CategoryForm
            key={editor.key}
            establishmentId={establishmentId}
            category={editor.item}
            onDone={editor.close}
          />
        )}
      </FormDialog>
    </section>
  );
}

/** Own component with its own mutation: the table does not re-render while it runs. */
function DeleteCategory({
  establishmentId,
  category,
}: {
  establishmentId: string;
  category: CustomerCategory;
}) {
  const { t } = useTranslation('partners');
  const remove = customerCategories.useRemove(establishmentId);
  return (
    <ConfirmDialog
      title={t('common.actions.confirmDelete', { ns: 'translation' })}
      description={t('categories.deleteConfirm', { name: category.name })}
      confirmLabel={t('common.actions.delete', { ns: 'translation' })}
      destructive
      errorText={(err) => describeM8Problem(err, t).title}
      onConfirm={async () => {
        await remove.mutateAsync({ id: category.id, version: category.version });
        toast.success(t('ui.deleted', { ns: 'translation' }));
      }}
      trigger={
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('categories.deleteAria', { name: category.name })}
          title={t('categories.deleteAria', { name: category.name })}
          className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
        >
          <Trash2Icon aria-hidden="true" />
        </Button>
      }
    />
  );
}

function CategoryForm({
  establishmentId,
  category,
  onDone,
}: {
  establishmentId: string;
  category?: CustomerCategory;
  onDone: () => void;
}) {
  const { t } = useTranslation('partners');
  const create = customerCategories.useCreate(establishmentId);
  const update = customerCategories.useUpdate(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<CategoryValues>({
    resolver: zodResolver(categorySchema),
    defaultValues: categoryValues(category),
  });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      const input = toCategoryInput(v);
      if (category) await update.mutateAsync({ id: category.id, version: category.version, input });
      else await create.mutateAsync(input);
      toast.success(category ? t('categories.updated') : t('categories.created'));
      onDone();
    } catch (err) {
      setFailure(describeM8Problem(err, t));
    }
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-4">
        <ProblemAlert problem={failure} />
        <div className="grid gap-4 sm:grid-cols-2">
          <FieldRow control={form.control} name="name" label={t('categories.name')}>
            {(field) => <Input autoComplete="off" {...field} />}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="code"
            label={t('categories.code')}
            description={t('categories.codeHelp')}
          >
            {(field) => <Input autoComplete="off" className="font-mono" {...field} />}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="register_category"
            label={t('categories.register')}
            description={t('categories.registerHelp')}
          >
            {(field) => (
              <OptionSelect
                value={field.value}
                onChange={field.onChange}
                options={REGISTER_CATEGORIES.map((r) => ({ value: r, label: t(`register.${r}`) }))}
              />
            )}
          </FieldRow>
          <FieldRow
            control={form.control}
            name="requirement"
            label={t('categories.requirement')}
            description={t('categories.requirementHelp')}
            optional
          >
            {(field) => <Input autoComplete="off" {...field} />}
          </FieldRow>
        </div>
        <FieldRow
          control={form.control}
          name="description"
          label={t('categories.descriptionField')}
          optional
        >
          {(field) => <Input autoComplete="off" {...field} />}
        </FieldRow>
        <FieldRow control={form.control} name="active" label={t('fields.active')}>
          {(field) => <Switch checked={field.value} onCheckedChange={field.onChange} />}
        </FieldRow>
        <FormActions onCancel={onDone} pending={create.isPending || update.isPending} />
      </form>
    </Form>
  );
}
