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

function setProperty(property: string, content: string) {
  let el = document.head.querySelector<HTMLMetaElement>(`meta[property="${property}"]`);
  if (!el) {
    el = document.createElement('meta');
    el.setAttribute('property', property);
    document.head.appendChild(el);
  }
  el.content = content;
}

/** Arenile defaults of index.html, used when the establishment has no logo. */
export const DEFAULT_ICON = '/b/icon.svg';
export const DEFAULT_OG_IMAGE = '/b/og-image.png';

/**
 * Favicon and social image: the establishment logo when it has one (its
 * branding stays primary), the Arenile marks of index.html otherwise.
 */
export function applyBrandImages(logoUrl: string | null): void {
  for (const link of document.head.querySelectorAll<HTMLLinkElement>('link[rel="icon"]')) {
    link.dataset.defaultHref ??= link.getAttribute('href') ?? DEFAULT_ICON;
    link.dataset.defaultType ??= link.getAttribute('type') ?? '';
    link.dataset.defaultSizes ??= link.getAttribute('sizes') ?? '';
    if (logoUrl) {
      link.href = logoUrl;
      link.removeAttribute('type');
      link.removeAttribute('sizes');
    } else {
      link.href = link.dataset.defaultHref;
      if (link.dataset.defaultType) link.type = link.dataset.defaultType;
      if (link.dataset.defaultSizes) link.setAttribute('sizes', link.dataset.defaultSizes);
    }
  }
  // Open Graph wants absolute URLs; the size tags describe only the Arenile card.
  setProperty('og:image', new URL(logoUrl ?? DEFAULT_OG_IMAGE, window.location.origin).href);
  for (const property of ['og:image:width', 'og:image:height']) {
    const el = document.head.querySelector<HTMLMetaElement>(`meta[property="${property}"]`);
    if (el) {
      el.dataset.defaultContent ??= el.content;
      el.content = logoUrl ? '' : el.dataset.defaultContent;
    }
  }
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
  opts: { embed: boolean; title: string; description: string; logoUrl?: string | null },
): void {
  document.title = `${e.name} – ${opts.title}`;
  setMeta('description', opts.description);
  setMeta('robots', opts.embed ? 'noindex' : 'index,follow');
  setProperty('og:title', document.title);
  setProperty('og:description', opts.description);
  // The resolved (http/https only) logo of the theme, never the raw value.
  applyBrandImages(opts.logoUrl ?? null);
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
