// Matomo on analytics.coste360.it, without cookies and honouring Do Not Track,
// so no consent banner is needed (privacy notice: PRIVACY_URL). Page views and
// links followed, only on the published site (ARENILE_DOCS_SITE, by default
// docs.arenile.app): dev and preview stay out of the statistics.
import { MATOMO } from '~/lib/constants.mjs';

const site = import.meta.env.SITE;
if (site && location.hostname === new URL(site).hostname) {
  const w = window as Window & { _paq?: unknown[][] };
  const paq = (w._paq = w._paq || []);
  paq.push(['disableCookies']);
  paq.push(['setDoNotTrack', true]);
  paq.push(['trackPageView']);
  paq.push(['enableLinkTracking']);
  paq.push(['setTrackerUrl', `${MATOMO.url}matomo.php`]);
  paq.push(['setSiteId', MATOMO.siteId]);

  const g = document.createElement('script');
  g.async = true;
  g.src = `${MATOMO.url}matomo.js`;
  document.head.append(g);
}
