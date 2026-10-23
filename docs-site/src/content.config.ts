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

/**
 * Product hero of the home pages (rendered by the Hero override when present):
 * the one-line key facts under the H1, a text link for technical readers and
 * the panel screenshot (plus the booking page on a phone) under the copy.
 */
const home = z
  .object({
    facts: z.string(),
    techLink: z.object({ text: z.string(), link: z.string() }),
    shot: z.object({ name: z.string(), alt: z.string() }),
    phone: z.object({ name: z.string(), alt: z.string() }).optional(),
  })
  .optional();

export const collections = {
  docs: defineCollection({ loader: docsLoader(), schema: docsSchema({ extend: z.object({ home }) }) }),
  // UI strings of Starlight overridden per locale (src/content/i18n/<lang>.json).
  i18n: defineCollection({ loader: i18nLoader(), schema: i18nSchema({ extend: arenileStrings }) }),
};
