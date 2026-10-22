import { ApiProblem, usePublicEstablishment } from '@arenile/shared/api';
import { SUPPORTED_LOCALES, changeLocale, isLocale, resolveLocale } from '@arenile/shared/i18n';
import { applyTheme, customText, resolveTheme } from '@arenile/shared/theme';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { connectParent, observeHeight } from './embed/parent';
import type { BookingLocation } from './location';
import { applySeo } from './seo';
import { Stepper } from './steps/Stepper';

export interface BookingPageProps {
  location: BookingLocation;
  /** Window used for embedding and SEO side effects (injectable for tests). */
  win?: Window;
}

function Message({ children, role = 'status' }: { children: string; role?: 'status' | 'alert' }) {
  return (
    <main className="mx-auto max-w-xl px-4 py-12 text-center">
      <p role={role} className="text-slate-700">
        {children}
      </p>
    </main>
  );
}

export function BookingPage({ location, win = window }: BookingPageProps) {
  const { t, i18n } = useTranslation();
  const query = usePublicEstablishment(location.slug);
  const establishment = query.data;
  const theme = useMemo(() => resolveTheme(establishment?.theme), [establishment?.theme]);

  // Language: ?lang, then browser, then establishment default (spec 20).
  useEffect(() => {
    if (!establishment) return;
    const desired = resolveLocale({
      param: location.lang,
      browser: win.navigator.languages,
      establishmentDefault: establishment.default_locale,
      enabled: establishment.enabled_locales,
    });
    if (desired !== i18n.language) void changeLocale(i18n, desired);
  }, [establishment, location.lang, i18n, win]);

  useEffect(() => {
    applyTheme(theme, win.document.documentElement);
  }, [theme, win]);

  useEffect(() => {
    if (!establishment) return;
    applySeo(establishment, {
      embed: location.embed,
      title: t('booking.title'),
      description:
        customText(theme, 'intro', i18n.language, establishment.default_locale) ??
        t('booking.poweredBy'),
    });
  }, [establishment, location.embed, t, theme, i18n.language]);

  // Embedded mode: report height to the verified host origin (spec 12.2).
  useEffect(() => {
    if (!location.embed) return;
    const channel = connectParent(win, location.parentOrigin);
    if (!channel) return;
    return observeHeight(win, channel);
  }, [location.embed, location.parentOrigin, win]);

  if (!location.slug) return <Message role="alert">{t('booking.notFound')}</Message>;
  if (query.isPending) return <Message>{t('common.loading')}</Message>;
  if (query.isError) {
    const notFound = query.error instanceof ApiProblem && query.error.status === 404;
    return (
      <main className="mx-auto max-w-xl px-4 py-12 text-center">
        <p role="alert" className="text-slate-700">
          {notFound
            ? t('booking.notFound')
            : t(query.error.isNetworkError ? 'common.errors.network' : 'common.errors.generic')}
        </p>
        {!notFound && (
          <button
            type="button"
            onClick={() => void query.refetch()}
            className="mt-4 h-10 rounded-[var(--ar-radius)] bg-[var(--ar-primary)] px-4 text-sm font-medium text-[var(--ar-primary-foreground)]"
          >
            {t('common.retry')}
          </button>
        )}
      </main>
    );
  }

  const e = establishment!;
  const showHeader = !(location.embed && theme.hideHeaderInEmbed);
  const intro = customText(theme, 'intro', i18n.language, e.default_locale);
  const locales = e.enabled_locales.filter(isLocale);
  const languages = locales.length > 0 ? locales : [...SUPPORTED_LOCALES];

  return (
    <div className="min-h-full" style={{ fontFamily: 'var(--ar-font-family)' }}>
      {showHeader && (
        <header className="bg-[var(--ar-primary)] text-[var(--ar-primary-foreground)]">
          <div className="mx-auto flex max-w-3xl items-center gap-3 px-4 py-3">
            {theme.logoUrl && (
              <img src={theme.logoUrl} alt="" className="h-10 w-auto max-w-[8rem] object-contain" />
            )}
            <p className="min-w-0 flex-1 truncate text-lg font-semibold">{e.name}</p>
            {languages.length > 1 && (
              <label className="flex items-center gap-1 text-sm">
                <span className="sr-only">{t('common.language')}</span>
                <select
                  value={i18n.language}
                  onChange={(ev) => {
                    if (isLocale(ev.target.value)) void changeLocale(i18n, ev.target.value);
                  }}
                  className="rounded border border-white/40 bg-transparent px-1 py-1 text-sm text-inherit"
                >
                  {languages.map((l) => (
                    <option key={l} value={l} className="text-slate-900">
                      {t(`common.languages.${l}`)}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </div>
        </header>
      )}
      <main className="mx-auto max-w-3xl space-y-5 px-4 py-6">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold">{showHeader ? t('booking.title') : e.name}</h1>
          {!showHeader && <p className="text-sm text-slate-600">{t('booking.title')}</p>}
          {intro && <p className="text-slate-700">{intro}</p>}
        </div>
        {e.booking_enabled ? (
          <Stepper />
        ) : (
          <p role="status" className="rounded-[var(--ar-radius)] bg-slate-100 p-4 text-slate-700">
            {t('booking.disabled')}
          </p>
        )}
        {(e.contact_phone || e.contact_email) && (
          <section aria-labelledby="contacts" className="text-sm text-slate-700">
            <h2 id="contacts" className="font-semibold">
              {t('booking.contact')}
            </h2>
            <ul>
              {e.contact_phone && (
                <li>
                  <a className="underline" href={`tel:${e.contact_phone.replace(/\s+/g, '')}`}>
                    {e.contact_phone}
                  </a>
                </li>
              )}
              {e.contact_email && (
                <li>
                  <a className="underline" href={`mailto:${e.contact_email}`}>
                    {e.contact_email}
                  </a>
                </li>
              )}
            </ul>
          </section>
        )}
      </main>
      <footer className="mx-auto max-w-3xl px-4 pb-6 text-xs text-slate-600">
        {t('booking.poweredBy')}
      </footer>
    </div>
  );
}
