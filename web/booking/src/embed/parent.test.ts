import { describe, expect, it, vi } from 'vitest';
import { connectParent, observeHeight, resolveParentOrigin } from './parent';

describe('resolveParentOrigin', () => {
  it.each([
    ['not framed', { framed: false, declared: 'https://host.it' }, null],
    [
      'ancestorOrigins wins',
      {
        framed: true,
        declared: null,
        ancestorOrigins: ['https://lido.it'],
        referrer: 'https://x.it/',
      },
      'https://lido.it',
    ],
    [
      'declared matches ancestor',
      { framed: true, declared: 'https://lido.it', ancestorOrigins: ['https://lido.it'] },
      'https://lido.it',
    ],
    [
      'declared differs from ancestor',
      { framed: true, declared: 'https://evil.it', ancestorOrigins: ['https://lido.it'] },
      null,
    ],
    [
      'referrer fallback',
      { framed: true, declared: null, referrer: 'https://lido.it/prenota?x=1' },
      'https://lido.it',
    ],
    [
      'declared differs from referrer',
      { framed: true, declared: 'https://evil.it', referrer: 'https://lido.it/' },
      null,
    ],
    [
      'declared without any evidence',
      { framed: true, declared: 'https://lido.it', referrer: '' },
      null,
    ],
    [
      'invalid declared value',
      { framed: true, declared: 'javascript:alert(1)', referrer: 'https://lido.it/' },
      null,
    ],
    [
      'non-http ancestor',
      { framed: true, declared: null, ancestorOrigins: ['file://'], referrer: '' },
      null,
    ],
  ] as const)('%s', (_name, inputs, expected) => {
    expect(resolveParentOrigin(inputs)).toBe(expected);
  });
});

describe('connectParent / observeHeight', () => {
  it('posts resize messages only to the verified origin', () => {
    const postMessage = vi.fn();
    const fakeWin = {
      parent: { postMessage },
      location: { ancestorOrigins: ['https://lido.it'] },
      document: { referrer: '', documentElement: { scrollHeight: 812.4 }, body: document.body },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as Window;
    const channel = connectParent(fakeWin, 'https://lido.it');
    expect(channel?.origin).toBe('https://lido.it');
    const stop = observeHeight(fakeWin, channel!);
    expect(postMessage).toHaveBeenCalledWith(
      { type: 'arenile:resize', height: 813 },
      'https://lido.it',
    );
    stop();
  });

  it('returns null when the page is not framed', () => {
    expect(connectParent(window, null)).toBeNull();
  });
});
