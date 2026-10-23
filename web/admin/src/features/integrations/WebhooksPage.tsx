import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { Input } from '@arenile/shared/ui/input';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import {
  ConfirmDialog,
  DataTable,
  DataTableColumnHeader,
  EmptyState,
  FormActions,
  FormDialog,
  MultiSelect,
  QueryState,
  RowActions,
  StatusBadge,
  type ColumnDef,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import {
  CircleCheckIcon,
  CircleXIcon,
  KeyRoundIcon,
  ListIcon,
  LockIcon,
  MoreHorizontalIcon,
  PlusIcon,
  SendIcon,
  ShieldCheckIcon,
  Trash2Icon,
  WebhookIcon,
  XIcon,
} from 'lucide-react';
import { useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { useEditor } from '../configuration/api/useCrud';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { BooleanField, Field } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { errorAt, fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import {
  useCreateWebhookEndpoint,
  useDeleteWebhookEndpoint,
  useRotateWebhookSecret,
  useSendWebhookTest,
  useUpdateWebhookEndpoint,
  useWebhookEndpoints,
  type WebhookDelivery,
  type WebhookEndpoint,
  type WebhookEndpointStatus,
} from './api';
import { DeliveriesSheet } from './components/DeliveriesSheet';
import { SecretDialog } from './components/SecretDialog';
import { useEventLabel } from './components/useEventLabel';
import { EVENT_FILTER_ORDER, nextEventSelection } from './lib/events';
import {
  toWebhookCreate,
  toWebhookUpdate,
  webhookDefaults,
  webhookSchema,
  type WebhookValues,
  redactWebhookUrl,
} from './lib/forms';

const ENDPOINT_TONE: Record<WebhookEndpointStatus, StatusTone> = {
  active: 'success',
  paused: 'neutral',
  disabled: 'danger',
};

const GRACE_HOURS = [0, 1, 24, 72, 168] as const;

/**
 * Configurazione → Webhook (owner only, users.manage; spec 18.2): endpoints
 * that receive signed events. Create/edit with an event picker, the signing
 * secret shown once (after creation and after rotation), a synchronous test
 * event and the delivery log with redelivery.
 */
export function WebhooksPage() {
  const { t } = useTranslation('integrations');
  const ctx = useConfigContext();
  const allowed = ctx.canManageUsers;
  const endpoints = useWebhookEndpoints(ctx.establishmentId, allowed);
  const remove = useDeleteWebhookEndpoint(ctx.establishmentId);
  const sendTest = useSendWebhookTest(ctx.establishmentId);
  const editor = useEditor<WebhookEndpoint>();
  const [secret, setSecret] = useState<{ value: string; rotated: boolean } | null>(null);
  const [rotating, setRotating] = useState<WebhookEndpoint | null>(null);
  // Dialog state lives here, not in the table cells: cells remount on every render.
  const [deleting, setDeleting] = useState<WebhookEndpoint | null>(null);
  const [logOf, setLogOf] = useState<WebhookEndpoint | null>(null);
  const [testResult, setTestResult] = useState<{
    endpoint: WebhookEndpoint;
    delivery: WebhookDelivery;
  } | null>(null);
  const fmt = useInstantFormat();
  const eventLabel = useEventLabel();
  const problemText = useProblemText();

  if (ctx.isLoading) {
    return (
      <ConfigPage title={t('webhooks.title')}>
        <QueryState isLoading error={null} />
      </ConfigPage>
    );
  }
  if (!allowed) {
    return (
      <ConfigPage title={t('webhooks.title')}>
        <EmptyState icon={LockIcon} title={t('ownerOnly')} />
      </ConfigPage>
    );
  }

  const runTest = (ep: WebhookEndpoint) => {
    setTestResult(null);
    sendTest.mutate(ep.id, {
      onSuccess: (delivery) => setTestResult({ endpoint: ep, delivery }),
      onError: (err) => toast.error(problemText(err)),
    });
  };

  const columns: ColumnDef<WebhookEndpoint>[] = [
    {
      accessorKey: 'url',
      meta: { label: t('webhooks.fields.url') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('webhooks.fields.url')} />
      ),
      cell: ({ row }) => (
        <span className="flex max-w-xs min-w-0 flex-col">
          <code className="truncate font-mono text-sm" title={redactWebhookUrl(row.original.url)}>
            {redactWebhookUrl(row.original.url)}
          </code>
          {row.original.description && (
            <span className="truncate text-xs text-muted-foreground">
              {row.original.description}
            </span>
          )}
        </span>
      ),
    },
    {
      id: 'events',
      accessorFn: (e) => e.events.join(' '),
      meta: { label: t('webhooks.fields.events') },
      header: t('webhooks.fields.events'),
      cell: ({ row }) => {
        const events = row.original.events;
        return (
          <span className="flex max-w-xs flex-wrap gap-1">
            {events.slice(0, 3).map((e) => (
              <Badge key={e} variant="secondary">
                {eventLabel(e)}
              </Badge>
            ))}
            {events.length > 3 && (
              <Badge variant="outline">{t('webhooks.moreEvents', { n: events.length - 3 })}</Badge>
            )}
          </span>
        );
      },
    },
    {
      accessorKey: 'status',
      meta: { label: t('webhooks.fields.status') },
      header: t('webhooks.fields.status'),
      cell: ({ row }) => {
        const ep = row.original;
        return (
          <span className="flex flex-col items-start gap-1">
            <StatusBadge tone={ENDPOINT_TONE[ep.status]}>
              {t(`webhooks.status.${ep.status}`)}
            </StatusBadge>
            {ep.status === 'disabled' && ep.disabled_reason && (
              <span className="max-w-48 text-xs text-muted-foreground">{ep.disabled_reason}</span>
            )}
            {ep.consecutive_failures > 0 && (
              <span className="text-xs text-muted-foreground">
                {t('webhooks.failures', { n: ep.consecutive_failures })}
              </span>
            )}
          </span>
        );
      },
    },
    {
      id: 'last_delivery',
      accessorFn: (e) => e.last_delivery_at ?? '',
      meta: { label: t('webhooks.fields.lastDelivery') },
      header: t('webhooks.fields.lastDelivery'),
      cell: ({ row }) => (
        <span className="whitespace-nowrap text-muted-foreground">
          {row.original.last_delivery_at ? fmt.dateTime(row.original.last_delivery_at) : t('never')}
        </span>
      ),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('actions')}</span>,
      cell: ({ row }) => {
        const ep = row.original;
        return (
          <RowActions
            editLabel={t('webhooks.editAria', { url: ep.url })}
            onEdit={() => editor.edit(ep)}
          >
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('webhooks.logAria', { url: ep.url })}
              title={t('webhooks.log')}
              onClick={() => setLogOf(ep)}
            >
              <ListIcon aria-hidden="true" />
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('webhooks.moreAria', { url: ep.url })}
                >
                  <MoreHorizontalIcon aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem disabled={sendTest.isPending} onSelect={() => runTest(ep)}>
                  <SendIcon aria-hidden="true" />
                  {t('webhooks.test')}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setRotating(ep)}>
                  <KeyRoundIcon aria-hidden="true" />
                  {t('webhooks.rotate')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onSelect={() => setDeleting(ep)}>
                  <Trash2Icon aria-hidden="true" />
                  {t('webhooks.delete')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </RowActions>
        );
      },
    },
  ];

  return (
    <ConfigPage
      title={t('webhooks.title')}
      description={t('webhooks.description')}
      actions={
        <Button onClick={editor.create}>
          <PlusIcon aria-hidden="true" />
          {t('webhooks.new')}
        </Button>
      }
    >
      <Alert>
        <ShieldCheckIcon aria-hidden="true" />
        <AlertTitle>{t('webhooks.signatureTitle')}</AlertTitle>
        <AlertDescription>
          <p>
            {t('webhooks.signatureHelp')}{' '}
            <code className="font-mono break-all text-foreground">Arenile-Signature: t=…,v1=…</code>
          </p>
          <p>{t('webhooks.signatureHow')}</p>
        </AlertDescription>
      </Alert>
      {testResult && (
        <TestResultAlert
          endpoint={testResult.endpoint}
          delivery={testResult.delivery}
          onOpenLog={() => setLogOf(testResult.endpoint)}
          onClose={() => setTestResult(null)}
        />
      )}
      {endpoints.error ? (
        <QueryState
          isLoading={false}
          error={endpoints.error}
          onRetry={() => void endpoints.refetch()}
        />
      ) : (
        <DataTable
          caption={t('webhooks.title')}
          columns={columns}
          data={endpoints.data ?? []}
          loading={endpoints.isPending}
          getRowId={(e) => e.id}
          empty={
            <EmptyState
              icon={WebhookIcon}
              title={t('webhooks.empty')}
              description={t('webhooks.emptyHint')}
              bordered={false}
            />
          }
        />
      )}
      <FormDialog
        open={editor.open}
        onOpenChange={editor.setOpen}
        wide
        title={editor.item ? t('webhooks.edit') : t('webhooks.new')}
        description={t('webhooks.formHint')}
      >
        <EndpointForm
          key={editor.key}
          establishmentId={ctx.establishmentId}
          endpoint={editor.item}
          onCreated={(value) => {
            editor.close();
            setSecret({ value, rotated: false });
          }}
          onDone={editor.close}
        />
      </FormDialog>
      <FormDialog
        open={rotating !== null}
        onOpenChange={(open) => !open && setRotating(null)}
        title={t('webhooks.rotate')}
        description={t('webhooks.rotateHint')}
      >
        {rotating && (
          <RotateForm
            establishmentId={ctx.establishmentId}
            endpoint={rotating}
            onRotated={(value) => {
              setRotating(null);
              setSecret({ value, rotated: true });
            }}
            onCancel={() => setRotating(null)}
          />
        )}
      </FormDialog>
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={t('webhooks.deleteTitle', { url: deleting?.url ?? '' })}
        description={t('webhooks.deleteConfirm')}
        confirmLabel={t('common.actions.delete', { ns: 'translation' })}
        destructive
        errorText={problemText}
        onConfirm={async () => {
          if (!deleting) return;
          await remove.mutateAsync(deleting.id);
          toast.success(t('webhooks.deleted'));
        }}
      />
      <SecretDialog
        secret={secret?.value ?? null}
        title={secret?.rotated ? t('webhooks.rotatedTitle') : t('webhooks.createdTitle')}
        description={
          secret?.rotated ? t('webhooks.rotatedDescription') : t('webhooks.createdDescription')
        }
        label={t('webhooks.fields.secret')}
        onClose={() => setSecret(null)}
      />
      <DeliveriesSheet
        establishmentId={ctx.establishmentId}
        endpoint={logOf}
        onOpenChange={(open) => !open && setLogOf(null)}
      />
    </ConfigPage>
  );
}

