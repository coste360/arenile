import type { PublicEstablishment } from '@arenile/shared/api';

function setMeta(name: string, content: string) {
  let el = document.head.querySelector<HTMLMetaElement>(`meta[name="${name}"]`);
  if (!el) {
    el = document.createElement('meta');
    el.name = name;
    document.head.appendChild(el);
  }
  el.content = content;
}

/** schema.org LocalBusiness data for the public page (spec 12.6). */
export function localBusinessJsonLd(e: PublicEstablishment, url: string): Record<string, unknown> {
  const data: Record<string, unknown> = {
    '@context': 'https://schema.org',
    '@type': 'LocalBusiness',
    name: e.name,
    url,
  };
  if (e.contact_phone) data.telephone = e.contact_phone;
  if (e.contact_email) data.email = e.contact_email;
  if (e.theme.logo_url) data.logo = e.theme.logo_url;
  const a = e.address;
  if (a && (a.street || a.city)) {
    data.address = {
      '@type': 'PostalAddress',
      ...(a.street ? { streetAddress: a.street } : {}),
      ...(a.city ? { addressLocality: a.city } : {}),
      ...(a.province ? { addressRegion: a.province } : {}),
      ...(a.postal_code ? { postalCode: a.postal_code } : {}),
      ...(a.country ? { addressCountry: a.country } : {}),
    };
  }
  return data;
}

/** Updates title, description, robots and structured data. */
export function applySeo(
  e: PublicEstablishment,
  opts: { embed: boolean; title: string; description: string },
): void {
  document.title = `${e.name} – ${opts.title}`;
  setMeta('description', opts.description);
  setMeta('robots', opts.embed ? 'noindex' : 'index,follow');
  const id = 'arenile-jsonld';
  document.getElementById(id)?.remove();
  if (opts.embed) return;
  const script = document.createElement('script');
  script.type = 'application/ld+json';
  script.id = id;
  const canonical = `${window.location.origin}${window.location.pathname}`;
  script.textContent = JSON.stringify(localBusinessJsonLd(e, canonical)).replace(/</g, '\\u003c');
  document.head.appendChild(script);
}
