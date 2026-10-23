import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useDebouncedValue } from './useDebouncedValue';

describe('useDebouncedValue', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('publishes the value only after it stopped changing', () => {
    const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v, 400), {
      initialProps: { v: { n: 1 } },
    });
    expect(result.current).toEqual({ n: 1 });
    rerender({ v: { n: 2 } });
    act(() => vi.advanceTimersByTime(300));
    rerender({ v: { n: 3 } });
    act(() => vi.advanceTimersByTime(300));
    expect(result.current).toEqual({ n: 1 });
    act(() => vi.advanceTimersByTime(100));
    expect(result.current).toEqual({ n: 3 });
  });

  it('does not restart for an equal value rebuilt on every render', () => {
    const { result, rerender } = renderHook(({ v }) => useDebouncedValue(v, 400), {
      initialProps: { v: { n: 1 } },
    });
    const first = result.current;
    rerender({ v: { n: 1 } });
    act(() => vi.advanceTimersByTime(500));
    expect(result.current).toBe(first);
  });
});
