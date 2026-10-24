import { useTranslation } from 'react-i18next';
import { useSystemUpdate } from './api';

/**
 * Small, muted status of the update check (ADR 0025): running version, the
 * switch (ARENILE_UPDATE_CHECK on server installs, the desktop setting on
 * the desktop app) and, when the last check failed, a neutral line, never
 * an error. Renders nothing for roles that cannot read it.
 */
export function UpdateCheckStatus({ showSwitchSource = true }: { showSwitchSource?: boolean }) {
  const { t } = useTranslation('updates');
  const { data: u } = useSystemUpdate();
  if (!u) return null;
  const server = u.how_to_update !== 'desktop';
  return (
    <div className="grid gap-0.5 text-xs text-muted-foreground" data-testid="update-check-status">
      <span>{t('status.version', { current: u.current })}</span>
      {showSwitchSource && (
        <span>
          {u.check_enabled ? t('status.enabled') : t('status.disabled')}
          {server && ' (ARENILE_UPDATE_CHECK)'}
        </span>
      )}
      {u.status === 'unavailable' && <span>{t('status.unavailable')}</span>}
      {u.status === 'ok' && u.latest && !u.update_available && <span>{t('status.upToDate')}</span>}
    </div>
  );
}
