import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { backoffDelay, useEstablishmentEvents, type EventSourceLike } from './events';
import { establishmentKeys } from './keys';

class FakeEventSource implements EventSourceLike {
  readyState = 0;
  onopen: ((ev: Event) => unknown) | null = null;
  onerror: ((ev: Event) => unknown) | null = null;
  closed = false;
  listeners = new Map<string, ((ev: MessageEvent) => void)[]>();
  constructor(readonly url: string) {}
  addEventListener(type: string, listener: (ev: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  open() {
    this.readyState = 1;
    this.onopen?.(new Event('open'));
  }
  emit(type: string, data: unknown) {
    for (const l of this.listeners.get(type) ?? []) {
      l(new MessageEvent(type, { data: JSON.stringify(data) }));
    }
  }
  fail() {
    this.readyState = 2;
    this.onerror?.(new Event('error'));
  }
}

function setup(establishmentId = 'e1') {
  const qc = new QueryClient();
  const spy = vi.spyOn(qc, 'invalidateQueries');
  const sources: FakeEventSource[] = [];
  const createEventSource = (url: string) => {
    const s = new FakeEventSource(url);
    sources.push(s);
    return s;
  };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  const hook = renderHook(
    () => useEstablishmentEvents(establishmentId, { createEventSource, debounceMs: 10 }),
    { wrapper },
  );
  return { hook, spy, sources };
}

function at<T>(items: T[], i: number): T {
  const item = items[i];
  if (item === undefined) throw new Error(`no item ${i}`);
  return item;
}

const keysOf = (spy: { mock: { calls: unknown[][] } }): string[] =>
  spy.mock.calls.map((c) => JSON.stringify((c[0] as { queryKey: unknown }).queryKey));

describe('useEstablishmentEvents', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('subscribes to the establishment stream and invalidates the areas of a topic', () => {
    const { hook, spy, sources } = setup();
    expect(at(sources, 0).url).toBe('/api/admin/v1/stream?establishment=e1');
    act(() => at(sources, 0).open());
    expect(hook.result.current).toBe('open');
    act(() => {
      at(sources, 0).emit('hold.changed', {
        id: '1',
        topic: 'hold.changed',
        type: 'hold.created',
        establishment_id: 'e1',
        occurred_at: '',
      });
      at(sources, 0).emit('hold.changed', {
        id: '2',
        topic: 'hold.changed',
        type: 'hold.released',
        establishment_id: 'e1',
        occurred_at: '',
      });
      vi.advanceTimersByTime(20);
    });
    const keys = keysOf(spy);
    expect(keys).toContain(JSON.stringify(establishmentKeys.area('e1', 'operations')));
    expect(keys).toContain(JSON.stringify(establishmentKeys.area('e1', 'holds')));
    // Bursts are coalesced: one invalidation per area.
    expect(
      keys.filter((k) => k === JSON.stringify(establishmentKeys.area('e1', 'holds'))),
    ).toHaveLength(1);
  });

  it('ignores messages of another establishment', () => {
    const { spy, sources } = setup();
    act(() => {
      at(sources, 0).open();
      at(sources, 0).emit('booking.changed', {
        id: '1',
        topic: 'booking.changed',
        type: 'booking.created',
        establishment_id: 'e2',
        occurred_at: '',
      });
      vi.advanceTimersByTime(20);
    });
    expect(spy).not.toHaveBeenCalled();
  });

  it('reconnects with backoff when the stream closes and refreshes everything once reopened', () => {
    const { hook, spy, sources } = setup();
    act(() => at(sources, 0).open());
    act(() => at(sources, 0).fail());
    expect(hook.result.current).toBe('reconnecting');
    expect(at(sources, 0).closed).toBe(true);
    act(() => vi.advanceTimersByTime(1300));
    expect(sources).toHaveLength(2);
    act(() => at(sources, 1).open());
    expect(keysOf(spy)).toContain(JSON.stringify(establishmentKeys.all('e1')));
  });

  it('closes the stream on unmount', () => {
    const { hook, sources } = setup();
    hook.unmount();
    expect(at(sources, 0).closed).toBe(true);
  });

  it('computes an exponential, capped backoff', () => {
    expect(backoffDelay(0, () => 0.5)).toBe(1000);
    expect(backoffDelay(3, () => 0.5)).toBe(8000);
    expect(backoffDelay(20, () => 0.5)).toBe(30000);
  });
});
