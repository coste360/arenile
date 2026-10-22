import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const ORIGIN = 'https://prenota.example.it';

async function loadEmbed() {
  vi.resetModules();
  delete window.Arenile;
  const script = document.createElement('script');
  script.src = `${ORIGIN}/embed.js`;
  document.head.appendChild(script);
  await import('./embed');
  return script;
}

function post(source: Window | null, origin: string, data: unknown) {
  window.dispatchEvent(new MessageEvent('message', { data, origin, source }));
}

describe('embed.js', () => {
  beforeEach(() => {
    document.body.innerHTML =
      '<div id="w" data-arenile="lido-x" data-lang="en" data-from="2027-07-10" data-title="Book"></div><div data-arenile="Bad Slug!"></div>';
  });
  afterEach(() => {
    document.head.innerHTML = '';
    document.body.innerHTML = '';
  });

  it('creates one iframe per valid container pointing to the script origin', async () => {
    await loadEmbed();
    const frames = document.querySelectorAll('iframe');
    expect(frames).toHaveLength(1);
    const src = new URL(frames[0]!.src);
    expect(src.origin).toBe(ORIGIN);
    expect(src.pathname).toBe('/b/lido-x');
    expect(src.searchParams.get('embed')).toBe('1');
    expect(src.searchParams.get('lang')).toBe('en');
    expect(src.searchParams.get('from')).toBe('2027-07-10');
    expect(src.searchParams.get('origin')).toBe(window.location.origin);
    expect(frames[0]!.title).toBe('Book');
    expect(frames[0]!.getAttribute('allow')).toBe('payment');
    // idempotent re-init
    window.Arenile?.init();
    expect(document.querySelectorAll('iframe')).toHaveLength(1);
  });

  it('resizes and re-dispatches events only for messages from the script origin and its iframe', async () => {
    await loadEmbed();
    const iframe = document.querySelector('iframe')!;
    const host = document.getElementById('w')!;
    const resized = vi.fn();
    const booked = vi.fn();
    document.addEventListener('arenile:resize', resized);
    host.addEventListener('arenile:booked', booked);

    post(iframe.contentWindow, 'https://evil.example', { type: 'arenile:resize', height: 999 });
    post(window, ORIGIN, { type: 'arenile:resize', height: 999 });
    expect(iframe.style.height).toBe('');
    expect(resized).not.toHaveBeenCalled();

    post(iframe.contentWindow, ORIGIN, { type: 'arenile:resize', height: 640.2 });
    expect(iframe.style.height).toBe('641px');
    expect(resized).toHaveBeenCalledTimes(1);

    post(iframe.contentWindow, ORIGIN, { type: 'arenile:resize', height: -5 });
    expect(iframe.style.height).toBe('641px');

    post(iframe.contentWindow, ORIGIN, { type: 'arenile:booked', code: 'AR-1234' });
    expect(booked).toHaveBeenCalledTimes(1);
    expect((booked.mock.calls[0]![0] as CustomEvent).detail).toEqual({
      type: 'arenile:booked',
      code: 'AR-1234',
    });

    post(iframe.contentWindow, ORIGIN, { type: 'something:else' });
    expect(booked).toHaveBeenCalledTimes(1);
  });

  it('lets the host cancel navigation requests', async () => {
    await loadEmbed();
    const iframe = document.querySelector('iframe')!;
    const host = document.getElementById('w')!;
    const onNavigate = vi.fn((e: Event) => e.preventDefault());
    host.addEventListener('arenile:navigate', onNavigate);
    post(iframe.contentWindow, ORIGIN, {
      type: 'arenile:navigate',
      url: 'https://checkout.example/pay',
    });
    expect(onNavigate).toHaveBeenCalledTimes(1);
  });
});
