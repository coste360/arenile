import { act } from '@testing-library/react';

/** Test helper: sets navigator.onLine and fires the window event (tests only). */
export function setOnline(online: boolean): void {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, get: () => online });
  act(() => {
    window.dispatchEvent(new Event(online ? 'online' : 'offline'));
  });
}
