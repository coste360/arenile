import { ApiProblem, mapProblemToForm, problemMessageKey } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Switch } from '@arenile/shared/ui/switch';
import {
  DataTable,
  EmptyState,
  FormActions,
  FormDialog,
  Page,
  QueryState,
  RowActions,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { EyeIcon, LockIcon, PlusIcon, WalletCardsIcon } from 'lucide-react';
import { useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  useCreateRegister,
  useDeleteRegister,
  useRegisters,
  useUpdateRegister,
  type CashRegister,
} from './api';
import { CashField } from './components/fields';
import { fieldAria } from './lib/aria';
import { registerSchema, type RegisterValues } from './schemas';
import { useCashContext } from './useCashContext';

/**
 * Configurazione → Casse (ADR 0019): the tills of the establishment. One
 * open session per register (INV-73), the default "Cassa principale" is
 * always there and active; a register with history can only be deactivated.
 * Owner and manager edit (establishment.configure); cash roles read.
 */
export function CashRegistersPage() {
  const { t } = useTranslation('cash');
  const ctx = useCashContext();
  const canSee = ctx.canRead || ctx.canConfigure;
  const list = useRegisters(ctx.establishmentId, true, !ctx.loading && canSee);
  const remove = useDeleteRegister(ctx.establishmentId);
  const [editing, setEditing] = useState<CashRegister | 'new' | null>(null);
  const readOnly = !ctx.canConfigure;

  if (ctx.loading) return <QueryState isLoading error={null} rows={4} />;
  if (!canSee) {
    return (
      <Page title={t('registersPage.title')} description={t('registersPage.description')}>
        <Alert>
          <LockIcon aria-hidden="true" />
          <AlertTitle>{t('perm.forbiddenTitle')}</AlertTitle>
          <AlertDescription>{t('perm.forbidden')}</AlertDescription>
        </Alert>
      </Page>
    );
  }

  const columns: ColumnDef<CashRegister>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('registersPage.fields.name') },
      header: t('registersPage.fields.name'),
      cell: ({ row }) => (
        <span className="flex flex-col gap-0.5">
          <span className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{row.original.name}</span>
            {row.original.is_default && (
              <StatusBadge tone="neutral">{t('registersPage.default')}</StatusBadge>
            )}
          </span>
          <span className="font-mono text-xs text-muted-foreground sm:hidden">
            {row.original.code}
          </span>
        </span>
      ),
    },
    {
      accessorKey: 'code',
      meta: { label: t('registersPage.fields.code'), hideBelow: 'sm' },
      header: t('registersPage.fields.code'),
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.code}</span>,
    },
    {
      id: 'status',
      meta: { label: t('registersPage.fields.status') },
      header: t('registersPage.fields.status'),
      cell: ({ row }) =>
        !row.original.active ? (
          <StatusBadge tone="neutral">{t('registersPage.inactive')}</StatusBadge>
        ) : row.original.open_session_id ? (
          <StatusBadge tone="success">{t('registersPage.sessionOpen')}</StatusBadge>
        ) : (
          <StatusBadge tone="info">{t('registersPage.sessionClosed')}</StatusBadge>
        ),
    },
    {
      accessorKey: 'sort_order',
      meta: { label: t('registersPage.fields.sortOrder'), align: 'right', hideBelow: 'md' },
      header: t('registersPage.fields.sortOrder'),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('registersPage.actions')}</span>,
      cell: ({ row }) => {
        const r = row.original;
        if (readOnly) return null;
        return (
          <RowActions
            editLabel={t('registersPage.editAria', { name: r.name })}
            onEdit={() => setEditing(r)}
            {...(r.is_default
              ? {}
              : {
                  deleteLabel: t('registersPage.deleteAria', { name: r.name }),
                  deleteMessage: t('registersPage.deleteConfirm', { name: r.name }),
                  onDelete: async () => {
                    try {
                      await remove.mutateAsync({ id: r.id, version: r.version });
                      toast.success(t('registersPage.deleted'));
                    } catch (err) {
                      toast.error(
                        err instanceof ApiProblem && err.status === 409
                          ? t('registersPage.deleteInUse')
                          : t(problemMessageKey(err), { ns: 'translation' }),
                      );
                    }
                  },
                })}
          />
        );
      },
    },
  ];

  return (
    <Page
      title={t('registersPage.title')}
      description={t('registersPage.description')}
      actions={
        !readOnly && (
          <Button onClick={() => setEditing('new')}>
            <PlusIcon aria-hidden="true" />
            {t('registersPage.add')}
          </Button>
        )
      }
      notice={
        readOnly && (
          <Alert role="note">
            <EyeIcon aria-hidden="true" />
            <AlertDescription>{t('registersPage.readOnly')}</AlertDescription>
          </Alert>
        )
      }
    >
      <DataTable
        caption={t('registersPage.title')}
        columns={columns}
        data={list.data ?? []}
        loading={list.isPending}
        error={
          list.error ? (
            <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
          ) : undefined
        }
        getRowId={(r) => r.id}
        pageSize={0}
        columnToggle={false}
        empty={
          <EmptyState icon={WalletCardsIcon} title={t('registersPage.empty')} bordered={false} />
        }
      />
      <FormDialog
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
        title={editing === 'new' ? t('registersPage.add') : t('registersPage.editTitle')}
        description={t('registersPage.formDescription')}
      >
        {editing !== null && (
          <RegisterForm
            key={editing === 'new' ? 'new' : editing.id}
            establishmentId={ctx.establishmentId}
            item={editing === 'new' ? undefined : editing}
            onDone={() => setEditing(null)}
          />
        )}
      </FormDialog>
    </Page>
  );
}

