import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * First start of the desktop edition (spec 29.1, outside the authenticated
 * shell): reads the one-time setup token from `location.hash` (#token=...),
 * removes it from the URL, asks name, email, password and organization
 * (react-hook-form + zod), calls POST /api/desktop/v1/first-run and then
 * continues to the guided setup (/e/$establishmentId/setup). Errors:
 * setup_token_invalid (reopen from the tray menu), setup_completed (go to
 * login). Owner: desktop runtime builder.
 */
export function FirstRunPage() {
  return (
    <PlaceholderPage
      titleKey="desktop:firstRun.title"
      descriptionKey="desktop:firstRun.description"
      milestone="M6"
    />
  );
}
