/**
 * Registry of the documentation screenshots (spec 28, M4).
 *
 * `screenshots.manifest.json` (docs-site root) is the single source of truth
 * shared with the capture pipeline (e2e/docs): every name there is captured
 * in light and dark into src/assets/screenshots/{light,dark}/<name>.webp.
 * This module joins the manifest with the images that exist on disk, so the
 * <Screenshot> component can fail fast on a typo and degrade gracefully when
 * the images were not generated yet.
 */
import type { ImageMetadata } from 'astro';
import manifest from '../../screenshots.manifest.json';

export type Theme = 'light' | 'dark';
export type ViewportName = keyof typeof manifest.viewports;

export interface ScreenshotEntry {
  name: string;
  section: string;
  route: string;
  viewport: ViewportName;
  auth?: boolean;
  description: string;
}

export interface ResolvedScreenshot extends ScreenshotEntry {
  size: { width: number; height: number };
  images: Partial<Record<Theme, ImageMetadata>>;
}

const files = import.meta.glob<{ default: ImageMetadata }>(
  '../assets/screenshots/{light,dark}/*.{webp,png}',
  { eager: true },
);

const byKey = new Map<string, ImageMetadata>();
for (const [path, mod] of Object.entries(files)) {
  const match = /\/(light|dark)\/([^/]+)\.(webp|png)$/.exec(path);
  if (!match) continue;
  const [, theme, name, ext] = match;
  const key = `${theme}/${name}`;
  // WebP wins over a leftover PNG of the same name.
  if (ext === 'webp' || !byKey.has(key)) byKey.set(key, mod.default);
}

export const SCREENSHOTS: readonly ScreenshotEntry[] = manifest.screenshots as ScreenshotEntry[];

const entries = new Map(SCREENSHOTS.map((s) => [s.name, s]));

/** Looks up a screenshot by name; throws on an unknown name (typos fail the build). */
export function resolveScreenshot(name: string): ResolvedScreenshot {
  const entry = entries.get(name);
  if (!entry) {
    throw new Error(
      `<Screenshot name="${name}">: unknown name. Add it to docs-site/screenshots.manifest.json ` +
        `(known: ${[...entries.keys()].join(', ')})`,
    );
  }
  return {
    ...entry,
    size: manifest.viewports[entry.viewport],
    images: {
      light: byKey.get(`light/${name}`),
      dark: byKey.get(`dark/${name}`),
    },
  };
}

/** True when missing images must fail the build (CI and releases). */
export const STRICT = (process.env.ARENILE_DOCS_STRICT ?? '') === '1';
