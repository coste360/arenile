import { describe, expect, it } from 'vitest';
import { contrastRatio, parseHexColor, type Rgb } from './contrast';
import {
  DARK_SURFACE,
  DEFAULT_PRIMARY,
  applyTheme,
  customText,
  resolveTheme,
  themeToCssVars,
} from './theme';

describe('resolveTheme', () => {
  it('applies defaults to an empty theme', () => {
    const t = resolveTheme(undefined);
    expect(t.primary).toBe(DEFAULT_PRIMARY);
    expect(t.primaryForeground).toBe('#ffffff');
    expect(t.radius).toBe('0.5rem');
    expect(t.showPricesOnMap).toBe(true);
    expect(t.hideHeaderInEmbed).toBe(true);
  });

  it('rejects unsafe values', () => {
    const t = resolveTheme({
      primary_color: 'red; background:url(evil)',
      logo_url: 'javascript:alert(1)',
      radius: 'huge',
      font_family: 'Comic',
    });
    expect(t.primary).toBe(DEFAULT_PRIMARY);
    expect(t.logoUrl).toBeNull();
    expect(t.radius).toBe('0.5rem');
    expect(t.fontFamily).toContain('system-ui');
  });

  it('computes a readable foreground for light primaries', () => {
    const t = resolveTheme({ primary_color: '#FFE600', radius: 'lg', font_family: 'serif' });
    expect(t.primary).toBe('#ffe600');
    expect(t.primaryForeground).toBe('#111827');
    expect(t.radius).toBe('1rem');
    expect(t.fontFamily).toContain('Georgia');
  });
});

describe('themeToCssVars / applyTheme', () => {
  it('exposes css variables and writes them on an element', () => {
    const vars = themeToCssVars({ primary_color: '#000000' });
    expect(vars['--primary']).toBe('#000000');
    expect(vars['--primary-foreground']).toBe('#ffffff');
    const el = document.createElement('div');
    applyTheme({ primary_color: '#000000' }, el);
    expect(el.style.getPropertyValue('--primary')).toBe('#000000');
    expect(el.getAttribute('data-themed')).toBe('');
    expect(Object.keys(vars).some((k) => k.startsWith('--ar-'))).toBe(false);
    // Never the scheme-independent variable: the .dark rule must stay in charge.
    expect(vars).not.toHaveProperty('--primary-on-surface');
  });

  it('keeps links and prices AA in both colour schemes (spec 12.6)', () => {
    const ratio = (a: string, b: string) =>
      contrastRatio(parseHexColor(a) as Rgb, parseHexColor(b) as Rgb);
    for (const primary of [DEFAULT_PRIMARY, '#0e6ba8', '#1a1a1a', '#ff6600', '#ffe600', '#2563eb']) {
      const t = resolveTheme({ primary_color: primary });
      expect(ratio(t.primaryOnSurface, '#ffffff')).toBeGreaterThanOrEqual(4.5);
      expect(ratio(t.primaryOnSurfaceDark, DARK_SURFACE)).toBeGreaterThanOrEqual(4.5);
      const vars = themeToCssVars(t);
      expect(vars['--primary-on-surface-light']).toBe(t.primaryOnSurface);
      expect(vars['--primary-on-surface-dark']).toBe(t.primaryOnSurfaceDark);
    }
  });
});

describe('customText', () => {
  it('falls back to the default locale', () => {
    const t = resolveTheme({ custom_texts: { it: { intro: 'Benvenuti' } } });
    expect(customText(t, 'intro', 'de', 'it')).toBe('Benvenuti');
    expect(customText(t, 'missing', 'de', 'it')).toBeUndefined();
  });
});
