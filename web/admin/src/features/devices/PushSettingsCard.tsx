import { ApiProblem, useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import { QueryState, SectionCard } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import { BellIcon, BellOffIcon } from 'lucide-react';
import { useId, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  usePushSettings,
  useUpdatePushSettings,
  type PushEvents,
  type PushSettings,
} from './pushApi';

const EVENTS = ['booking_new_online', 'booking_approval_requested', 'payment_failed'] as const;

/**
 * Notifiche push dell'app mobile (ADR 0027): the manager's opt-in, off by
 * default, and the events sent. Shown to users with
 * establishment.configure. Without the relay key on the instance the
 * switches are disabled and a note explains why.
 */
export function PushSettingsCard({ establishmentId }: { establishmentId: string }) {
  const settings = usePushSettings(establishmentId, true);
  if (settings.error) {
    // An instance without the operation (older server) shows nothing.
    if (settings.error instanceof ApiProblem && settings.error.status === 404) return null;
    return (
      <QueryState
        isLoading={false}
        error={settings.error}
        onRetry={() => void settings.refetch()}
      />
    );
  }
  if (!settings.data) return null;
  return (
    <PushSettingsForm
      key={settings.data.version}
      establishmentId={establishmentId}
      settings={settings.data}
    />
  );
}

function PushSettingsForm({
  establishmentId,
  settings,
}: {
  establishmentId: string;
  settings: PushSettings;
}) {
  const { t } = useTranslation('devices');
  const id = useId();
  const problemText = useProblemText();
  const save = useUpdatePushSettings(establishmentId);
  const [enabled, setEnabled] = useState(settings.enabled);
  const [events, setEvents] = useState<PushEvents>(settings.events);
  const relay = settings.relay_configured;
  const dirty =
    enabled !== settings.enabled || EVENTS.some((e) => events[e] !== settings.events[e]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await save.mutateAsync({ version: settings.version, body: { enabled, events } });
      toast.success(t('push.saved'));
    } catch (err) {
      toast.error(problemText(err));
    }
  };

  return (
    <form noValidate onSubmit={(e) => void submit(e)}>
      <SectionCard
        icon={enabled && relay ? BellIcon : BellOffIcon}
        title={t('push.title')}
        description={t('push.description')}
        footer={
          <Button type="submit" disabled={!relay || !dirty || save.isPending}>
            {save.isPending && <Spinner />}
            {t('push.save')}
          </Button>
        }
      >
        <div className="space-y-4">
          {!relay && (
            <Alert role="note">
              <BellOffIcon aria-hidden="true" />
              <AlertTitle>{t('push.noRelay.title')}</AlertTitle>
              <AlertDescription>{t('push.noRelay.description')}</AlertDescription>
            </Alert>
          )}
          <div className="flex items-start gap-3">
            <Switch
              id={`${id}-enabled`}
              checked={enabled}
              onCheckedChange={setEnabled}
              disabled={!relay}
            />
            <div className="min-w-0 space-y-1">
              <Label htmlFor={`${id}-enabled`}>{t('push.enabled.label')}</Label>
              <p className="text-sm text-muted-foreground">{t('push.enabled.help')}</p>
            </div>
          </div>
          <fieldset className="space-y-3 border-t pt-4" disabled={!relay || !enabled}>
            <legend className="mb-2 text-sm font-medium">{t('push.events.legend')}</legend>
            {EVENTS.map((ev) => (
              <div key={ev} className="flex items-start gap-3">
                <Switch
                  id={`${id}-${ev}`}
                  checked={events[ev]}
                  onCheckedChange={(v) => setEvents((cur) => ({ ...cur, [ev]: v }))}
                  disabled={!relay || !enabled}
                />
                <div className="min-w-0 space-y-1">
                  <Label htmlFor={`${id}-${ev}`}>{t(`push.events.${ev}.label`)}</Label>
                  <p className="text-sm text-muted-foreground">{t(`push.events.${ev}.help`)}</p>
                </div>
              </div>
            ))}
          </fieldset>
        </div>
      </SectionCard>
    </form>
  );
}
