import type { ParentChannel } from '../embed/parent';

export type RedirectOutcome = 'self' | 'top' | 'parent' | 'manual';

function httpUrl(url: string): string | null {
  try {
    const u = new URL(url);
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : null;
  } catch {
    return null;
  }
}

/**
 * Opens the payment page at top level (spec 13.2, 12.3 passo 4): payment
 * providers refuse to run inside third-party frames.
 * - not framed: same window;
 * - framed with a verified host page (embed.js): `arenile:navigate`, so the
 *   host receives the cancelable event documented in spec 12.1 and embed.js
 *   navigates unless the site handled it (`preventDefault()`);
 * - framed without a channel (plain iframe): top window navigation (allowed
 *   after the user's click), else "manual": the caller shows a link with
 *   `target=_top` / a new tab.
 */
export function redirectToPayment(
  url: string,
  win: Window,
  channel: ParentChannel | null,
): RedirectOutcome {
  const href = httpUrl(url);
  if (!href) return 'manual';
  if (win.top === win || win.parent === win) {
    win.location.assign(href);
    return 'self';
  }
  if (channel) {
    channel.post({ type: 'arenile:navigate', url: href });
    return 'parent';
  }
  try {
    if (win.top) {
      win.top.location.href = href;
      return 'top';
    }
  } catch {
    // Cross-origin top navigation blocked (sandbox, no user activation).
  }
  return 'manual';
}
