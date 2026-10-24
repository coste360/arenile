import { zodResolver } from '@hookform/resolvers/zod';
import { useMe } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { QueryState, SegmentedControl, StatusBadge } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  BanknoteIcon,
  CircleCheckIcon,
  CircleXIcon,
  FlaskConicalIcon,
  KeyRoundIcon,
  PlugZapIcon,
  WebhookIcon,
} from 'lucide-react';
import { useState } from 'react';
import { Controller, useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
import { BooleanField, Field, SelectField } from '../configuration/components/fields';
import { FormAlert } from '../configuration/components/FormAlert';
import { fieldAria } from '../configuration/lib/aria';
import { problemToFormState, type FormAlertState } from '../configuration/lib/formErrors';
import {
  usePaymentSettings,
  useTestPaymentSettings,
  useUpdatePaymentSettings,
  type PaymentSettings,
} from './api';
import { CodeBlock } from './components/CodeBlock';
import { LivePaymentsBlockedAlert } from './components/LivePaymentsBlockedAlert';
import { SectionCard } from './components/SectionCard';
import { hasPermission } from './lib/permissions';
import {
  STRIPE_EVENTS,
  paymentSchema,
  toPaymentInput,
  toPaymentValues,
  type PaymentValues,
} from './pages/payments.schema';

function SecretField({
  id,
  label,
  hint,
  configured,
  configuredText,
  clearName,
  form,
  name,
  readOnly,
}: {
  id: string;
  label: string;
  hint: string;
  configured: boolean;
  configuredText: string;
  clearName: 'clear_secret_key' | 'clear_webhook_secret';
  name: 'secret_key' | 'webhook_secret';
  form: ReturnType<typeof useForm<PaymentValues>>;
  readOnly: boolean;
}) {
  const { t } = useTranslation('online');
  const clear = useWatch({ control: form.control, name: clearName });
  const error = form.formState.errors[name];
  return (
    <Field id={id} label={label} error={error} hint={hint}>
      <div className="flex gap-2">
        <Input
          {...fieldAria(id, error, true)}
          type="password"
          autoComplete="off"
          spellCheck={false}
          disabled={readOnly || clear}
          placeholder={configured && !clear ? configuredText : ''}
          className="font-mono"
          {...form.register(name)}
        />
        {configured && !readOnly && (
          <Button
            type="button"
            variant="outline"
            aria-pressed={clear}
            onClick={() => {
              form.setValue(clearName, !clear, { shouldDirty: true });
              form.setValue(name, '', { shouldValidate: true });
            }}
          >
            {clear ? t('payments.keep') : t('payments.remove')}
          </Button>
        )}
      </div>
    </Field>
  );
}

function PaymentForm({
  settings,
  establishmentId,
  readOnly,
}: {
  settings: PaymentSettings;
  establishmentId: string;
  readOnly: boolean;
}) {
  const { t } = useTranslation('online');
  const { t: tg } = useTranslation();
  const fmt = useInstantFormat();
  const [version, setVersion] = useState(settings.version);
  const [alert, setAlert] = useState<FormAlertState | null>(null);
  const update = useUpdatePaymentSettings(establishmentId);
  const test = useTestPaymentSettings(establishmentId);
  const query = usePaymentSettings(establishmentId);
  const current = query.data ?? settings;
  const form = useForm<PaymentValues>({
    resolver: zodResolver(paymentSchema),
    defaultValues: toPaymentValues(settings),
  });
  const provider = useWatch({ control: form.control, name: 'provider' });
  const mode = useWatch({ control: form.control, name: 'mode' });
  const errors = form.formState.errors;
  const stripe = current.stripe;

  const submit = form.handleSubmit(async (v) => {
    setAlert(null);
    try {
      const saved = await update.mutateAsync({ version, body: toPaymentInput(v) });
      setVersion(saved.version);
      form.reset(toPaymentValues(saved));
      toast.success(t('saved'));
    } catch (err) {
      setAlert(
        problemToFormState(err, form.setError, {
          formKeys: Object.keys(form.getValues()),
          aliases: {
            'stripe.publishable_key': 'publishable_key',
            'stripe.secret_key': 'secret_key',
            'stripe.webhook_secret': 'webhook_secret',
          },
        }),
      );
    }
  });

  const reload = async () => {
    const res = await query.refetch();
    if (res.data) {
      setVersion(res.data.version);
      form.reset(toPaymentValues(res.data));
      setAlert(null);
    }
  };

  const ready =
    current.provider === 'stripe' && stripe.configured && stripe.webhook_secret_configured;
  const configuredText = stripe.secret_key_last4
    ? t('payments.configuredLast4', { last4: stripe.secret_key_last4 })
    : t('payments.configured');

  return (
    <div className="space-y-6">
      <SectionCard
        id="pay-status"
        icon={PlugZapIcon}
        title={t('payments.status.title')}
        description={t('payments.status.description')}
        actions={
          current.provider !== 'manual' && !readOnly ? (
            <Button variant="outline" onClick={() => test.mutate()} disabled={test.isPending}>
              {test.isPending ? <Spinner /> : <PlugZapIcon aria-hidden="true" />}
              {t('payments.test')}
            </Button>
          ) : undefined
        }
      >
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">
              {t('payments.providerLabel', {
                provider: t(`payments.providers.${current.provider}`, {
                  defaultValue: current.provider,
                }),
              })}
            </Badge>
            <StatusBadge tone={current.mode === 'live' ? 'success' : 'warning'}>
              {t(`payments.modes.${current.mode}`)}
            </StatusBadge>
            {current.provider === 'stripe' && (
              <>
                <StatusBadge tone={stripe.configured ? 'success' : 'danger'}>
                  {stripe.configured ? configuredText : t('payments.secretMissing')}
                </StatusBadge>
                <StatusBadge tone={stripe.webhook_secret_configured ? 'success' : 'danger'}>
                  {stripe.webhook_secret_configured
                    ? t('payments.webhookConfigured')
                    : t('payments.webhookMissing')}
                </StatusBadge>
                {stripe.verified_at && (
                  <span className="text-sm text-muted-foreground">
                    {t('payments.verifiedAt', { date: fmt.dateTime(stripe.verified_at) })}
                  </span>
                )}
              </>
            )}
          </div>
          {current.live_payments_blocked && (
            <LivePaymentsBlockedAlert establishmentId={establishmentId} />
          )}
          {current.provider === 'manual' && (
            <p className="text-sm text-muted-foreground">{t('payments.manualHint')}</p>
          )}
          {current.provider === 'stripe' && !ready && (
            <Alert>
              <KeyRoundIcon aria-hidden="true" />
              <AlertDescription>{t('payments.incomplete')}</AlertDescription>
            </Alert>
          )}
          {test.data && (
            <Alert variant={test.data.ok ? 'default' : 'destructive'} role="status">
              {test.data.ok ? (
                <CircleCheckIcon aria-hidden="true" />
              ) : (
                <CircleXIcon aria-hidden="true" />
              )}
              <AlertTitle>
                {test.data.ok ? t('payments.testOk') : t('payments.testFailed')}
              </AlertTitle>
              <AlertDescription>
                {[test.data.account_label, test.data.message].filter(Boolean).join(' · ') ||
                  t(`payments.modes.${test.data.mode}`)}
              </AlertDescription>
            </Alert>
          )}
          {test.error && (
            <p role="alert" className="text-sm text-destructive">
              {tg('common.errors.generic')}
            </p>
          )}
        </div>
      </SectionCard>

      <form noValidate onSubmit={submit}>
        <SectionCard
          id="pay-provider"
          icon={BanknoteIcon}
          title={t('payments.provider.title')}
          description={t('payments.provider.description')}
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
          <div className="space-y-5">
            <FormAlert state={alert} onReload={() => void reload()} />
            <div className="grid gap-5 sm:grid-cols-2">
              <SelectField
                control={form.control}
                name="provider"
                id="pay-provider-select"
                label={t('payments.provider.label')}
                disabled={readOnly}
                options={current.available_providers.map((p) => ({
                  value: p,
                  label: t(`payments.providers.${p}`, { defaultValue: p }),
                }))}
              />
              <div className="space-y-2">
                <p aria-hidden="true" className="text-sm font-medium">
                  {t('payments.mode')}
                </p>
                <Controller
                  control={form.control}
                  name="mode"
                  render={({ field }) => (
                    <SegmentedControl
                      collapseOnNarrow
                      label={t('payments.mode')}
                      value={field.value}
                      onValueChange={field.onChange}
                      options={[
                        {
                          value: 'test',
                          icon: FlaskConicalIcon,
                          label: t('payments.modes.test'),
                          disabled: readOnly,
                        },
                        {
                          value: 'live',
                          icon: BanknoteIcon,
                          label: t('payments.modes.live'),
                          disabled: readOnly,
                        },
                      ]}
                    />
                  )}
                />
              </div>
            </div>
            {mode === 'live' && provider === 'stripe' && (
              <Alert role="note">
                <BanknoteIcon aria-hidden="true" />
                <AlertDescription>{t('payments.liveWarning')}</AlertDescription>
              </Alert>
            )}
            {provider === 'stripe' && (
              <div className="grid gap-5">
                <Field
                  id="pay-pk"
                  label={t('payments.publishableKey')}
                  error={errors.publishable_key}
                  hint={t('payments.publishableKeyHint', { mode })}
                >
                  <Input
                    {...fieldAria('pay-pk', errors.publishable_key, true)}
                    autoComplete="off"
                    spellCheck={false}
                    disabled={readOnly}
                    placeholder={`pk_${mode}_…`}
                    className="font-mono"
                    {...form.register('publishable_key')}
                  />
                </Field>
                <SecretField
                  id="pay-sk"
                  label={t('payments.secretKey')}
                  hint={t('payments.secretKeyHint', { mode })}
                  configured={stripe.configured}
                  configuredText={configuredText}
                  name="secret_key"
                  clearName="clear_secret_key"
                  form={form}
                  readOnly={readOnly}
                />
                <SecretField
                  id="pay-whsec"
                  label={t('payments.webhookSecret')}
                  hint={t('payments.webhookSecretHint')}
                  configured={stripe.webhook_secret_configured}
                  configuredText={t('payments.configured')}
                  name="webhook_secret"
                  clearName="clear_webhook_secret"
                  form={form}
                  readOnly={readOnly}
                />
                <BooleanField
                  control={form.control}
                  name="embedded_mode"
                  id="pay-embedded"
                  variant="switch"
                  disabled
                  label={t('payments.embedded')}
                  description={t('payments.embeddedHint')}
                />
              </div>
            )}
          </div>
        </SectionCard>
      </form>

      {provider === 'stripe' && (
        <SectionCard
          id="pay-webhook"
          icon={WebhookIcon}
          title={t('payments.webhook.title')}
          description={t('payments.webhook.description')}
        >
          <div className="space-y-4">
            <CodeBlock
              label={t('payments.webhook.url')}
              value={current.webhook_url}
              language="URL"
            />
            <div className="space-y-2 text-sm">
              <p className="font-medium">{t('payments.webhook.events')}</p>
              <ul className="flex flex-wrap gap-1.5">
                {STRIPE_EVENTS.map((e) => (
                  <li key={e}>
                    <Badge variant="secondary" className="font-mono">
                      {e}
                    </Badge>
                  </li>
                ))}
              </ul>
              <ol className="list-inside list-decimal space-y-1 text-muted-foreground">
                <li>{t('payments.webhook.step1')}</li>
                <li>{t('payments.webhook.step2')}</li>
                <li>{t('payments.webhook.step3')}</li>
              </ol>
            </div>
          </div>
        </SectionCard>
      )}
    </div>
  );
}

/**
 * "Pagamenti online" (spec 13): provider (manual / Stripe Checkout), test or
 * live mode, Stripe keys (write-only: only "configurata ••••4242" comes
 * back), the webhook URL to paste in the Stripe dashboard with the events to
 * enable, "verifica credenziali" and the status. Requires payments.configure.
 */
export function PaymentSettingsPage() {
  const { t } = useTranslation('online');
  const ctx = useConfigContext();
  const me = useMe();
  const est = ctx.establishment;
  const canConfigure = hasPermission(me.data, est, 'payments.configure');
  const query = usePaymentSettings(ctx.establishmentId);
  return (
    <ConfigPage
      title={t('payments.title')}
      description={t('payments.description')}
      readOnly={!!est && !!me.data && !canConfigure}
    >
      {!query.data || !est ? (
        <QueryState
          isLoading={query.isPending || ctx.isLoading}
          error={query.error ?? ctx.error}
          onRetry={() => void query.refetch()}
          rows={6}
        />
      ) : (
        <PaymentForm
          key={est.id}
          settings={query.data}
          establishmentId={est.id}
          readOnly={!canConfigure}
        />
      )}
    </ConfigPage>
  );
}
