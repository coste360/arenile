import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Link, useRouterState } from '@tanstack/react-router';
import { SparklesIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSystemUpdate } from './api';
import { browserStorage, readDismissed, writeDismissed } from './lib';
import { UpdateActions } from './UpdateActions';

/**
 * "New version available" notice of the shell (ADR 0025), for owners and
 * managers only. A polite status region (not a modal, not an alert): it
 * never interrupts the cash desk. Dismissing it remembers that version, so
 * it comes back for the next one. A failed check never shows anything here.
 * Its actions are the ones of "Informazioni su Arenile" (UpdateActions),
 * which it links for the details.
 */
export function UpdateBanner({ storage = browserStorage() }: { storage?: Storage }) {
  const { t } = useTranslation('updates');
  const query = useSystemUpdate();
  const [dismissed, setDismissed] = useState(() => readDismissed(storage));
  // "Informazioni su Arenile" shows the same actions in full.
  const onAbout = useRouterState({
    select: (s) => /\/informazioni\/?$/.test(s.location.pathname),
  });
  const u = query.data;
  if (!u?.update_available || !u.latest || dismissed === u.latest || onAbout) return null;
  const latest = u.latest;

  const dismiss = () => {
    writeDismissed(storage, latest);
    setDismissed(latest);
  };

  return (
    <div className="px-4 pt-4 md:px-6 print:hidden" data-testid="update-banner">
      <Alert role="status" aria-live="polite">
        <SparklesIcon aria-hidden="true" />
        <AlertTitle className="pr-8">{t('banner.title', { latest })}</AlertTitle>
        <AlertDescription>
          <p>{t('banner.current', { current: u.current })}</p>
          <div className="flex flex-wrap items-center gap-2 pt-1">
            <UpdateActions u={u} size="sm" />
            <Button asChild size="sm" variant="ghost">
              <Link to="/informazioni">{t('banner.details')}</Link>
            </Button>
          </div>
        </AlertDescription>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="absolute top-2 right-2 size-7"
          onClick={dismiss}
          aria-label={t('banner.dismiss')}
        >
          <XIcon aria-hidden="true" />
        </Button>
      </Alert>
    </div>
  );
}
