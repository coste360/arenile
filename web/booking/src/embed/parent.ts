import type { ArenileMessage } from './protocol';

function httpOrigin(value: string | null | undefined): string | null {
  if (!value) return null;
  try {
    const url = new URL(value);
    if (url.protocol !== 'https:' && url.protocol !== 'http:') return null;
    return url.origin;
  } catch {
    return null;
  }
}

export interface ParentOriginInputs {
  /** True when the page runs inside a frame. */
  framed: boolean;
  /** `?origin=` declared by embed.js; untrusted until confirmed. */
  declared: string | null;
  /** `location.ancestorOrigins` when the browser exposes it (Chromium, WebKit). */
  ancestorOrigins?: ArrayLike<string> | null;
  /** `document.referrer` of the frame (the host page). */
  referrer?: string | null;
}

/**
 * Determines the host page origin we may post messages to (spec 12.2).
 * The browser-provided ancestor origin is authoritative; otherwise the
 * referrer is used. A declared origin is accepted only if it matches what
 * the browser reports; nothing is trusted when it cannot be confirmed.
 * Which hosts may frame the page at all is enforced by the server through
 * `Content-Security-Policy: frame-ancestors`.
 */
export function resolveParentOrigin(inputs: ParentOriginInputs): string | null {
  if (!inputs.framed) return null;
  const declared = httpOrigin(inputs.declared);
  const ancestor = inputs.ancestorOrigins?.length ? httpOrigin(inputs.ancestorOrigins[0]) : null;
  const observed = ancestor ?? httpOrigin(inputs.referrer);
  if (!observed) return null;
  if (inputs.declared && declared !== observed) return null;
  return observed;
}

export interface ParentChannel {
  readonly origin: string;
  post(message: ArenileMessage): void;
}

/** Creates a channel to the verified parent, or null when not embedded/verified. */
export function connectParent(win: Window, declared: string | null): ParentChannel | null {
  const framed = win.parent !== win;
  const origin = resolveParentOrigin({
    framed,
    declared,
    ancestorOrigins: win.location.ancestorOrigins ?? null,
    referrer: win.document.referrer,
  });
  if (!origin) return null;
  return {
    origin,
    post(message) {
      win.parent.postMessage(message, origin);
    },
  };
}

/** Reports the document height to the parent whenever it changes. */
export function observeHeight(win: Window, channel: ParentChannel): () => void {
  let last = -1;
  const send = () => {
    const height = Math.ceil(win.document.documentElement.scrollHeight);
    if (height !== last) {
      last = height;
      channel.post({ type: 'arenile:resize', height });
    }
  };
  send();
  if (typeof ResizeObserver === 'undefined') {
    win.addEventListener('resize', send);
    return () => win.removeEventListener('resize', send);
  }
  const ro = new ResizeObserver(send);
  ro.observe(win.document.body);
  return () => ro.disconnect();
}
