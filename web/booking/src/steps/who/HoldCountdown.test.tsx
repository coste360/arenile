import { act, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { testI18n } from '../../test-utils';
import { announcementBucket } from './countdown';
import { HoldCountdown } from './HoldCountdown';

describe('HoldCountdown', () => {
  let now = 0;
  beforeEach(() => {
    vi.useFakeTimers();
    now = Date.parse('2099-07-01T10:00:00Z');
  });
  afterEach(() => vi.useRealTimers());

  function renderCountdown(expiresAt: string, onExpire = vi.fn()) {
    render(
      <I18nextProvider i18n={testI18n()}>
        <HoldCountdown expiresAt={expiresAt} onExpire={onExpire} now={() => now} />
      </I18nextProvider>,
    );
    return onExpire;
  }

  const tick = (ms: number) =>
    act(() => {
      now += ms;
      vi.advanceTimersByTime(ms);
    });

  it('shows the remaining time and announces it at coarse steps', () => {
    renderCountdown('2099-07-01T10:02:05Z');
    expect(screen.getByTestId('hold-countdown')).toHaveTextContent('Posto riservato per 2:05');
    expect(screen.getByRole('status')).toHaveTextContent('Posto riservato ancora per 3 minuti');
    tick(70_000);
    expect(screen.getByTestId('hold-countdown')).toHaveTextContent('0:55');
    expect(screen.getByRole('status')).toHaveTextContent('ancora per 60 secondi');
  });

  it('calls onExpire once and says the time is up', () => {
    const onExpire = renderCountdown('2099-07-01T10:00:03Z');
    tick(2000);
    expect(onExpire).not.toHaveBeenCalled();
    tick(2000);
    tick(5000);
    expect(onExpire).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('hold-countdown')).toHaveTextContent('Tempo scaduto');
  });

  it('expires immediately for a hold already in the past', () => {
    const onExpire = renderCountdown('2099-07-01T09:59:00Z');
    expect(onExpire).toHaveBeenCalledTimes(1);
  });
});

describe('announcementBucket', () => {
  it.each([
    [600_000, { unit: 'minutes', value: 10 }],
    [61_000, { unit: 'minutes', value: 2 }],
    [60_000, { unit: 'seconds', value: 60 }],
    [29_000, { unit: 'seconds', value: 30 }],
    [5_000, { unit: 'seconds', value: 10 }],
    [0, null],
  ])('%i ms', (ms, expected) => {
    expect(announcementBucket(ms)).toEqual(expected);
  });
});
