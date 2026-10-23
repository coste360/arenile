import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Email e notifiche" (spec 19): transport from the server (SMTP / file /
 * log, read-only with guidance on ARENILE_SMTP_URL), sender name, reply-to,
 * manager email, customer and manager notification toggles, test email,
 * message log with masked recipients and statuses. API: GET/PUT
 * email-settings (If-Match), POST email-settings/test, GET messages.
 * Owner: admin online builder.
 */
export function EmailSettingsPage() {
  return (
    <PlaceholderPage
      titleKey="online:email.title"
      descriptionKey="online:email.description"
      milestone="M4"
    />
  );
}
