import { ApiProblem } from '@arenile/shared/api';
import { describe, expect, it, vi } from 'vitest';
import { todayIn } from './context';
import { formatCountdown, formatMoney, localize } from './format';
import { redirectToPayment } from './navigate';
import { isHoldExpired, isResourceProblem, problemMessageKey } from './problems';

const problem = (type: string, status = 409) =>
  new ApiProblem({ type: type as never, status, title: type });

describe('problemMessageKey', () => {
  it.each([
    [problem('spot_unavailable'), 'public:errors.spot_unavailable'],
    [problem('quota_exceeded'), 'public:errors.quota_exceeded'],
    [problem('hold_expired', 410), 'public:errors.hold_expired'],
    [problem('unknown_error', 410), 'public:errors.hold_expired'],
    [problem('payment_provider_unavailable', 503), 'public:errors.payment_provider_unavailable'],
    [new ApiProblem({ type: 'network_error', status: 0, title: 'x' }), 'common.errors.network'],
    [problem('internal_error', 500), 'common.errors.generic'],
    [new Error('boom'), 'common.errors.generic'],
  ])('%s → %s', (error, key) => {
    expect(problemMessageKey(error)).toBe(key);
  });

  it('classifies resource and expiry problems', () => {
    expect(isResourceProblem(problem('pool_exhausted'))).toBe(true);
    expect(isResourceProblem(problem('policy_violation', 422))).toBe(false);
    expect(isHoldExpired(problem('hold_expired', 410))).toBe(true);
  });
});

describe('format', () => {
  it('formats cents in the page language', () => {
    expect(formatMoney(123450, 'EUR', 'it')).toBe('1.234,50 €');
    expect(formatMoney(2500, 'EUR', 'en')).toBe('€25.00');
  });

  it('localizes manager texts with fallback', () => {
    expect(localize({ it: 'Pineta', en: 'Pine wood' }, 'en')).toBe('Pine wood');
    expect(localize({ it: 'Pineta' }, 'de')).toBe('Pineta');
    expect(localize({ fr: 'Pinède' }, 'de', 'fr')).toBe('Pinède');
    expect(localize(undefined, 'it')).toBe('');
  });

  it('formats the countdown', () => {
    expect(formatCountdown(600_000)).toBe('10:00');
    expect(formatCountdown(61_001)).toBe('1:02');
    expect(formatCountdown(-5)).toBe('0:00');
  });

  it('computes today in the establishment time zone', () => {
    // 23:30 UTC on 30 June is already 1 July in Rome.
    expect(todayIn('Europe/Rome', new Date('2099-06-30T23:30:00Z'))).toBe('2099-07-01');
  });
});

describe('redirectToPayment', () => {
  const url = 'https://checkout.stripe.com/c/pay/cs_test_1';

  it('navigates the same window when not framed', () => {
    const assign = vi.fn();
    const win = { location: { assign } } as unknown as Window;
    (win as { top: Window }).top = win;
    (win as { parent: Window }).parent = win;
    expect(redirectToPayment(url, win, null)).toBe('self');
    expect(assign).toHaveBeenCalledWith(url);
  });

  it('navigates the top window from a frame', () => {
    const top = { location: { href: '' } };
    const win = { top, parent: top, location: { assign: vi.fn() } } as unknown as Window;
    expect(redirectToPayment(url, win, null)).toBe('top');
    expect(top.location.href).toBe(url);
  });

  it('asks the verified host page when top navigation is blocked, else falls back to a link', () => {
    const top = {
      get location(): Location {
        throw new DOMException('blocked', 'SecurityError');
      },
    };
    const win = { top, parent: top } as unknown as Window;
    const post = vi.fn();
    expect(redirectToPayment(url, win, { origin: 'https://lido.it', post })).toBe('parent');
    expect(post).toHaveBeenCalledWith({ type: 'arenile:navigate', url });
    expect(redirectToPayment(url, win, null)).toBe('manual');
  });

  it('refuses non-http URLs', () => {
    const win = {} as Window;
    expect(redirectToPayment('javascript:alert(1)', win, null)).toBe('manual');
  });
});
