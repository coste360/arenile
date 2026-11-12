// Shared by astro.config.mjs and the components (plain JS: the config imports it).
/** Base path of the API reference generated from api/openapi.yaml (starlight-openapi). */
export const API_REFERENCE_BASE = 'api/riferimento';

/**
 * Matomo on Coste360's own server (site 3, shared with arenile.app): no cookies,
 * Do Not Track respected.
 */
export const MATOMO = { url: 'https://analytics.coste360.it/', siteId: '3' };

/** Privacy notice of arenile.app, linked from the footer. */
export const PRIVACY_URL = { it: 'https://arenile.app/privacy/', en: 'https://arenile.app/en/privacy/' };
