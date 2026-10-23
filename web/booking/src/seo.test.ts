import { afterEach, describe, expect, it } from 'vitest';
import { DEFAULT_OG_IMAGE, applyBrandImages } from './seo';

const icons = () => [...document.head.querySelectorAll<HTMLLinkElement>('link[rel="icon"]')];
const ogImage = () =>
  document.head.querySelector<HTMLMetaElement>('meta[property="og:image"]')?.content;

describe('applyBrandImages', () => {
  afterEach(() => {
    document.head.innerHTML = '';
  });

  it('uses the establishment logo, then restores the Arenile defaults', () => {
    document.head.innerHTML =
      '<link rel="icon" type="image/svg+xml" href="/b/icon.svg"><link rel="icon" type="image/png" href="/b/favicon-32x32.png">';

    applyBrandImages('https://lido.example.com/logo.png');
    expect(icons().map((l) => l.getAttribute('href'))).toEqual([
      'https://lido.example.com/logo.png',
      'https://lido.example.com/logo.png',
    ]);
    expect(icons().every((l) => !l.hasAttribute('type'))).toBe(true);
    expect(ogImage()).toBe('https://lido.example.com/logo.png');

    applyBrandImages(null);
    expect(icons().map((l) => l.getAttribute('href'))).toEqual([
      '/b/icon.svg',
      '/b/favicon-32x32.png',
    ]);
    expect(icons().map((l) => l.type)).toEqual(['image/svg+xml', 'image/png']);
    expect(ogImage()).toBe(new URL(DEFAULT_OG_IMAGE, window.location.origin).href);
  });
});
