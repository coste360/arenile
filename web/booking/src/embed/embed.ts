/**
 * Arenile embed loader, served at /embed.js (spec 12.1, 12.2). Kept tiny:
 * no dependencies, ES2017, < 3 KB gzip.
 *
 *   <div data-arenile="lido-x" data-lang="it"></div>
 *   <script src="https://{host}/embed.js" async></script>
 *
 * Optional attributes: data-lang, data-from, data-to, data-pack, data-title.
 * Messages from the iframe are accepted only from the script origin and from
 * the iframe's own window; they are re-dispatched on the container as
 * bubbling CustomEvents ("arenile:resize", "arenile:booked", "arenile:navigate").
 */

interface ArenileEmbedApi {
  init(root?: ParentNode): void;
  origin: string;
}

declare global {
  interface Window {
    Arenile?: ArenileEmbedApi;
  }
}

const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;
const PARAMS = ['lang', 'from', 'to', 'pack'] as const;

function scriptOrigin(): string {
  const current = document.currentScript as HTMLScriptElement | null;
  const src =
    current?.src ||
    Array.from(document.querySelectorAll<HTMLScriptElement>('script[src]'))
      .map((s) => s.src)
      .filter((s) => /\/embed\.js(\?|$)/.test(s))
      .pop() ||
    location.href;
  return new URL(src, location.href).origin;
}

function setup(origin: string): ArenileEmbedApi {
  const frames = new Map<Window, { iframe: HTMLIFrameElement; host: HTMLElement }>();

  window.addEventListener('message', (event: MessageEvent) => {
    if (event.origin !== origin) return;
    const entry = event.source ? frames.get(event.source as Window) : undefined;
    const data = event.data as { type?: unknown; height?: unknown; url?: unknown } | null;
    if (!entry || !data || typeof data.type !== 'string') return;
    const { iframe, host } = entry;
    if (data.type === 'arenile:resize') {
      const h = Number(data.height);
      if (!(h > 0 && h < 100000)) return;
      iframe.style.height = `${Math.ceil(h)}px`;
    } else if (data.type !== 'arenile:booked' && data.type !== 'arenile:navigate') {
      return;
    }
    const ev = new CustomEvent(data.type, { detail: data, bubbles: true, cancelable: true });
    const proceed = host.dispatchEvent(ev);
    if (data.type === 'arenile:navigate' && proceed && typeof data.url === 'string') {
      const url = new URL(data.url, origin);
      if (url.protocol === 'https:' || url.protocol === 'http:') location.assign(url.href);
    }
  });

  function init(root: ParentNode = document): void {
    root
      .querySelectorAll<HTMLElement>('[data-arenile]:not([data-arenile-ready])')
      .forEach((host) => {
        const slug = (host.getAttribute('data-arenile') || '').trim().toLowerCase();
        if (!SLUG_RE.test(slug)) return;
        const url = new URL(`/b/${slug}`, origin);
        url.searchParams.set('embed', '1');
        PARAMS.forEach((p) => {
          const v = host.getAttribute(`data-${p}`);
          if (v) url.searchParams.set(p, v);
        });
        url.searchParams.set('origin', location.origin);
        const iframe = document.createElement('iframe');
        iframe.src = url.href;
        iframe.title = host.getAttribute('data-title') || 'Prenota il tuo posto';
        iframe.setAttribute('loading', 'lazy');
        iframe.setAttribute('allow', 'payment');
        iframe.style.cssText = 'width:100%;border:0;min-height:480px;display:block';
        host.setAttribute('data-arenile-ready', '');
        host.appendChild(iframe);
        if (iframe.contentWindow) frames.set(iframe.contentWindow, { iframe, host });
      });
  }

  return { init, origin };
}

if (!window.Arenile) {
  window.Arenile = setup(scriptOrigin());
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => window.Arenile?.init());
  } else {
    window.Arenile.init();
  }
}

export {};
