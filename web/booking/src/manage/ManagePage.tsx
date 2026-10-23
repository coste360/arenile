import { ApiProblem } from '@arenile/shared/api';
import { formatInstantDate } from '@arenile/shared/lib/dates';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@arenile/shared/ui/collapsible';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  ArrowLeftIcon,
  CalendarPlusIcon,
  CalendarX2Icon,
  ChevronDownIcon,
  CreditCardIcon,
  PencilIcon,
  SunIcon,
  UmbrellaIcon,
  type LucideIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  manageDocumentUrl,
  newIdempotencyKey,
  useManagedBooking,
  useManagePay,
  type ManageBooking,
} from '../api/public';
import { BookingDetails } from '../confirmation/BookingDetails';
import { svgDataUrl } from '../confirmation/documents';
import { BookingEnvProvider } from '../flow/context';
import { formatMoney, intlLocale } from '../flow/format';
import { redirectToPayment } from '../flow/navigate';
import { problemMessageKey } from '../flow/problems';
import { usePublicPage } from '../flow/usePublicPage';
import { PUBLIC_NS } from '../i18n';
import type { BookingLocation } from '../location';
import { CancelSection } from './CancelSection';
import { ModifySection } from './ModifySection';
import { ReleaseSection } from './ReleaseSection';
import { RequestLinkForm } from './RequestLinkForm';

export interface ManagePageProps {
  location: BookingLocation;
  win?: Window;
  now?: () => number;
}

function ActionPanel({
  icon: Icon,
  title,
  children,
}: {
  icon: LucideIcon;
  title: string;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-xl border bg-card">
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="h-12 w-full justify-start gap-2 rounded-xl px-4">
          <Icon aria-hidden="true" />
          <span className="flex-1 text-left font-medium">{title}</span>
          <ChevronDownIcon
            aria-hidden="true"
            className={open ? 'rotate-180 transition-transform' : 'transition-transform'}
          />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="px-4 pb-4">{children}</CollapsibleContent>
    </Collapsible>
  );
}

function PayBalance({
  slug,
  token,
  booking,
  win,
}: {
  slug: string;
  token: string;
  booking: ManageBooking;
  win: Window;
}) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const pay = useManagePay(slug, token);
  const amount = formatMoney(booking.summary.balance_due, booking.summary.currency, i18n.language);
  return (
    <div className="space-y-2">
      <Button
        className="h-11 w-full sm:w-auto"
        disabled={pay.isPending}
        onClick={() =>
          pay.mutate(
            { idempotencyKey: newIdempotencyKey() },
            {
              onSuccess: (r) => {
                if (r.mode === 'redirect' && r.redirect_url) {
                  redirectToPayment(r.redirect_url, win, null);
                }
              },
            },
          )
        }
      >
        {pay.isPending ? <Spinner /> : <CreditCardIcon aria-hidden="true" />}
        {t('manage.payBalanceAmount', { amount })}
      </Button>
      {pay.error && (
        <p role="alert" className="text-sm text-destructive">
          {tc(problemMessageKey(pay.error))}
        </p>
      )}
    </div>
  );
}

/**
 * Customer manage area at `/b/{slug}/m/{token}` (spec 12.4), lazy-loaded
 * from main.tsx: details and QR, .ics, pay the balance, modify / cancel
 * within the policy with a preview of the new price or of the refund,
 * release days for season pass holders, and "send me a new link" when the
 * link is invalid or expired (404 without telling which).
 */
