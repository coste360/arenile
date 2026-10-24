import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Field, FieldContent, FieldDescription, FieldLabel } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { ConfirmDialog, QueryState, SectionCard, StatCard } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import {
  CopyIcon,
  ExternalLinkIcon,
  FlaskConicalIcon,
  LinkIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
import {
  useCreatePreviewLink,
  usePurgeTestBookings,
  useTestMode,
  useUpdateTestMode,
  type TestBookingsPurgeResult,
  type TestMode,
  type TestModePreviewLink,
} from './api';

/**
 * Configurazione → Modalità prova (spec 15.4, ADR 0019): the switch, the
 * preview link of the public page and the bulk purge of the test bookings.
 * The persistent banner while test mode is on lives in the shell header
 * (TestModeBanner in AppShell).
 */
export function TestModePage() {
  const { t } = useTranslation('testMode');
  const ctx = useConfigContext();
  const query = useTestMode(ctx.establishmentId);
  const readOnly = !!ctx.establishment && !ctx.canConfigure;
  return (
    <ConfigPage title={t('page.title')} description={t('page.description')} readOnly={readOnly}>
      {!query.data ? (
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
          rows={4}
        />
      ) : (
        <TestModeSections
          establishmentId={ctx.establishmentId}
          state={query.data}
          readOnly={!ctx.canConfigure}
        />
      )}
    </ConfigPage>
  );
}

function TestModeSections({
  establishmentId,
  state,
  readOnly,
}: {
  establishmentId: string;
  state: TestMode;
  readOnly: boolean;
}) {
  const { t } = useTranslation('testMode');
  const problemText = useProblemText();
  const fmt = useInstantFormat();
  const update = useUpdateTestMode(establishmentId);

  const save = (enabled: boolean, publicEnabled: boolean) =>
    update.mutate(
      { version: state.version, body: { enabled, public_enabled: enabled && publicEnabled } },
      {
        onSuccess: (s) =>
          toast.success(
            s.enabled === state.enabled
              ? t('switch.savedPublic')
              : s.enabled
                ? t('switch.enabledToast')
                : t('switch.disabledToast'),
          ),
        onError: (e) => toast.error(problemText(e)),
      },
    );

  return (
    <div className="grid gap-6">
      <SectionCard
        icon={FlaskConicalIcon}
        title={t('switch.title')}
        description={t('switch.description')}
        actions={
          state.enabled ? (
            <Badge variant="outline">{t('switch.on')}</Badge>
          ) : (
            <Badge variant="secondary">{t('switch.off')}</Badge>
          )
        }
      >
        <div className="grid gap-4">
          <Field
            orientation="horizontal"
            data-disabled={readOnly || update.isPending || undefined}
            className="rounded-lg border p-3"
          >
            <FieldContent>
              <FieldLabel htmlFor="test-mode-enabled">{t('switch.enabled')}</FieldLabel>
              <FieldDescription id="test-mode-enabled-hint">
                {state.enabled && state.enabled_at
                  ? t('switch.enabledSince', { when: fmt.dateTime(state.enabled_at) })
                  : t('switch.enabledHint')}
              </FieldDescription>
            </FieldContent>
            <Switch
              id="test-mode-enabled"
              aria-describedby="test-mode-enabled-hint"
              checked={state.enabled}
              disabled={readOnly || update.isPending}
              onCheckedChange={(v) => save(v, false)}
            />
          </Field>
          <Field
            orientation="horizontal"
            data-disabled={readOnly || !state.enabled || update.isPending || undefined}
            className="rounded-lg border p-3"
          >
            <FieldContent>
              <FieldLabel htmlFor="test-mode-public">{t('switch.public')}</FieldLabel>
              <FieldDescription id="test-mode-public-hint">
                {t('switch.publicHint')}
              </FieldDescription>
            </FieldContent>
            <Switch
              id="test-mode-public"
              aria-describedby="test-mode-public-hint"
              checked={state.public_enabled}
              disabled={readOnly || !state.enabled || update.isPending}
              onCheckedChange={(v) => save(true, v)}
            />
          </Field>
          {state.public_enabled && (
            <Alert variant="warning">
              <TriangleAlertIcon aria-hidden="true" />
              <AlertTitle>{t('switch.publicWarningTitle')}</AlertTitle>
              <AlertDescription>{t('switch.publicWarning')}</AlertDescription>
            </Alert>
          )}
          <Alert variant={state.stripe_test_configured ? 'success' : 'default'} role="note">
            <FlaskConicalIcon aria-hidden="true" />
            <AlertTitle>
              {state.stripe_test_configured
                ? t('stripe.configuredTitle')
                : t('stripe.missingTitle')}
            </AlertTitle>
            <AlertDescription>
              {state.stripe_test_configured ? t('stripe.configured') : t('stripe.missing')}
            </AlertDescription>
          </Alert>
        </div>
      </SectionCard>

      <PreviewSection
        establishmentId={establishmentId}
        enabled={state.enabled}
        readOnly={readOnly}
      />
      <PurgeSection establishmentId={establishmentId} state={state} readOnly={readOnly} />
    </div>
  );
}

function PreviewSection({
  establishmentId,
  enabled,
  readOnly,
}: {
  establishmentId: string;
  enabled: boolean;
  readOnly: boolean;
}) {
  const { t } = useTranslation('testMode');
  const problemText = useProblemText();
  const fmt = useInstantFormat();
  const create = useCreatePreviewLink(establishmentId);
  const [link, setLink] = useState<TestModePreviewLink | null>(null);

  const copy = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      toast.success(t('preview.copied'));
    } catch {
      toast.error(t('preview.copyFailed'));
    }
  };

  return (
    <SectionCard
      icon={LinkIcon}
      title={t('preview.title')}
      description={t('preview.description')}
      actions={
        <Button
          variant="outline"
          disabled={!enabled || readOnly || create.isPending}
          onClick={() =>
            create.mutate(undefined, {
              onSuccess: setLink,
              onError: (e) => toast.error(problemText(e)),
            })
          }
        >
          {create.isPending ? <Spinner /> : <LinkIcon aria-hidden="true" />}
          {link ? t('preview.renew') : t('preview.create')}
        </Button>
      }
    >
      {!enabled ? (
        <p className="text-sm text-muted-foreground">{t('preview.needsEnabled')}</p>
      ) : link && enabled ? (
        <div className="grid gap-2">
          <div className="flex min-w-0 flex-wrap gap-2">
            <Input
              readOnly
              value={link.url}
              aria-label={t('preview.urlLabel')}
              className="min-w-0 flex-1 basis-60 font-mono"
              onFocus={(e) => e.currentTarget.select()}
            />
            <Button variant="outline" onClick={() => void copy(link.url)}>
              <CopyIcon aria-hidden="true" />
              {t('preview.copy')}
            </Button>
            <Button variant="outline" asChild>
              <a href={link.url} target="_blank" rel="noopener noreferrer">
                <ExternalLinkIcon aria-hidden="true" />
                {t('preview.open')}
              </a>
            </Button>
          </div>
          <p className="text-sm text-muted-foreground">
            {t('preview.expires', { when: fmt.dateTime(link.expires_at) })}
          </p>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('preview.hint')}</p>
      )}
    </SectionCard>
  );
}

