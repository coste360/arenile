/**
 * Registry of the documentation components (import them in .mdx pages):
 *
 *   import { Screenshot, LiveMap, LivePriceSimulator, StatusBadges, ApiOperation }
 *     from '~/components';
 *
 * - Screenshot          a captured screen (screenshots.manifest.json), light/dark, zoom
 * - LiveMap             the real BeachMap + map kit on the demo map
 * - LivePriceSimulator  real pricing engine output on the demo price list
 * - StatusBadges        spot status badges of web/shared (static)
 * - ApiOperation        method + path + summary from api/openapi.yaml, linked to the reference
 *
 * Live components reuse web/shared (alias @arenile/shared): no copies, no
 * re-implemented business logic. The showcase at /componenti/ renders them all.
 */
export { default as Screenshot } from './Screenshot.astro';
export { default as LiveMap } from './LiveMap.astro';
export { default as LivePriceSimulator } from './LivePriceSimulator.astro';
export { default as StatusBadges } from './StatusBadges.astro';
export { default as ApiOperation } from './ApiOperation.astro';

// Home page building blocks (src/content/docs/index.mdx and en/index.mdx).
export { default as SectionIntro } from './home/SectionIntro.astro';
export { default as FeatureRow } from './home/FeatureRow.astro';
export { default as FeatureCards } from './home/FeatureCards.astro';
export { default as StartSteps } from './home/StartSteps.astro';
export { default as Faq } from './home/Faq.astro';
export { default as TechPanel } from './home/TechPanel.astro';
export { default as HelpCta } from './home/HelpCta.astro';
