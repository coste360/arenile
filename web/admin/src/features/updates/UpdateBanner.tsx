import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { DownloadIcon, ExternalLinkIcon, SparklesIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSystemUpdate } from './api';
import { browserStorage, readDismissed, upgradeDocsUrl, writeDismissed } from './lib';

/**
 * "New version available" notice of the shell (ADR 0025), for owners and
 * managers only. A polite status region (not a modal, not an alert): it
 * never interrupts the cash desk. Dismissing it remembers that version, so
 * it comes back for the next one. A failed check never shows anything here.
 */
export function UpdateBanner({ storage = browserStorage() }: { storage?: Storage }) {
  const { t, i18n } = useTranslation('updates');
  const query = useSystemUpdate();
  const [dismissed, setDismissed] = useState(() => readDismissed(storage));
  const u = query.data;
  if (!u?.update_available || !u.latest || dismissed === u.latest) return null;
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
            {u.how_to_update === 'docker' ? (
              <Button asChild size="sm">
                <a href={upgradeDocsUrl(i18n.language)} target="_blank" rel="noreferrer">
                  <ExternalLinkIcon aria-hidden="true" />
                  {t('banner.howTo')}
                </a>
              </Button>
            ) : (
              u.download_url && (
                <Button asChild size="sm">
                  <a href={u.download_url} target="_blank" rel="noreferrer">
                    <DownloadIcon aria-hidden="true" />
                    {t('banner.download')}
                  </a>
                </Button>
              )
            )}
            {u.release_notes_url && (
              <Button asChild size="sm" variant="outline">
                <a href={u.release_notes_url} target="_blank" rel="noreferrer">
                  {t('banner.notes')}
                </a>
              </Button>
            )}
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
