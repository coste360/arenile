import { ApiProblem, usePublicEstablishment } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { SUPPORTED_LOCALES, changeLocale, isLocale, resolveLocale } from '@arenile/shared/i18n';
import { applyTheme, customText, resolveTheme } from '@arenile/shared/theme';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Separator } from '@arenile/shared/ui/separator';
import {
  InfoIcon,
  MailIcon,
  MapPinIcon,
  PhoneIcon,
  RotateCwIcon,
  UmbrellaIcon,
} from 'lucide-react';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { connectParent, observeHeight } from './embed/parent';
import type { BookingLocation } from './location';
import { applySeo } from './seo';
import { LanguageSelect } from './LanguageSelect';
import { Stepper } from './steps/Stepper';

export interface BookingPageProps {
  location: BookingLocation;
  /** Window used for embedding and SEO side effects (injectable for tests). */
  win?: Window;
}

function Message({ children, role = 'status' }: { children: string; role?: 'status' | 'alert' }) {
  return (
    <main className="mx-auto max-w-xl px-4 py-12 text-center">
      <p role={role} className="text-muted-foreground">
        {children}
      </p>
    </main>
  );
}

function Loading({ label }: { label: string }) {
  return (
    <main className="mx-auto max-w-3xl space-y-4 px-4 py-8">
      <p role="status" className="sr-only">
        {label}
      </p>
      <Skeleton aria-hidden="true" className="h-8 w-2/3" />
      <Skeleton aria-hidden="true" className="h-20 w-full" />
      <Skeleton aria-hidden="true" className="h-64 w-full" />
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
  if (query.isPending) return <Loading label={t('common.loading')} />;
  if (query.isError) {
    const notFound = query.error instanceof ApiProblem && query.error.status === 404;
    return (
      <main className="mx-auto max-w-xl space-y-4 px-4 py-12 text-center">
        <p role="alert" className="text-muted-foreground">
          {notFound
            ? t('booking.notFound')
            : t(query.error.isNetworkError ? 'common.errors.network' : 'common.errors.generic')}
        </p>
        {!notFound && (
          <Button onClick={() => void query.refetch()}>
            <RotateCwIcon aria-hidden="true" />
            {t('common.retry')}
          </Button>
        )}
      </main>
    );
  }

  const e = establishment!;
  const showHeader = !(location.embed && theme.hideHeaderInEmbed);
  const intro = customText(theme, 'intro', i18n.language, e.default_locale);
  const locales = e.enabled_locales.filter(isLocale);
  const languages = locales.length > 0 ? locales : [...SUPPORTED_LOCALES];

  const address = e.address;
  const addressLine = address
    ? [
        address.street,
        [address.postal_code, address.city].filter(Boolean).join(' '),
        address.province,
      ]
        .filter(Boolean)
        .join(', ')
    : '';
  const hasContacts = !!(e.contact_phone || e.contact_email || addressLine);

  return (
    <div className="flex min-h-svh flex-col bg-muted/30">
      {showHeader && (
        <header className="bg-primary text-primary-foreground shadow-sm">
          <div className="mx-auto flex max-w-5xl items-center gap-3 px-4 py-3">
            {theme.logoUrl ? (
              <img
                src={theme.logoUrl}
                alt=""
                className="h-10 w-auto max-w-32 rounded-md object-contain"
              />
            ) : (
              <span
                aria-hidden="true"
                className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary-foreground/15"
              >
                <UmbrellaIcon className="size-5" />
              </span>
            )}
            <div className="min-w-0 flex-1">
              <p className="truncate text-lg leading-tight font-semibold">{e.name}</p>
              {address?.city && (
                <p className="truncate text-xs text-primary-foreground">{address.city}</p>
              )}
            </div>
            {languages.length > 1 && <LanguageSelect languages={languages} />}
          </div>
        </header>
      )}
      <main className="mx-auto grid w-full max-w-5xl flex-1 gap-6 px-4 py-6 sm:py-8 lg:grid-cols-[minmax(0,1fr)_18rem] lg:items-start">
        <div className="space-y-6">
          <div className="space-y-1.5">
            <h1 className="text-2xl font-semibold tracking-tight text-balance sm:text-3xl">
              {showHeader ? t('booking.title') : e.name}
            </h1>
            {!showHeader && <p className="text-sm text-muted-foreground">{t('booking.title')}</p>}
            {(address?.city || e.season) && (
              <p className="text-sm text-muted-foreground">
                {[
                  address?.city,
                  e.season &&
                    t('booking.season', {
                      year: e.season.year,
                      from: formatIsoDate(e.season.opens_on),
                      to: formatIsoDate(e.season.closes_on),
                    }),
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </p>
            )}
            {intro && <p className="text-pretty text-muted-foreground">{intro}</p>}
          </div>
          {e.booking_enabled ? (
            <Stepper season={e.season} />
          ) : (
            <Alert>
              <InfoIcon aria-hidden="true" />
              <AlertDescription role="status">{t('booking.disabled')}</AlertDescription>
            </Alert>
          )}
        </div>
        {hasContacts && (
          <Card className="gap-3 py-4 lg:sticky lg:top-6" aria-labelledby="contacts">
            <CardHeader className="px-4">
              <CardTitle>
                <h2 id="contacts" className="text-base">
                  {t('booking.contact')}
                </h2>
              </CardTitle>
            </CardHeader>
            <CardContent className="px-4">
              <ul className="space-y-2.5 text-sm">
                {e.contact_phone && (
                  <li className="flex items-center gap-2">
                    <PhoneIcon
                      aria-hidden="true"
                      className="size-4 shrink-0 text-muted-foreground"
                    />
                    <a
                      className="text-primary-on-surface underline-offset-4 hover:underline"
                      href={`tel:${e.contact_phone.replace(/\s+/g, '')}`}
                    >
                      {e.contact_phone}
                    </a>
                  </li>
                )}
                {e.contact_email && (
                  <li className="flex items-center gap-2">
                    <MailIcon
                      aria-hidden="true"
                      className="size-4 shrink-0 text-muted-foreground"
                    />
                    <a
                      className="min-w-0 break-all text-primary-on-surface underline-offset-4 hover:underline"
                      href={`mailto:${e.contact_email}`}
                    >
                      {e.contact_email}
                    </a>
                  </li>
                )}
                {addressLine && (
                  <li className="flex items-start gap-2 text-muted-foreground">
                    <MapPinIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
                    <span>{addressLine}</span>
                  </li>
                )}
              </ul>
            </CardContent>
          </Card>
        )}
      </main>
      <footer className="mx-auto w-full max-w-5xl px-4 pb-8">
        <Separator className="mb-4" />
        <p className="flex items-center justify-center gap-1.5 text-xs text-muted-foreground">
          <UmbrellaIcon aria-hidden="true" className="size-3.5" />
          {t('booking.poweredBy')}
        </p>
      </footer>
    </div>
  );
}
