import type { Theme } from '@arenile/shared/theme';
import type { PreviewThemeMessage } from './protocol';

function isThemeMessage(data: unknown): data is PreviewThemeMessage {
  if (typeof data !== 'object' || data === null) return false;
  const d = data as { type?: unknown; theme?: unknown };
  return (
    d.type === 'arenile:preview-theme' &&
    typeof d.theme === 'object' &&
    d.theme !== null &&
    !Array.isArray(d.theme)
  );
}

/**
 * Theme preview channel of the admin panel (`?preview=1`). Accepts theme
 * messages only from the same origin AND from the parent window: a third
 * party host can never restyle the page. The theme still goes through
 * `resolveTheme` (hex colours and known fonts only). Announces readiness so
 * the panel sends the current draft. Returns the unsubscribe function.
 */
export function listenPreviewTheme(win: Window, onTheme: (theme: Theme) => void): () => void {
  if (win.parent === win) return () => {};
  const origin = win.location.origin;
  const onMessage = (event: MessageEvent) => {
    if (event.origin !== origin || event.source !== win.parent) return;
    if (!isThemeMessage(event.data)) return;
    onTheme(event.data.theme as Theme);
  };
  win.addEventListener('message', onMessage);
  win.parent.postMessage({ type: 'arenile:preview-ready' }, origin);
  return () => win.removeEventListener('message', onMessage);
}
