/**
 * Form schemas and mappers of the integrations pages. Validation messages
 * are i18n keys (integrations namespace), rendered by <FieldError>. The
 * server validates again; these rules only give early feedback.
 */
import { z } from 'zod';
import type {
  ApiKey,
  ApiKeyCreate,
  ApiKeyUpdate,
  Permission,
  WebhookEndpoint,
  WebhookEndpointCreate,
  WebhookEndpointUpdate,
} from '../api';
import { API_KEY_SCOPES } from '../api';

export const M = {
  required: 'integrations:validation.required',
  scopes: 'integrations:validation.scopes',
  expiryPast: 'integrations:validation.expiryPast',
  url: 'integrations:validation.url',
  events: 'integrations:validation.events',
} as const;

// ---- API keys ----------------------------------------------------------------

/** End of `date` (YYYY-MM-DD) in the browser zone, as an RFC 3339 instant. */
export function endOfDayInstant(date: string): string {
  return new Date(`${date}T23:59:59`).toISOString();
}

/** Calendar day (browser zone) of an instant, for the date picker. */
export function instantToDate(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function apiKeySchema(today: string) {
  return z.object({
    name: z.string().trim().min(1, M.required).max(100),
    scopes: z.array(z.string()).min(1, M.scopes),
    expires_on: z.string().refine((v) => v === '' || v >= today, M.expiryPast),
  });
}

export type ApiKeyValues = z.infer<ReturnType<typeof apiKeySchema>>;

export function apiKeyDefaults(key?: ApiKey): ApiKeyValues {
  return {
    name: key?.name ?? '',
    scopes: key ? [...key.scopes] : ['bookings.read'],
    expires_on: instantToDate(key?.expires_at),
  };
}

const scopesOf = (values: ApiKeyValues): Permission[] =>
  API_KEY_SCOPES.filter((s) => values.scopes.includes(s));

export function toApiKeyCreate(values: ApiKeyValues): ApiKeyCreate {
  return {
    name: values.name.trim(),
    scopes: scopesOf(values),
    ...(values.expires_on ? { expires_at: endOfDayInstant(values.expires_on) } : {}),
  };
}

/** PATCH body: only what changed; a removed expiry becomes clear_expiry. */
export function toApiKeyUpdate(key: ApiKey, values: ApiKeyValues): ApiKeyUpdate {
  const patch: ApiKeyUpdate = {};
  const name = values.name.trim();
  if (name !== key.name) patch.name = name;
  const scopes = scopesOf(values);
  if (scopes.join() !== API_KEY_SCOPES.filter((s) => key.scopes.includes(s)).join()) {
    patch.scopes = scopes;
  }
  const before = instantToDate(key.expires_at);
  if (values.expires_on !== before) {
    if (values.expires_on) patch.expires_at = endOfDayInstant(values.expires_on);
    else patch.clear_expiry = true;
  }
  return patch;
}

// ---- Webhook endpoints ----------------------------------------------------------

/** https, or http only towards this machine (local development), as the server requires. */
export function isAllowedWebhookUrl(value: string): boolean {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    return false;
  }
  if (url.protocol === 'https:') return url.hostname !== '';
  return url.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname);
}

export const webhookSchema = z.object({
  url: z.string().trim().min(8, M.url).max(2048).refine(isAllowedWebhookUrl, M.url),
  description: z.string().trim().max(200),
  events: z.array(z.string()).min(1, M.events).max(50),
  active: z.boolean(),
});

export type WebhookValues = z.infer<typeof webhookSchema>;

export function webhookDefaults(endpoint?: WebhookEndpoint): WebhookValues {
  return {
    url: endpoint?.url ?? 'https://',
    description: endpoint?.description ?? '',
    events: endpoint ? [...endpoint.events] : ['booking.*'],
    active: endpoint?.active ?? true,
  };
}

export function toWebhookCreate(values: WebhookValues): WebhookEndpointCreate {
  return {
    url: values.url.trim(),
    description: values.description.trim(),
    events: values.events,
    active: values.active,
  };
}

export function toWebhookUpdate(
  endpoint: WebhookEndpoint,
  values: WebhookValues,
): WebhookEndpointUpdate {
  const patch: WebhookEndpointUpdate = {};
  const v = toWebhookCreate(values);
  if (v.url !== endpoint.url) patch.url = v.url;
  if (v.description !== endpoint.description) patch.description = v.description;
  if (v.events.join() !== endpoint.events.join()) patch.events = v.events;
  if (v.active !== endpoint.active) patch.active = v.active;
  return patch;
}