function TestResultAlert({
  endpoint,
  delivery,
  onOpenLog,
  onClose,
}: {
  endpoint: WebhookEndpoint;
  delivery: WebhookDelivery;
  onOpenLog: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('integrations');
  const ok = delivery.status === 'delivered';
  return (
    <Alert role="status" variant={ok ? 'default' : 'destructive'}>
      {ok ? <CircleCheckIcon aria-hidden="true" /> : <CircleXIcon aria-hidden="true" />}
      <AlertTitle className="pr-8">
        {ok
          ? t('webhooks.testOk', { url: endpoint.url })
          : t('webhooks.testFailed', { url: endpoint.url })}
      </AlertTitle>
      <AlertDescription>
        <p>
          {delivery.status_code
            ? t('webhooks.testCode', { code: delivery.status_code })
            : t('webhooks.testNoResponse')}
          {delivery.last_error ? ` · ${delivery.last_error}` : ''}
        </p>
        <Button variant="link" className="h-auto p-0" onClick={onOpenLog}>
          {t('webhooks.log')}
        </Button>
      </AlertDescription>
      <Button
        size="icon-sm"
        variant="ghost"
        className="absolute top-2 right-2"
        aria-label={t('common.close', { ns: 'translation' })}
        onClick={onClose}
      >
        <XIcon aria-hidden="true" />
      </Button>
    </Alert>
  );
}

function EndpointForm({
  establishmentId,
  endpoint,
  onCreated,
  onDone,
}: {
  establishmentId: string;
  endpoint: WebhookEndpoint | undefined;
  /** Receives the signing secret, shown once. */
  onCreated: (secret: string) => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('integrations');
  const eventLabel = useEventLabel();
  const create = useCreateWebhookEndpoint(establishmentId);
  const update = useUpdateWebhookEndpoint(establishmentId);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const form = useForm<WebhookValues>({
    resolver: zodResolver(webhookSchema),
    defaultValues: webhookDefaults(endpoint),
  });
  const {
    register,
    control,
    formState: { errors },
  } = form;
  const options = EVENT_FILTER_ORDER.map((f) => ({
    value: f,
    label: f.endsWith('*') ? eventLabel(f) : `${eventLabel(f)} (${f})`,
  }));
  const submit = form.handleSubmit(async (values) => {
    setAlert(null);
    try {
      if (endpoint) {
        const patch = toWebhookUpdate(endpoint, values);
        if (Object.keys(patch).length > 0) await update.mutateAsync({ id: endpoint.id, patch });
        toast.success(t('saved'));
        onDone();
      } else {
        const res = await create.mutateAsync(toWebhookCreate(values));
        onCreated(res.secret);
      }
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(values) }));
    }
  });
  return (
    <form noValidate onSubmit={submit} className="space-y-5">
      <FormAlert state={alert} />
      <Field
        id="wh-url"
        label={t('webhooks.fields.url')}
        error={errors.url}
        hint={t('webhooks.urlHint')}
      >
        <Input
          {...fieldAria('wh-url', errors.url, true)}
          type="url"
          inputMode="url"
          autoComplete="off"
          {...register('url')}
        />
      </Field>
      <Field
        id="wh-description"
        label={t('webhooks.fields.description')}
        error={errors.description}
      >
        <Input
          {...fieldAria('wh-description', errors.description)}
          autoComplete="off"
          {...register('description')}
        />
      </Field>
      <Field
        id="wh-events"
        label={t('webhooks.fields.events')}
        error={errorAt(errors, 'events')}
        hint={t('webhooks.eventsHint')}
      >
        <Controller
          control={control}
          name="events"
          render={({ field }) => (
            <MultiSelect
              id="wh-events"
              value={field.value}
              onChange={(next) => field.onChange(nextEventSelection(field.value, next))}
              options={options}
              placeholder={t('webhooks.eventsPlaceholder')}
              aria-describedby="wh-events-hint"
              aria-invalid={errors.events ? true : undefined}
            />
          )}
        />
      </Field>
      <BooleanField
        control={control}
        name="active"
        id="wh-active"
        variant="switch"
        label={t('webhooks.fields.active')}
        description={t('webhooks.activeHint')}
      />
      <FormActions
        onCancel={onDone}
        pending={create.isPending || update.isPending}
        submitLabel={endpoint ? undefined : t('webhooks.create')}
      />
    </form>
  );
}

