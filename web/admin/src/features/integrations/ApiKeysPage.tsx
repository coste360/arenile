import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import {
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
  Field as UiField,
} from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import {
  ConfirmDialog,
  DataTable,
  DataTableColumnHeader,
  DatePicker,
  EditButton,
  EmptyState,
  FormActions,
  FormDialog,
  QueryState,
  StatusBadge,
  type ColumnDef,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { Switch } from '@arenile/shared/ui/switch';
import { zodResolver } from '@hookform/resolvers/zod';
import { BanIcon, InfoIcon, KeyRoundIcon, LockIcon, PlusIcon } from 'lucide-react';
import { useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useEditor } from '../configuration/api/useCrud';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { Field, FieldError } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { errorAt, fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import {
  API_KEY_SCOPES,
  useApiKeys,
  useCreateApiKey,
  useRevokeApiKey,
  useUpdateApiKey,
  type ApiKey,
  type ApiKeyStatus,
} from './api';
import { SecretDialog } from './components/SecretDialog';
import {
  apiKeyDefaults,
  apiKeySchema,
  toApiKeyCreate,
  toApiKeyUpdate,
  type ApiKeyValues,
} from './lib/forms';

const STATUS_TONE: Record<ApiKeyStatus, StatusTone> = {
  active: 'success',
  expired: 'warning',
  revoked: 'neutral',
};

/** Today in the browser calendar (expiry must be today or later). */
function todayIso(): string {
  return new Intl.DateTimeFormat('en-CA').format(new Date());
}

/**
 * Configurazione → Chiavi API (owner only, users.manage; spec 16, 17): keys
 * for external systems with limited scopes and optional expiry. The full
 * key is shown once after creation; revoking is immediate and final.
 */
export function ApiKeysPage() {
  const { t } = useTranslation('integrations');
  const ctx = useConfigContext();
  const allowed = ctx.canManageUsers;
  const [includeRevoked, setIncludeRevoked] = useState(false);
  const keys = useApiKeys(ctx.establishmentId, includeRevoked, allowed);
  const revoke = useRevokeApiKey(ctx.establishmentId);
  const editor = useEditor<ApiKey>();
  const [secret, setSecret] = useState<string | null>(null);
  // Dialog state lives here, not in the table cells: cells remount on every render.
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const fmt = useInstantFormat();
  const problemText = useProblemText();

  if (ctx.isLoading) {
    return (
      <ConfigPage title={t('apiKeys.title')}>
        <QueryState isLoading error={null} />
      </ConfigPage>
    );
  }
  if (!allowed) {
    return (
      <ConfigPage title={t('apiKeys.title')}>
        <EmptyState icon={LockIcon} title={t('ownerOnly')} />
      </ConfigPage>
    );
  }

  const columns: ColumnDef<ApiKey>[] = [
    {
      accessorKey: 'name',
      meta: { label: t('apiKeys.fields.name') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('apiKeys.fields.name')} />
      ),
      cell: ({ row }) => (
        <span className="flex min-w-0 flex-col">
          <span className="font-medium">{row.original.name}</span>
          <code className="truncate font-mono text-xs text-muted-foreground">
            {row.original.prefix}…
          </code>
        </span>
      ),
    },
    {
      id: 'scopes',
      accessorFn: (k) => k.scopes.join(' '),
      meta: { label: t('apiKeys.fields.scopes') },
      header: t('apiKeys.fields.scopes'),
      cell: ({ row }) => (
        <span className="flex max-w-xs flex-wrap gap-1">
          {row.original.scopes.map((s) => (
            <Badge key={s} variant="secondary">
              {t(`scopes.${s}.label`)}
            </Badge>
          ))}
        </span>
      ),
    },
    {
      accessorKey: 'status',
      meta: { label: t('apiKeys.fields.status') },
      header: t('apiKeys.fields.status'),
      cell: ({ row }) => (
        <StatusBadge tone={STATUS_TONE[row.original.status]}>
          {t(`apiKeys.status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      id: 'created',
      accessorFn: (k) => k.created_at,
      meta: { label: t('apiKeys.fields.created') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('apiKeys.fields.created')} />
      ),
      cell: ({ row }) => (
        <span className="flex flex-col text-sm">
          <span className="whitespace-nowrap">{fmt.date(row.original.created_at)}</span>
          {row.original.created_by_name && (
            <span className="text-xs text-muted-foreground">{row.original.created_by_name}</span>
          )}
        </span>
      ),
    },
    {
      id: 'last_used',
      accessorFn: (k) => k.last_used_at ?? '',
      meta: { label: t('apiKeys.fields.lastUsed') },
      header: t('apiKeys.fields.lastUsed'),
      cell: ({ row }) => (
        <span className="whitespace-nowrap text-muted-foreground">
          {row.original.last_used_at ? fmt.dateTime(row.original.last_used_at) : t('never')}
        </span>
      ),
    },
    {
      id: 'expires',
      accessorFn: (k) => k.expires_at ?? '',
      meta: { label: t('apiKeys.fields.expires') },
      header: t('apiKeys.fields.expires'),
      cell: ({ row }) => (
        <span className="whitespace-nowrap text-muted-foreground">
          {row.original.revoked_at
            ? t('apiKeys.revokedOn', { date: fmt.date(row.original.revoked_at) })
            : row.original.expires_at
              ? fmt.date(row.original.expires_at)
              : t('apiKeys.noExpiry')}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const k = row.original;
        if (k.status === 'revoked') return null;
        return (
          <div className="flex items-center justify-end gap-0.5">
            <EditButton
              label={t('apiKeys.editAria', { name: k.name })}
              onClick={() => editor.edit(k)}
            />
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('apiKeys.revokeAria', { name: k.name })}
              title={t('apiKeys.revoke')}
              className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              onClick={() => setRevoking(k)}
            >
              <BanIcon aria-hidden="true" />
            </Button>
          </div>
        );
      },
    },
  ];

  return (
    <ConfigPage
      title={t('apiKeys.title')}
      description={t('apiKeys.description')}
      actions={
        <Button onClick={editor.create}>
          <PlusIcon aria-hidden="true" />
          {t('apiKeys.new')}
        </Button>
      }
    >
      <Alert>
        <InfoIcon aria-hidden="true" />
        <AlertDescription>
          <p>
            {t('apiKeys.usage')}{' '}
            <code className="font-mono break-all text-foreground">
              Authorization: Bearer &lt;…&gt;
            </code>
          </p>
        </AlertDescription>
      </Alert>
      {keys.error ? (
        <QueryState isLoading={false} error={keys.error} onRetry={() => void keys.refetch()} />
      ) : (
        <DataTable
          caption={t('apiKeys.title')}
          columns={columns}
          data={keys.data ?? []}
          loading={keys.isPending}
          getRowId={(k) => k.id}
          searchable
          toolbar={
            <div className="flex items-center gap-2">
              <Switch
                id="api-keys-revoked"
                checked={includeRevoked}
                onCheckedChange={setIncludeRevoked}
              />
              <Label htmlFor="api-keys-revoked">{t('apiKeys.showRevoked')}</Label>
            </div>
          }
          empty={
            <EmptyState
              icon={KeyRoundIcon}
              title={t('apiKeys.empty')}
              description={t('apiKeys.emptyHint')}
              bordered={false}
            />
          }
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('apiKeys.edit') : t('apiKeys.new')}
        description={t('apiKeys.formHint')}
      >
        <ApiKeyForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          apiKey={editor.item}
          onCreated={(created) => {
            editor.close();
            setSecret(created);
          }}
          onDone={editor.close}
        />
      </FormDialog>
      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => !open && setRevoking(null)}
        title={t('apiKeys.revokeTitle', { name: revoking?.name ?? '' })}
        description={t('apiKeys.revokeConfirm')}
        confirmLabel={t('apiKeys.revoke')}
        destructive
        errorText={problemText}
        onConfirm={async () => {
          if (!revoking) return;
          await revoke.mutateAsync(revoking.id);
          toast.success(t('apiKeys.revoked'));
        }}
      />
      <SecretDialog
        secret={secret}
        title={t('apiKeys.createdTitle')}
        description={t('apiKeys.createdDescription')}
        label={t('apiKeys.fields.key')}
        hint={t('apiKeys.createdHint')}
        onClose={() => setSecret(null)}
      />
    </ConfigPage>
  );
}

function ApiKeyForm({
  establishmentId,
  apiKey,
  onCreated,
  onDone,
}: {
  establishmentId: string;
  apiKey: ApiKey | undefined;
  /** Receives the full key, shown once. */
  onCreated: (secret: string) => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('integrations');
  const create = useCreateApiKey(establishmentId);
  const update = useUpdateApiKey(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const [today] = useState(todayIso);
  const form = useForm<ApiKeyValues>({
    resolver: zodResolver(apiKeySchema(today)),
    defaultValues: apiKeyDefaults(apiKey),
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      if (apiKey) {
        const patch = toApiKeyUpdate(apiKey, values);
        if (Object.keys(patch).length > 0) await update.mutateAsync({ id: apiKey.id, patch });
        toast.success(t('saved'));
        onDone();
      } else {
        const res = await create.mutateAsync(toApiKeyCreate(values));
        onCreated(res.secret);
      }
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(values) }));
    }
  });
  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      <FormAlert state={alert} />
      <div className="grid gap-5 sm:grid-cols-2">
        <Field
          id="key-name"
          label={t('apiKeys.fields.name')}
          error={errors.name}
          hint={t('apiKeys.nameHint')}
        >
          <Input
            {...fieldAria('key-name', errors.name, true)}
            autoComplete="off"
            {...register('name')}
          />
        </Field>
        <Field
          id="key-expiry"
          label={t('apiKeys.fields.expires')}
          error={errors.expires_on}
          hint={t('apiKeys.expiryHint')}
        >
          <Controller
            control={control}
            name="expires_on"
            render={({ field }) => (
              <DatePicker
                id="key-expiry"
                value={field.value || undefined}
                onChange={(v) => field.onChange(v ?? '')}
                min={today}
                clearable
                aria-describedby="key-expiry-hint"
                aria-invalid={errors.expires_on ? true : undefined}
              />
            )}
          />
        </Field>
      </div>
      <Controller
        control={control}
        name="scopes"
        render={({ field }) => (
          <FieldSet>
            <FieldLegend variant="label">{t('apiKeys.fields.scopes')}</FieldLegend>
            <FieldDescription>{t('apiKeys.scopesHint')}</FieldDescription>
            <FieldGroup data-slot="checkbox-group" className="grid gap-3 sm:grid-cols-2">
              {API_KEY_SCOPES.map((scope) => {
                const id = `scope-${scope.replace('.', '-')}`;
                return (
                  <UiField key={scope} orientation="horizontal">
                    <Checkbox
                      id={id}
                      checked={field.value.includes(scope)}
                      onCheckedChange={(v) =>
                        field.onChange(
                          v === true
                            ? [...field.value, scope]
                            : field.value.filter((s) => s !== scope),
                        )
                      }
                    />
                    <FieldContent>
                      <FieldLabel htmlFor={id}>{t(`scopes.${scope}.label`)}</FieldLabel>
                      <FieldDescription>{t(`scopes.${scope}.description`)}</FieldDescription>
                    </FieldContent>
                  </UiField>
                );
              })}
            </FieldGroup>
            <FieldError id="key-scopes" error={errorAt(errors, 'scopes')} />
          </FieldSet>
        )}
      />
      <FormActions
        onCancel={onDone}
        pending={create.isPending || update.isPending}
        submitLabel={apiKey ? undefined : t('apiKeys.create')}
      />
    </form>
  );
}
