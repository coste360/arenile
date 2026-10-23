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
 * - framed: top window navigation (allowed after the user's click), else the
 *   verified host page through `arenile:navigate` (embed.js navigates and
 *   lets the host intercept it), else "manual": the caller shows a link with
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
  try {
    if (win.top) {
      win.top.location.href = href;
      return 'top';
    }
  } catch {
    // Cross-origin top navigation blocked (sandbox, no user activation).
  }
  if (channel) {
    channel.post({ type: 'arenile:navigate', url: href });
    return 'parent';
  }
  return 'manual';
}
