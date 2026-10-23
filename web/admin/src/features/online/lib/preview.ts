export type Scheme = 'light' | 'dark';

/** URL of the real public page in preview mode (spec 12.5). */
export function previewUrl(slug: string, lang: string, scheme: Scheme): string {
  const q = new URLSearchParams({ embed: '1', preview: '1', lang, theme: scheme });
  return `/b/${encodeURIComponent(slug)}?${q.toString()}`;
}