function PurgeSection({
  establishmentId,
  state,
  readOnly,
}: {
  establishmentId: string;
  state: TestMode;
  readOnly: boolean;
}) {
  const { t } = useTranslation('testMode');
  const problemText = useProblemText();
  const purge = usePurgeTestBookings(establishmentId);
  const [preview, setPreview] = useState<TestBookingsPurgeResult | null>(null);

  const ask = () =>
    purge.mutate(
      { dryRun: true },
      { onSuccess: setPreview, onError: (e) => toast.error(problemText(e)) },
    );

  return (
    <SectionCard
      icon={Trash2Icon}
      title={t('purge.title')}
      description={t('purge.description')}
      actions={
        <Button
          variant="destructive"
          disabled={readOnly || state.test_bookings_count === 0 || purge.isPending}
          onClick={ask}
        >
          {purge.isPending && !preview ? <Spinner /> : <Trash2Icon aria-hidden="true" />}
          {t('purge.action')}
        </Button>
      }
    >
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label={t('purge.count')}
          value={state.test_bookings_count}
          icon={FlaskConicalIcon}
        />
      </div>
      <ConfirmDialog
        open={!!preview}
        onOpenChange={(open) => !open && setPreview(null)}
        destructive
        title={t('purge.confirmTitle')}
        description={
          preview
            ? t('purge.confirmDescription', {
                bookings: preview.bookings,
                payments: preview.payments,
              })
            : undefined
        }
        confirmLabel={t('purge.confirm')}
        errorText={problemText}
        onConfirm={() =>
          purge.mutateAsync({ dryRun: false }).then((res) => {
            toast.success(t('purge.done', { bookings: res.bookings, payments: res.payments }));
            setPreview(null);
          })
        }
      />
    </SectionCard>
  );
}
