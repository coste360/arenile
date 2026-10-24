import { describe, expect, it } from 'vitest';
import { isLocalUrl } from './localUrl';

describe('isLocalUrl', () => {
  it.each([
    ['http://127.0.0.1:8371/webhooks/payments/stripe/x', true],
    ['http://localhost:8080/webhooks/payments/stripe/x', true],
    ['https://arenile.local/webhooks/payments/stripe/x', true],
    ['http://192.168.1.20/webhooks/payments/stripe/x', true],
    ['http://10.0.0.5/webhooks/payments/stripe/x', true],
    ['http://172.20.0.2/webhooks/payments/stripe/x', true],
    ['http://[::1]:8080/webhooks/payments/stripe/x', true],
    ['https://prenota.example/webhooks/payments/stripe/x', false],
    ['https://172.32.0.1/webhooks/payments/stripe/x', false],
    ['not a url', false],
  ])('%s → %s', (url, local) => {
    expect(isLocalUrl(url)).toBe(local);
  });
});
