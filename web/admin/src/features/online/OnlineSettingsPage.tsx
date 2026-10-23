import { PlaceholderPage } from '../../components/PlaceholderPage';

/**
 * "Pagina pubblica" (spec 12, 15.1): booking on/off, theme editor (logo,
 * primary/accent colours with automatic contrast, font, radius, prices on
 * the map, header in embed, custom texts per language) with a LIVE preview
 * iframe of the real page (`/b/{slug}?embed=1&preview=1`, theme applied
 * through postMessage before saving), allowed embed origins, online flow
 * options (formulas, slots, rate periods in the calendar, promo codes,
 * phone required), terms versions (publish new version) and the embed
 * snippets (iframe and embed.js) with copy buttons. API:
 * GET/PUT online-settings (If-Match), GET/POST terms-versions.
 * Owner: admin online builder.
 */
export function OnlineSettingsPage() {
  return (
    <PlaceholderPage
      titleKey="online:settings.title"
      descriptionKey="online:settings.description"
      milestone="M4"
    />
  );
}
