import { useEffect, useState } from 'react';

/**
 * `value` after it stopped changing for `delayMs` (the quote of the price
 * summary, spec 12.3). Values are compared by their JSON form, so an equal
 * selection rebuilt on every render does not restart the timer.
 */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const key = JSON.stringify(value);
  const [state, setState] = useState<{ key: string; value: T }>({ key, value });
  useEffect(() => {
    if (key === state.key) return;
    const id = setTimeout(() => setState({ key, value }), delayMs);
    return () => clearTimeout(id);
    // `value` is represented by `key`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, delayMs, state.key]);
  return state.value;
}