export function ManagePage({ location, win = window, now = Date.now }: ManagePageProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const { query: est, establishment, env } = usePublicPage(location, win, now);
  const slug = location.slug ?? '';
  const token = location.manageToken;
  const booking = useManagedBooking(slug, token);

  const invalid =
    !location.slug ||
    !token ||
    (booking.error instanceof ApiProblem && booking.error.status === 404);

  let content: ReactNode;
  if (invalid) {
    content = (
      <Card>
        <CardHeader>
          <CardTitle>
            <h1 className="text-xl">{t('manage.title')}</h1>
          </CardTitle>
          <CardDescription role="alert">{t('manage.linkInvalid')}</CardDescription>
        </CardHeader>
        <CardContent>{location.slug && <RequestLinkForm slug={location.slug} />}</CardContent>
      </Card>
    );
  } else if (booking.isPending || est.isPending || !env) {
    content = (
      <div role="status" className="space-y-4">
        <span className="sr-only">{tc('common.loading')}</span>
        <Skeleton aria-hidden="true" className="h-10 w-1/2" />
        <Skeleton aria-hidden="true" className="h-64 w-full" />
      </div>
    );
  } else if (booking.isError || !booking.data) {
    content = (
      <p role="alert" className="text-center text-muted-foreground">
        {tc(problemMessageKey(booking.error))}
      </p>
    );
  } else {
    const b = booking.data;
    const allowed = b.allowed;
    const anyAction =
      (allowed.pay_balance && b.summary.balance_due > 0) ||
      allowed.modify ||
      allowed.cancel ||
      (b.is_season_pass && allowed.release_days);
    content = (
      <div className="space-y-6">
        <Card>
          <CardHeader className="gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle>
                <h1 className="text-xl">{t('manage.title')}</h1>
              </CardTitle>
              <Badge variant={b.status === 'confirmed' ? 'default' : 'secondary'}>
                {t(`manage.status.${b.status}`)}
              </Badge>
            </div>
            <CardDescription>
              {t('manage.customer')}: {b.customer.first_name} {b.customer.last_name}
              {b.customer.email_masked ? ` · ${b.customer.email_masked}` : ''}
            </CardDescription>
          </CardHeader>
          <CardContent className="grid gap-6 sm:grid-cols-[1fr_auto]">
            <BookingDetails
              summary={b.summary}
              fallbackLocale={establishment?.default_locale ?? 'it'}
            />
            <div className="flex flex-col items-center gap-3">
              <p className="font-mono text-xl font-semibold tracking-widest">{b.code}</p>
              {b.qr_svg && (
                <img
                  src={svgDataUrl(b.qr_svg)}
                  alt={t('confirmation.qr')}
                  className="size-44 rounded-lg bg-white p-2"
                />
              )}
              <Button asChild variant="outline" className="h-11">
                <a href={manageDocumentUrl(slug, token, 'ics')} download={`${b.code}.ics`}>
                  <CalendarPlusIcon aria-hidden="true" />
                  {t('confirmation.addToCalendar')}
                </a>
              </Button>
              <p className="text-xs text-muted-foreground">
                {t('manage.linkExpires', {
                  date: formatInstantDate(b.link_expires_at, {
                    locale: intlLocale(i18n.language),
                    timeZone: establishment?.timezone,
                  }),
                })}
              </p>
            </div>
          </CardContent>
        </Card>

        {allowed.pay_balance && b.summary.balance_due > 0 && (
          <PayBalance slug={slug} token={token} booking={b} win={win} />
        )}
        <div className="space-y-3">
          {allowed.modify && (
            <ActionPanel icon={PencilIcon} title={t('manage.modifyTitle')}>
              <ModifySection slug={slug} token={token} booking={b} />
            </ActionPanel>
          )}
          {b.is_season_pass && allowed.release_days && (
            <ActionPanel icon={SunIcon} title={t('manage.releaseTitle')}>
              <ReleaseSection slug={slug} token={token} booking={b} />
            </ActionPanel>
          )}
          {allowed.cancel && (
            <ActionPanel icon={CalendarX2Icon} title={t('manage.cancelTitle')}>
              <CancelSection slug={slug} token={token} booking={b} />
            </ActionPanel>
          )}
        </div>
        {!anyAction && <p className="text-sm text-muted-foreground">{t('manage.noActions')}</p>}
      </div>
    );
  }

  return (
    <div className="flex min-h-svh flex-col bg-muted/30" lang={i18n.language}>
      <header className="bg-primary text-primary-foreground shadow-sm">
        <div className="mx-auto flex max-w-3xl items-center gap-3 px-4 py-3">
          <UmbrellaIcon aria-hidden="true" className="size-5" />
          <p className="min-w-0 flex-1 truncate font-semibold">{establishment?.name ?? ''}</p>
          {location.slug && (
            <Button
              asChild
              variant="ghost"
              size="sm"
              className="h-11 text-primary-foreground hover:bg-primary-foreground/10 hover:text-primary-foreground"
            >
              <a href={`/b/${encodeURIComponent(location.slug)}`}>
                <ArrowLeftIcon aria-hidden="true" />
                {t('manage.back')}
              </a>
            </Button>
          )}
        </div>
      </header>
      <main className="mx-auto w-full max-w-3xl flex-1 px-4 py-6 sm:py-8">
        <BookingEnvProvider value={env}>{content}</BookingEnvProvider>
      </main>
    </div>
  );
}

export default ManagePage;
