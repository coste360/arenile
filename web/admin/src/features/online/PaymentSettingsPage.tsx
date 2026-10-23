import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Pagamenti online" (spec 13): provider (manual / stripe), test or live
 * mode, Stripe publishable key, secret key and webhook secret (write-only
 * inputs: show only "configurata ••••4242"), embedded mode toggle, webhook
 * URL to copy into the Stripe dashboard, "verifica credenziali" action,
 * link to the reconciliation report. API: GET/PUT payment-settings
 * (If-Match), POST payment-settings/test, GET/POST payments/reconciliation.
 * Owner: admin online builder.
 */
export function PaymentSettingsPage() {
  return (
    <PlaceholderPage
      titleKey="online:payments.title"
      descriptionKey="online:payments.description"
      milestone="M4"
    />
  );
}
