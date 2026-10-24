import { zodResolver } from '@hookform/resolvers/zod';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { FieldLegend, FieldSet } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import {
  DataTable,
  EmptyState,
  QueryState,
  StatusBadge,
  type ColumnDef,
  type StatusTone,
} from '@arenile/shared/ui/patterns';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { Spinner } from '@arenile/shared/ui/spinner';
import { BellIcon, InboxIcon, MailIcon, SendIcon, ServerIcon } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { useId, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { BooleanField, Field } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import {
  useEmailSettings,
  useMessages,
  useSendTestEmail,
  useUpdateEmailSettings,
  type EmailSettings,
  type MessageStatus,
} from './api';
import { SectionCard } from './components/SectionCard';
import {
  CUSTOMER_NOTIFICATIONS,
  MANAGER_NOTIFICATIONS,
  emailSchema,
  testEmailSchema,
  toEmailInput,
  toEmailValues,
  type EmailValues,
} from './pages/email.schema';

const STATUS_TONE: Record<MessageStatus, StatusTone> = {
  queued: 'info',
  sending: 'info',
  sent: 'success',
  failed: 'danger',
  cancelled: 'neutral',
};
/** Reminders are configured in "Email ai clienti" (CustomerEmailsPage, ADR 0022). */
const REMINDERS = new Set<string>(['balance_reminder', 'day_before_reminder']);
type Message = NonNullable<ReturnType<typeof useMessages>['data']>['items'][number];

const STATUSES: MessageStatus[] = ['queued', 'sending', 'sent', 'failed', 'cancelled'];

function TransportCard({ settings }: { settings: EmailSettings }) {
  const { t } = useTranslation('online');
  return (
    <SectionCard
      id="mail-transport"
      icon={ServerIcon}
      title={t('email.transport.title')}
      description={t('email.transport.description')}
    >
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge tone={settings.transport === 'smtp' ? 'success' : 'warning'}>
            {t(`email.transport.${settings.transport}`)}
          </StatusBadge>
          <span className="text-sm text-muted-foreground">
            {t('email.transport.from', { from: settings.mail_from })}
          </span>
        </div>
        {settings.transport !== 'smtp' && (
          <Alert role="note">
            <ServerIcon aria-hidden="true" />
            <AlertTitle>{t('email.transport.noSmtpTitle')}</AlertTitle>
            <AlertDescription>
              {t(
                settings.transport === 'file'
                  ? 'email.transport.fileHint'
                  : 'email.transport.logHint',
              )}
            </AlertDescription>
          </Alert>
        )}
      </div>
    </SectionCard>
  );
}

function TestEmail({ establishmentId, disabled }: { establishmentId: string; disabled: boolean }) {
  const { t } = useTranslation('online');
  const send = useSendTestEmail(establishmentId);
  const form = useForm<{ to: string }>({
    resolver: zodResolver(testEmailSchema),
    defaultValues: { to: '' },
  });
  const error = form.formState.errors.to;
  return (
    <form
      noValidate
      className="flex flex-wrap items-end gap-2"
      onSubmit={form.handleSubmit(async (v) => {
        try {
          await send.mutateAsync(v);
          toast.success(t('email.test.queued'));
          form.reset({ to: '' });
        } catch {
          toast.error(t('email.test.failed'));
        }
      })}
    >
      <Field id="mail-test-to" label={t('email.test.to')} error={error} className="min-w-64 flex-1">
        <Input
          {...fieldAria('mail-test-to', error)}
          type="email"
          autoComplete="email"
          disabled={disabled}
          {...form.register('to')}
        />
      </Field>
      <Button type="submit" variant="outline" disabled={disabled || send.isPending}>
        {send.isPending ? <Spinner /> : <SendIcon aria-hidden="true" />}
        {t('email.test.send')}
      </Button>
    </form>
  );
}

function MessageLog({ establishmentId }: { establishmentId: string }) {
  const { t } = useTranslation('online');
  const fmt = useInstantFormat();
  const id = useId();
  const [status, setStatus] = useState<MessageStatus | ''>('');
  const query = useMessages(establishmentId, status || undefined);
  const items = query.data?.items ?? [];
  const columns: ColumnDef<Message>[] = [
    {
      id: 'date',
      header: t('email.log.date'),
      meta: { label: t('email.log.date') },
      cell: ({ row }) => (
        <span className="whitespace-nowrap tabular-nums">
          {fmt.dateTime(row.original.sent_at ?? row.original.created_at)}
        </span>
      ),
    },
    {
      id: 'template',
      header: t('email.log.template'),
      meta: { label: t('email.log.template') },
      cell: ({ row }) => (
        <>
          {t(`email.templates.${row.original.template}`, { defaultValue: row.original.template })}
          <span className="ml-1 text-xs text-muted-foreground uppercase">
            {row.original.locale}
          </span>
        </>
      ),
    },
    {
      id: 'recipient',
      header: t('email.log.recipient'),
      meta: { label: t('email.log.recipient') },
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.recipient_masked}</span>,
    },
    {
      id: 'booking',
      header: t('email.log.booking'),
      meta: { label: t('email.log.booking') },
      cell: ({ row }) => (
        <span className="font-mono text-xs">{row.original.booking_code ?? '—'}</span>
      ),
    },
    {
      id: 'status',
      header: t('email.log.status'),
      meta: { label: t('email.log.status') },
      cell: ({ row }) => {
        const m = row.original;
        return (
          <>
            <StatusBadge tone={STATUS_TONE[m.status]}>{t(`email.status.${m.status}`)}</StatusBadge>
            {m.status === 'failed' && m.last_error && (
              <p
                className="mt-1 max-w-64 truncate text-xs text-muted-foreground"
                title={m.last_error}
              >
                {t('email.log.attempts', { count: m.attempts })} · {m.last_error}
              </p>
            )}
          </>
        );
      },
    },
  ];
  return (
    <SectionCard
      id="mail-log"
      icon={InboxIcon}
      title={t('email.log.title')}
      description={t('email.log.description')}
      flush
    >
      <DataTable
        embedded
        caption={t('email.log.title')}
        columns={columns}
        data={items}
        getRowId={(m) => m.id}
        columnToggle={false}
        loading={query.isPending}
        error={
          query.error ? (
            <QueryState
              isLoading={false}
              error={query.error}
              onRetry={() => void query.refetch()}
            />
          ) : undefined
        }
        filters={
          <div className="flex items-center gap-2">
            <label htmlFor={id} className="sr-only">
              {t('email.log.filter')}
            </label>
            <NativeSelect
              id={id}
              value={status}
              onChange={(e) => setStatus(e.target.value as MessageStatus | '')}
            >
              <NativeSelectOption value="">{t('email.log.all')}</NativeSelectOption>
              {STATUSES.map((s) => (
                <NativeSelectOption key={s} value={s}>
                  {t(`email.status.${s}`)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
        }
        empty={
          <EmptyState
            bordered={false}
            icon={InboxIcon}
            title={t('email.log.empty')}
            description={t('email.log.emptyHint')}
          />
        }
      />
    </SectionCard>
  );
}

function EmailForm({
  settings,
  establishmentId,
  readOnly,
}: {
  settings: EmailSettings;
  establishmentId: string;
  readOnly: boolean;
}) {
  const { t } = useTranslation('online');
  const { t: tg } = useTranslation();
  const [version, setVersion] = useState(settings.version);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const update = useUpdateEmailSettings(establishmentId);
  const query = useEmailSettings(establishmentId);
  const form = useForm<EmailValues>({
    resolver: zodResolver(emailSchema),
    defaultValues: toEmailValues(settings),
  });
  const errors = form.formState.errors;
  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      const saved = await update.mutateAsync({ version, body: toEmailInput(v) });
      setVersion(saved.version);
      form.reset(toEmailValues(saved));
      toast.success(t('saved'));
    } catch (err) {
      setAlert(problemToFormState(err, form.setError, { formKeys: Object.keys(form.getValues()) }));
    }
  });
  const reload = async () => {
    const res = await query.refetch();
    if (res.data) {
      setVersion(res.data.version);
      form.reset(toEmailValues(res.data));
      setAlert(null);
    }
  };
  return (
    <form noValidate onSubmit={submit}>
      <SectionCard
        id="mail-settings"
        icon={MailIcon}
        title={t('email.settings.title')}
        description={t('email.settings.description')}
        footer={
          readOnly ? undefined : (
            <>
              {form.formState.isDirty && (
                <span className="mr-auto text-sm text-muted-foreground">{t('unsaved')}</span>
              )}
              <Button type="submit" disabled={update.isPending}>
                {update.isPending && <Spinner />}
                {tg('common.actions.save')}
              </Button>
            </>
          )
        }
      >
        <div className="space-y-6">
          <FormAlert state={alert} onReload={() => void reload()} />
          <div className="grid gap-5 sm:grid-cols-3">
            <Field
              id="mail-sender"
              label={t('email.settings.senderName')}
              error={errors.sender_name}
            >
              <Input
                {...fieldAria('mail-sender', errors.sender_name)}
                disabled={readOnly}
                {...form.register('sender_name')}
              />
            </Field>
            <Field
              id="mail-reply"
              label={t('email.settings.replyTo')}
              error={errors.reply_to}
              hint={t('email.settings.replyToHint')}
            >
              <Input
                {...fieldAria('mail-reply', errors.reply_to, true)}
                type="email"
                disabled={readOnly}
                {...form.register('reply_to')}
              />
            </Field>
            <Field
              id="mail-manager"
              label={t('email.settings.managerEmail')}
              error={errors.manager_email}
              hint={t('email.settings.managerEmailHint')}
            >
              <Input
                {...fieldAria('mail-manager', errors.manager_email, true)}
                type="email"
                disabled={readOnly}
                {...form.register('manager_email')}
              />
            </Field>
          </div>
          <div className="grid gap-6 lg:grid-cols-2">
            <FieldSet className="gap-3">
              <FieldLegend variant="label">{t('email.settings.customer')}</FieldLegend>
              {CUSTOMER_NOTIFICATIONS.filter((k) => !REMINDERS.has(k)).map((k) => (
                <BooleanField
                  key={k}
                  control={form.control}
                  name={`customer_notifications.${k}`}
                  id={`mail-c-${k}`}
                  variant="switch"
                  disabled={readOnly}
                  label={t(`email.customer.${k}`)}
                />
              ))}
              <p className="text-sm text-muted-foreground">
                <Link
                  to="/e/$establishmentId/config/customer-emails"
                  params={{ establishmentId }}
                  className="underline underline-offset-4"
                >
                  {t('email.customer.remindersMoved')}
                </Link>
              </p>
            </FieldSet>
            <FieldSet className="gap-3">
              <FieldLegend variant="label">
                <BellIcon aria-hidden="true" className="mr-1 inline size-4" />
                {t('email.settings.manager')}
              </FieldLegend>
              {MANAGER_NOTIFICATIONS.map((k) => (
                <BooleanField
                  key={k}
                  control={form.control}
                  name={`manager_notifications.${k}`}
                  id={`mail-m-${k}`}
                  variant="switch"
                  disabled={readOnly}
                  label={t(`email.manager.${k}`)}
                />
              ))}
            </FieldSet>
          </div>
        </div>
      </SectionCard>
    </form>
  );
}

/**
 * "Email e notifiche" (spec 19): transport from the server configuration
 * (SMTP, `.eml` files or log; read-only with guidance), sender name,
 * reply-to, manager address, customer and manager notifications, a test
 * email and the message log with masked recipients. API: GET/PUT
 * email-settings (If-Match), POST email-settings/test, GET messages.
 */
export function EmailSettingsPage() {
  const { t } = useTranslation('online');
  const ctx = useConfigContext();
  const est = ctx.establishment;
  const query = useEmailSettings(ctx.establishmentId);
  const readOnly = !ctx.canConfigure;
  return (
    <ConfigPage
      title={t('email.title')}
      description={t('email.description')}
      readOnly={!!est && readOnly}
    >
      {!query.data || !est ? (
        <QueryState
          isLoading={query.isPending || ctx.isLoading}
          error={query.error ?? ctx.error}
          onRetry={() => void query.refetch()}
          rows={6}
        />
      ) : (
        <div className="space-y-6">
          <TransportCard settings={query.data} />
          <EmailForm
            key={est.id}
            settings={query.data}
            establishmentId={est.id}
            readOnly={readOnly}
          />
          <SectionCard
            id="mail-test"
            icon={SendIcon}
            title={t('email.test.title')}
            description={t('email.test.description')}
          >
            <TestEmail establishmentId={est.id} disabled={readOnly} />
          </SectionCard>
          <MessageLog establishmentId={est.id} />
        </div>
      )}
    </ConfigPage>
  );
}
