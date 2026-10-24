import { ApiProblem } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { SUPPORTED_LOCALES, isLocale } from '@arenile/shared/i18n';
import { customText } from '@arenile/shared/theme';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Separator } from '@arenile/shared/ui/separator';
import {
  FlaskConicalIcon,
  InfoIcon,
  MailIcon,
  MapPinIcon,
  PhoneIcon,
  RotateCwIcon,
} from 'lucide-react';
import { useEffect } from 'react';
// App icon and mono symbol only, not BrandLogo: that would bundle every logo variant into the
// budgeted page.
import { ARENILE_URL, ArenileAppIcon, ArenileMonoSymbol } from '@arenile/shared/ui/patterns/Brand';
import { useTranslation } from 'react-i18next';
import { BookingEnvProvider } from './flow/context';
import { usePublicPage } from './flow/usePublicPage';
import type { BookingLocation } from './location';
import { applySeo } from './seo';
import { LanguageSelect } from './LanguageSelect';
import { Stepper } from './steps/Stepper';
import { serverNow } from './flow/serverClock';

export interface BookingPageProps {
  location: BookingLocation;
  /** Window used for embedding and SEO side effects (injectable for tests). */
  win?: Window;
  /** Current time in ms (tests). */
  now?: () => number;
}

function Message({ children, role = 'status' }: { children: string; role?: 'status' | 'alert' }) {
  return (
    <main className="mx-auto max-w-xl px-4 py-12 text-center">
      {/* The message is the page's only heading (no establishment to name). */}
      <div role={role}>
        <h1 className="text-base font-normal text-muted-foreground">{children}</h1>
      </div>
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

export function BookingPage({ location, win = window, now = serverNow }: BookingPageProps) {
  const { t, i18n } = useTranslation();
  const { query, establishment, theme, env } = usePublicPage(location, win, now);

  useEffect(() => {
    if (!establishment) return;
    applySeo(establishment, {
      embed: location.embed,
      title: t('booking.title'),
      logoUrl: theme.logoUrl,
      description:
        customText(theme, 'intro', i18n.language, establishment.default_locale) ??
        t('booking.metaDescription'),
    });
  }, [establishment, location.embed, t, theme, i18n.language]);

  if (!location.slug) return <Message>{t('booking.notFound')}</Message>;
  if (query.isPending) return <Loading label={t('common.loading')} />;
  if (query.isError) {
    const notFound = query.error instanceof ApiProblem && query.error.status === 404;
    return (
      <main className="mx-auto max-w-xl space-y-4 px-4 py-12 text-center">
        {/* Not found is the page's static content; a failed load is an alert. */}
        <div role={notFound ? 'status' : 'alert'}>
          <h1 className="text-base font-normal text-muted-foreground">
            {notFound
              ? t('booking.notFound')
              : t(query.error.isNetworkError ? 'common.errors.network' : 'common.errors.generic')}
          </h1>
        </div>
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

  const heading = (
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
  );

  const contacts = hasContacts ? (
    <Card className="gap-3 py-4" aria-labelledby="contacts">
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
              <PhoneIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
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
              <MailIcon aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
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
  ) : null;

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
              <ArenileAppIcon className="size-10 rounded-lg" />
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
      <main className="flex flex-1 flex-col">
        {e.test_mode && (
          <div className="mx-auto w-full max-w-5xl px-4 pt-4">
            {/* A static notice of the page, not an alert announced on load. */}
            <Alert variant="warning" role="note">
              <FlaskConicalIcon aria-hidden="true" />
              <AlertDescription>{t('booking.testMode')}</AlertDescription>
            </Alert>
          </div>
        )}
        <BookingEnvProvider value={env}>
          {e.booking_enabled || location.preview ? (
            <Stepper
              header={heading}
              aside={contacts}
              returnedBookingId={location.checkoutBookingId}
            />
          ) : (
            <div className="mx-auto w-full max-w-5xl space-y-6 px-4 py-6 sm:py-8">
              {heading}
              <Alert role="status">
                <InfoIcon aria-hidden="true" />
                <AlertDescription>{t('booking.disabled')}</AlertDescription>
              </Alert>
              {contacts}
            </div>
          )}
        </BookingEnvProvider>
      </main>
      <footer className="mx-auto w-full max-w-5xl px-4 pb-8">
        <Separator className="mb-4" />
        <p className="flex items-center justify-center gap-1 text-xs text-muted-foreground">
          {t('booking.poweredBy')}
          <a
            href={ARENILE_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 font-medium underline-offset-4 hover:text-foreground hover:underline"
          >
            <ArenileMonoSymbol alt="" className="h-3.5" />
            Arenile
          </a>
        </p>
      </footer>
    </div>
  );
}
