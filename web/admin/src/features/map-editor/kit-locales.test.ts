import { KIT_CATEGORIES, KIT_MANIFEST } from '@arenile/shared/map';
import { describe, expect, it } from 'vitest';
import de from './locales/de.json';
import en from './locales/en.json';
import fr from './locales/fr.json';
import itMessages from './locales/it.json';

describe('map kit labels', () => {
  it('Italian labels are the kit manifest labels (source language)', () => {
    for (const a of KIT_MANIFEST) {
      expect(itMessages.kit.asset[a.id as keyof typeof itMessages.kit.asset], a.id).toBe(a.label);
    }
  });

  it.each([
    ['en', en],
    ['de', de],
    ['fr', fr],
  ])('%s translates every asset and category', (_lang, messages) => {
    for (const a of KIT_MANIFEST) {
      expect(messages.kit.asset[a.id as keyof typeof messages.kit.asset], a.id).toBeTruthy();
    }
    for (const c of KIT_CATEGORIES) expect(messages.kit.category[c], c).toBeTruthy();
  });
});
