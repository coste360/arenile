import { Button } from '@arenile/shared/ui/button';
import { DownloadIcon, ExternalLinkIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { SystemUpdate } from './api';
import { isPackageUrl, platformLabel, upgradeDocsUrl } from './lib';

/**
 * Actions for a new version, shared by the banner and "Informazioni su
 * Arenile" so both say the same thing: the package for this system (desktop
 * app, server binary) or the upgrade guide (Docker), and the release notes.
 * Every link opens outside the panel (new tab; the default browser from
 * the desktop app window).
 */
export function UpdateActions({
  u,
  size = 'default',
}: {
  u: SystemUpdate;
  size?: 'sm' | 'default';
}) {
  const { t, i18n } = useTranslation('updates');
  if (!u.update_available || !u.latest) return null;
  const platform = isPackageUrl(u.download_url) ? platformLabel(u.os, u.arch) : undefined;
  return (
    <div className="flex flex-wrap items-center gap-2">
      {u.how_to_update === 'docker' ? (
        <Button asChild size={size}>
          <a href={upgradeDocsUrl(i18n.language)} target="_blank" rel="noreferrer">
            <ExternalLinkIcon aria-hidden="true" />
            {t('banner.howTo')}
          </a>
        </Button>
      ) : (
        u.download_url && (
          <Button asChild size={size}>
            <a href={u.download_url} target="_blank" rel="noreferrer">
              <DownloadIcon aria-hidden="true" />
              {platform
                ? t('actions.downloadFor', { latest: u.latest, platform })
                : t('actions.download', { latest: u.latest })}
            </a>
          </Button>
        )
      )}
      {u.release_notes_url && (
        <Button asChild size={size} variant="outline">
          <a href={u.release_notes_url} target="_blank" rel="noreferrer">
            {t('banner.notes')}
          </a>
        </Button>
      )}
    </div>
  );
}
