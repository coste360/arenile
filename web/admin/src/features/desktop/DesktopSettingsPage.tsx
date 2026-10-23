import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "App desktop" (Configurazione, desktop edition only, owner): backup
 * folder and retention, daily and on-exit backups, "Backup ora", list of
 * backups with "Ripristina" (AlertDialog, then a reload notice), update
 * check opt-in, autostart, LAN mode with address and certificate
 * fingerprint. API: GET/PUT /api/desktop/v1/settings, GET/POST
 * /api/desktop/v1/backups, POST backups/{backupName}/restore. Owner:
 * desktop runtime builder.
 */
export function DesktopSettingsPage() {
  return (
    <PlaceholderPage
      titleKey="desktop:settings.title"
      descriptionKey="desktop:settings.description"
      milestone="M6"
    />
  );
}
