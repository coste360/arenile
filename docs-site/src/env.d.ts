/// <reference path="../.astro/types.d.ts" />

// UI strings of the Arenile components (`arenile.*`, src/content/i18n/it.json)
// typed for Astro.locals.t().
declare namespace StarlightApp {
  type UIStrings = typeof import('./content/i18n/it.json');
  interface I18n extends UIStrings {}
}
