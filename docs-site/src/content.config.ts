import { defineCollection } from 'astro:content';
import { z } from 'astro/zod';
import { docsLoader, i18nLoader } from '@astrojs/starlight/loaders';
import { docsSchema, i18nSchema } from '@astrojs/starlight/schema';
import italian from './content/i18n/it.json';

/**
 * Strings of the Arenile components (`arenile.*` keys): declared from the
 * Italian source file, so `Astro.locals.t('arenile.…')` is type-checked and
 * every locale is validated against the same keys.
 */
const arenileKeys = Object.keys(italian).filter((k) => k.startsWith('arenile.'));
const arenileStrings = z.object(Object.fromEntries(arenileKeys.map((k) => [k, z.string().optional()])));

export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema() }),
  // UI strings of Starlight overridden per locale (src/content/i18n/<lang>.json).
  i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema({ extend: arenileStrings }) }),
};
