import { useMe } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Page, SectionCard } from '@arenile/shared/ui/patterns';
import { ExternalLinkIcon, InfoIcon, RefreshCwIcon, SparklesIcon } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../establishment';
import { inDesktopWindow, useInstanceMeta } from '../../lib/meta';
import { canSeeUpdates, useCheckSystemUpdate, useSystemUpdate, type SystemUpdate } from './api';
import { DOWNLOADS_PAGE, upgradeDocsUrl } from './lib';
import { UpdateActions } from './UpdateActions';

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 sm:grid-cols-3 sm:gap-4">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium sm:col-span-2">{children}</dd>
    </div>
  );
}

/** The notification text of a finished manual check (toast). */
function checkResult(u: SystemUpdate, t: (key: string, opts?: Record<string, unknown>) => string) {
  if (u.status === 'disabled') {
    return u.check_enabled ? t('status.devBuild') : t('check.disabled');
  }
  if (u.update_available && u.latest) return t('check.available', { latest: u.latest });
  if (u.status === 'unavailable' || !u.latest) return t('status.unavailable');
  return t('status.upToDate');
}

/**
 * "Informazioni su Arenile" (/admin/informazioni): the one place with the
 * version in use, the edition, the update check (state, last check, latest
 * version, release notes), "Verifica aggiornamenti" and the download of the
 * new version for this system (desktop app: the package, via the browser;
 * Docker: the upgrade guide). Reached from the user menu, the update banner,
 * the desktop app menu and a click on the "new version" notification.
 * Everybody sees the version; the update status and actions are for owners
 * and managers (ADR 0025).
 */
export function AboutPage() {
  const { t, i18n } = useTranslation('updates');
  const me = useMe();
  const meta = useInstanceMeta();
  const query = useSystemUpdate();
  const check = useCheckSystemUpdate();
  const fmt = useInstantFormat();
  const allowed = canSeeUpdates(me.data);
  const u = query.data;

  const how = u?.how_to_update ?? (meta.data?.edition === 'desktop' ? 'desktop' : undefined);
  const edition =
    how === 'desktop'
      ? t('about.editionDesktop')
      : how === 'docker'
        ? t('about.editionDocker')
        : t('about.editionServer');
  const current = u?.current ?? meta.data?.version ?? '—';

  const onCheck = () =>
    check.mutate(undefined, {
      onSuccess: (s) => toast(checkResult(s, t)),
      onError: () => toast.error(t('check.failed')),
    });

  let lastCheck = t('about.never');
  if (u?.pending && u.check_enabled) lastCheck = t('about.pending');
  else if (u?.last_attempt_at) lastCheck = fmt.dateTime(u.last_attempt_at);

  return (
    <Page title={t('about.title')} description={t('about.description')}>
      <SectionCard
        title={t('about.versionTitle')}
        icon={InfoIcon}
        actions={
          allowed &&
          u && (
            <Button
              type="button"
              variant="outline"
              onClick={onCheck}
              disabled={check.isPending || !u.check_enabled}
            >
              <RefreshCwIcon
                aria-hidden="true"
                className={check.isPending ? 'animate-spin' : undefined}
              />
              {check.isPending ? t('about.checking') : t('about.checkNow')}
            </Button>
          )
        }
      >
        <dl className="grid gap-3 text-sm" data-testid="about-details">
          <Row label={t('about.current')}>{current}</Row>
          <Row label={t('about.edition')}>{edition}</Row>
          {u && (
            <>
              <Row label={t('about.check')}>
                {u.check_enabled ? t('about.checkOn') : t('about.checkOff')}
                {!u.check_enabled && (
                  <span className="block text-xs font-normal text-muted-foreground">
                    {u.how_to_update === 'desktop'
                      ? t('about.checkOffDesktop')
                      : t('about.checkOffServer')}
                  </span>
                )}
              </Row>
              {u.check_enabled && <Row label={t('about.lastCheck')}>{lastCheck}</Row>}
              {u.check_enabled && (
                <Row label={t('about.latest')}>
                  {u.latest ?? '—'}
                  {u.latest && u.published_at && (
                    <span className="block text-xs font-normal text-muted-foreground">
                      {t('about.published', { date: fmt.date(u.published_at) })}
                    </span>
                  )}
                </Row>
              )}
            </>
          )}
        </dl>
        {u && (
          <p className="mt-4 text-sm text-muted-foreground" role="status" aria-live="polite">
            {u.check_enabled && u.status === 'disabled' && t('status.devBuild')}
            {u.status === 'unavailable' && t('status.unavailable')}
            {u.status === 'ok' && u.latest && !u.update_available && t('status.upToDate')}
          </p>
        )}
        {!allowed && me.data && (
          <Alert className="mt-4">
            <AlertDescription>{t('about.ownersOnly')}</AlertDescription>
          </Alert>
        )}
      </SectionCard>

      {u?.update_available && u.latest && (
        <SectionCard
          title={t('banner.title', { latest: u.latest })}
          description={t('about.backupFirst')}
          icon={SparklesIcon}
        >
          <div className="space-y-3">
            <UpdateActions u={u} />
            {u.how_to_update !== 'docker' && inDesktopWindow() && (
              <p className="text-sm text-muted-foreground">{t('about.opensInBrowser')}</p>
            )}
          </div>
        </SectionCard>
      )}

      {allowed && u && (
        <div className="flex flex-wrap gap-2">
          {u.how_to_update !== 'docker' && (
            <Button asChild variant="link" className="px-0">
              <a href={DOWNLOADS_PAGE} target="_blank" rel="noreferrer">
                <ExternalLinkIcon aria-hidden="true" />
                {t('about.allVersions')}
              </a>
            </Button>
          )}
          {u.how_to_update !== 'desktop' && (
            <Button asChild variant="link" className="px-0">
              <a href={upgradeDocsUrl(i18n.language)} target="_blank" rel="noreferrer">
                <ExternalLinkIcon aria-hidden="true" />
                {t('banner.howTo')}
              </a>
            </Button>
          )}
        </div>
      )}
    </Page>
  );
}