function RegisterForm({
  establishmentId,
  item,
  onDone,
}: {
  establishmentId: string;
  item?: CashRegister;
  onDone: () => void;
}) {
  const { t } = useTranslation('cash');
  const create = useCreateRegister(establishmentId);
  const update = useUpdateRegister(establishmentId);
  const [alert, setAlert] = useState<string[] | null>(null);
  const form = useForm<RegisterValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      name: item?.name ?? '',
      code: item?.code ?? '',
      sort_order: item?.sort_order ?? 0,
      active: item?.active ?? true,
    },
  });
  const errors = form.formState.errors;
  const pending = create.isPending || update.isPending;

  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    const body = { name: v.name.trim(), sort_order: v.sort_order, active: v.active };
    try {
      if (item) {
        await update.mutateAsync({ register: item, body });
      } else {
        await create.mutateAsync({ ...body, ...(v.code ? { code: v.code } : {}) });
      }
      toast.success(t('registersPage.saved'));
      onDone();
    } catch (err) {
      const mapped = mapProblemToForm(err, form.setError, {
        formKeys: ['name', 'code', 'sort_order', 'active'],
      });
      if (err instanceof ApiProblem && err.type === 'cash_session_already_open') {
        setAlert([t('registersPage.deactivateOpen')]);
      } else if (err instanceof ApiProblem && err.status === 409) {
        setAlert([err.detail ?? t(problemMessageKey(err), { ns: 'translation' })]);
      } else if (mapped.errors.length === 0) {
        setAlert([t(problemMessageKey(err), { ns: 'translation' })]);
      } else if (mapped.unmatched.length > 0) {
        setAlert(mapped.unmatched);
      }
    }
  });

  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      {alert && (
        <Alert variant="destructive">
          <AlertDescription>
            {alert.map((a) => (
              <span key={a} className="block">
                {a}
              </span>
            ))}
          </AlertDescription>
        </Alert>
      )}
      <div className="grid gap-5 sm:grid-cols-2">
        <CashField id="register-name" label={t('registersPage.fields.name')} error={errors.name}>
          <Input {...fieldAria('register-name', errors.name)} {...form.register('name')} />
        </CashField>
        <CashField
          id="register-code"
          label={t('registersPage.fields.code')}
          hint={item ? t('registersPage.codeFixed') : t('registersPage.codeHint')}
          error={errors.code}
        >
          <Input
            {...fieldAria('register-code', errors.code, true)}
            className="font-mono"
            disabled={!!item}
            autoCapitalize="none"
            spellCheck={false}
            {...form.register('code')}
          />
        </CashField>
        <CashField
          id="register-sort"
          label={t('registersPage.fields.sortOrder')}
          error={errors.sort_order}
        >
          <Input
            {...fieldAria('register-sort', errors.sort_order)}
            inputMode="numeric"
            {...form.register('sort_order', { valueAsNumber: true })}
          />
        </CashField>
        <CashField
          id="register-active"
          label={t('registersPage.fields.active')}
          hint={item?.is_default ? t('registersPage.defaultAlwaysActive') : undefined}
          error={errors.active}
        >
          <Controller
            control={form.control}
            name="active"
            render={({ field }) => (
              <Switch
                id="register-active"
                checked={field.value}
                onCheckedChange={field.onChange}
                disabled={item?.is_default}
                aria-describedby={item?.is_default ? 'register-active-hint' : undefined}
              />
            )}
          />
        </CashField>
      </div>
      <FormActions onCancel={onDone} pending={pending} />
    </form>
  );
}
