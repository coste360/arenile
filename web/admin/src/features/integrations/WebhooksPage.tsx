import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Webhook" (Configurazione, owner only; spec 18.2). DataTable of the
 * endpoints (URL, description, events, status active/paused/disabled with
 * reason, failures, last delivery); dialog to create/edit (https URL,
 * description, events picker grouped by prefix with "tutti" and
 * "<gruppo>.*", active switch); secret shown ONCE after creation and after
 * "Ruota segreto" (grace hours, default 24); "Invia evento di prova" with
 * the result inline; detail view with the delivery log (DataTable: event
 * type, status, attempts, HTTP code, next attempt / delivered / failed,
 * cursor pagination, status filter), delivery detail sheet (envelope JSON,
 * response excerpt) and "Reinvia". Signature help: header
 * `Arenile-Signature: t=…,v1=…` (HMAC-SHA256 of "<t>.<body>"). API:
 * webhook-endpoints/*. Owner: integrations frontend builder.
 */
export function WebhooksPage() {
  return (
    <PlaceholderPage
      titleKey="nav.config.webhooks"
      descriptionKey="integrations:webhooks.description"
      milestone="M5"
    />
  );
}
