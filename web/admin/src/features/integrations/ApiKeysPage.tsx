import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Chiavi API" (Configurazione, owner only; spec 16, 17). DataTable of the
 * keys (name, prefix `ar_live_xxxx`, scopes as badges, status, created by,
 * last used, expiry) with "Mostra revocate" toggle; dialog "Nuova chiave"
 * (name, scopes checklist from Schemas['Permission'] minus users.manage,
 * optional expiry); after creation a dialog shows the full key ONCE with a
 * copy button and the warning that it will not be shown again; edit dialog
 * (name, scopes, expiry / remove expiry); revoke with confirmation. Usage
 * hint: `Authorization: Bearer <chiave>`. API: listAPIKeys, createAPIKey,
 * updateAPIKey, revokeAPIKey. Owner: integrations frontend builder.
 */
export function ApiKeysPage() {
  return (
    <PlaceholderPage
      titleKey="nav.config.apiKeys"
      descriptionKey="integrations:apiKeys.description"
      milestone="M5"
    />
  );
}