function RotateForm({
  establishmentId,
  endpoint,
  onRotated,
  onCancel,
}: {
  establishmentId: string;
  endpoint: WebhookEndpoint;
  onRotated: (secret: string) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation('integrations');
  const rotate = useRotateWebhookSecret(establishmentId);
  const problemText = useProblemText();
  const [grace, setGrace] = useState<string>('24');
  const [error, setError] = useState<string | null>(null);
  return (
    <form
      noValidate
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault();
        setError(null);
        rotate.mutate(
          { id: endpoint.id, graceHours: Number(grace) },
          {
            onSuccess: (res) => onRotated(res.secret),
            onError: (err) => setError(problemText(err)),
          },
        );
      }}
    >
      {error && (
        <Alert variant="destructive">
          <CircleXIcon aria-hidden="true" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <p className="text-sm break-all">
        <code className="font-mono">{endpoint.url}</code>
      </p>
      <Field id="wh-grace" label={t('webhooks.grace')} hint={t('webhooks.graceHint')}>
        <NativeSelect
          id="wh-grace"
          aria-describedby="wh-grace-hint"
          value={grace}
          onChange={(e) => setGrace(e.target.value)}
        >
          {GRACE_HOURS.map((h) => (
            <NativeSelectOption key={h} value={String(h)}>
              {t(`webhooks.graceOption.h${h}`)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </Field>
      <FormActions
        onCancel={onCancel}
        pending={rotate.isPending}
        submitLabel={t('webhooks.rotateSubmit')}
      />
    </form>
  );
}
