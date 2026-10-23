import { describe, expect, it } from 'vitest';
import type { ApiKey, WebhookEndpoint } from '../api';
import { canRedeliver } from './deliveries';
import { EVENT_GROUPS, filtersMatch, nextEventSelection } from './events';
import {
  apiKeyDefaults,
  apiKeySchema,
  endOfDayInstant,
  isAllowedWebhookUrl,
  redactWebhookUrl,
  toApiKeyCreate,
  toApiKeyUpdate,
  toWebhookUpdate,
  webhookDefaults,
  webhookSchema,
} from './forms';

describe('event filters', () => {
  it('lists the groups in spec order', () => {
    expect(EVENT_GROUPS).toEqual([
      'booking',
      'allocation',
      'season',
      'waitlist',
      'credit',
      'payment',
      'customer',
      'cash_session',
      'config',
    ]);
  });
  it.each([
    [[], ['payment.failed', 'booking.created'], ['booking.created', 'payment.failed']],
    [['booking.created', 'payment.failed'], ['booking.created', 'payment.failed', '*'], ['*']],
    [['*'], ['*', 'booking.created'], ['booking.created']],
    [
      ['booking.created', 'booking.held', 'payment.failed'],
      ['booking.created', 'booking.held', 'payment.failed', 'booking.*'],
      ['booking.*', 'payment.failed'],
    ],
    [['booking.*'], ['booking.*', 'booking.held'], ['booking.held']],
    [['booking.*', 'payment.*'], ['payment.*'], ['payment.*']],
  ])('%j -> %j gives %j', (prev, next, out) => {
    expect(nextEventSelection(prev, next)).toEqual(out);
  });
  it.each([
    [['*'], 'payment.failed', true],
    [['payment.*'], 'payment.failed', true],
    [['payment.*'], 'booking.created', false],
    [['booking.created'], 'booking.created', true],
    [['booking.created'], 'booking.cancelled', false],
  ])('%j matches %s: %s', (filters, event, match) => {
    expect(filtersMatch(filters, event)).toBe(match);
  });
});

const key: ApiKey = {
  id: 'k1',
  name: 'Gestionale',
  prefix: 'ar_live_abcd2345',
  establishment_id: 'e1',
  scopes: ['reports.read', 'bookings.read'],
  status: 'active',
  created_at: '2026-01-01T00:00:00Z',
  expires_at: endOfDayInstant('2026-12-31'),
};

describe('API key form', () => {
  const schema = apiKeySchema('2026-07-15');
  it('requires a name and a scope, and a future expiry', () => {
    const r = schema.safeParse({ name: ' ', scopes: [], expires_on: '2026-07-14' });
    expect(r.success).toBe(false);
    expect(r.error?.issues.map((i) => i.message).sort()).toEqual([
      'integrations:validation.expiryPast',
      'integrations:validation.required',
      'integrations:validation.scopes',
    ]);
    expect(schema.safeParse({ name: 'X', scopes: ['bookings.read'], expires_on: '' }).success).toBe(
      true,
    );
  });
  it('builds the create body with scopes in canonical order', () => {
    expect(
      toApiKeyCreate({
        name: ' Sito ',
        scopes: ['reports.read', 'bookings.write'],
        expires_on: '',
      }),
    ).toEqual({ name: 'Sito', scopes: ['bookings.write', 'reports.read'] });
    expect(
      toApiKeyCreate({ name: 'Sito', scopes: ['bookings.read'], expires_on: '2026-08-01' })
        .expires_at,
    ).toBe(endOfDayInstant('2026-08-01'));
  });
  it('patches only what changed and clears a removed expiry', () => {
    const defaults = apiKeyDefaults(key);
    expect(defaults.expires_on).toBe('2026-12-31');
    expect(toApiKeyUpdate(key, defaults)).toEqual({});
    expect(toApiKeyUpdate(key, { ...defaults, expires_on: '' })).toEqual({ clear_expiry: true });
    expect(toApiKeyUpdate(key, { ...defaults, name: 'Nuovo', scopes: ['bookings.read'] })).toEqual({
      name: 'Nuovo',
      scopes: ['bookings.read'],
    });
  });
});

describe('webhook form', () => {
  it.each([
    ['https://hooks.example.it/arenile', true],
    ['http://localhost:9000/hook', true],
    ['http://127.0.0.1/hook', true],
    ['http://hooks.example.it/arenile', false],
    ['ftp://example.it', false],
    ['non un url', false],
  ])('%s allowed: %s', (url, ok) => {
    expect(isAllowedWebhookUrl(url)).toBe(ok);
  });
  it('requires at least one event', () => {
    const r = webhookSchema.safeParse({ ...webhookDefaults(), url: 'https://x.it/h', events: [] });
    expect(r.error?.issues[0]?.message).toBe('integrations:validation.events');
  });
  it('patches only what changed', () => {
    const ep: WebhookEndpoint = {
      id: 'w1',
      url: 'https://x.it/h',
      description: '',
      events: ['booking.*'],
      active: true,
      status: 'active',
      consecutive_failures: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    };
    expect(toWebhookUpdate(ep, webhookDefaults(ep))).toEqual({});
    expect(toWebhookUpdate(ep, { ...webhookDefaults(ep), active: false, events: ['*'] })).toEqual({
      active: false,
      events: ['*'],
    });
  });
});

describe('canRedeliver', () => {
  it.each([
    ['active', 'failed', true],
    ['paused', 'delivered', true],
    ['active', 'pending', false],
    ['disabled', 'failed', false],
  ] as const)('endpoint %s, delivery %s: %s', (ep, d, ok) => {
    expect(canRedeliver({ status: ep }, { status: d })).toBe(ok);
  });
});

describe('redactWebhookUrl', () => {
  it('hides the query string where a receiver token may live', () => {
    expect(redactWebhookUrl('https://hooks.example.it/arenile')).toBe(
      'https://hooks.example.it/arenile',
    );
    expect(redactWebhookUrl('https://hooks.example.it/arenile?token=s3cr3t&x=1')).toBe(
      'https://hooks.example.it/arenile?…',
    );
    expect(redactWebhookUrl('https://hooks.example.it/arenile?')).toBe(
      'https://hooks.example.it/arenile',
    );
  });
});
