import { ApiProblem } from '@arenile/shared/api';
import { formatInstant } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import { Label } from '@arenile/shared/ui/label';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  ArrowLeftIcon,
  CalendarCheckIcon,
  CircleAlertIcon,
  HourglassIcon,
  LogOutIcon,
  UmbrellaIcon,
  XCircleIcon,
} from 'lucide-react';
import { useId, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { newIdempotencyKey } from '../../api/public';
import { BookingEnvProvider } from '../../flow/context';
import { formatStay, intlLocale } from '../../flow/format';
import { problemMessageKey } from '../../flow/problems';
import { serverNow } from '../../flow/serverClock';
import { usePublicPage } from '../../flow/usePublicPage';
import type { BookingLocation } from '../../location';
import { timeLeft, useTicker } from '../renewal/time';
import {
  useAcceptOffer,
  useDeclineOffer,
  useLeaveWaitlist,
  useRefreshWaitlist,
  useWaitlistEntry,
  type PublicWaitlistEntry,
} from './api';
import { useWaitlistMessages, WAITLIST_NS } from './i18n';

export interface WaitlistOfferPageProps {
  location: BookingLocation;
  win?: Window;
  now?: () => number;
}

function problemKey(error: unknown): string {
  if (error instanceof ApiProblem) {
    if (error.type === 'waitlist_offer_expired' || error.status === 410)
      return `${WAITLIST_NS}:problem.offer_expired`;
    if (error.type === 'spot_unavailable') return `${WAITLIST_NS}:problem.spot_unavailable`;
  }
  return problemMessageKey(error);
}

function OfferPanel({
  slug,
  token,
  entry,
  win,
  now,
  timeZone,
}: {
  slug: string;
  token: string;
  entry: PublicWaitlistEntry & { offer: NonNullable<PublicWaitlistEntry['offer']> };
  win: Window;
  now: () => number;
  timeZone?: string;
}) {
  const { t, i18n } = useTranslation(WAITLIST_NS);
  const { t: tc } = useTranslation();
  const id = useId();
  const ticked = useTicker(now, 15_000);
  const left = timeLeft(entry.offer.expires_at, ticked);
  const accept = useAcceptOffer(slug, token);
  const decline = useDeclineOffer(slug, token);
  const refresh = useRefreshWaitlist(slug, token);
  const [key, setKey] = useState(newIdempotencyKey);
  const [declining, setDeclining] = useState(false);
  const [leave, setLeave] = useState(false);
  const expires = formatInstant(entry.offer.expires_at, {
    locale: intlLocale(i18n.language),
    timeZone,
  });

  if (left.expired) {
    return (
      <Alert variant="warning" role="status">
        <CircleAlertIcon aria-hidden="true" />
        <AlertTitle>{t('offer.expired')}</AlertTitle>
        <AlertDescription>{t('offer.expiredHint')}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4 rounded-xl border bg-card p-4">
      <div className="space-y-1">
        <p className="font-semibold">
          {entry.offer.spot_label
            ? t('offer.titleSpot', { spot: entry.offer.spot_label })
            : t('offer.title')}
        </p>
        <p className="text-sm">
          {t('offer.until', { date: expires })}{' '}
          <span className="font-medium">
            {left.days > 0
              ? t('offer.leftDays', { count: left.days })
              : t('offer.left', { hours: left.hours, minutes: left.minutes })}
          </span>
        </p>
      </div>
      {!declining ? (
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button
            className="h-11"
            disabled={accept.isPending}
            onClick={() =>
              accept.mutate(
                { idempotencyKey: key },
                {
                  // The held booking continues in the normal checkout (customer step, payment).
                  onSuccess: () => win.location.assign(`/b/${encodeURIComponent(slug)}`),
                  onError: (err) => {
                    setKey(newIdempotencyKey());
                    if (err instanceof ApiProblem && err.status === 410) void refresh();
                  },
                },
              )
            }
          >
            {accept.isPending ? <Spinner /> : <CalendarCheckIcon aria-hidden="true" />}
            {t('offer.accept')}
          </Button>
          <Button variant="outline" className="h-11" onClick={() => setDeclining(true)}>
            <XCircleIcon aria-hidden="true" />
            {t('offer.decline')}
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="flex items-start gap-3">
            <Checkbox
              id={`${id}-leave`}
              checked={leave}
              onCheckedChange={(v) => setLeave(v === true)}
              className="size-5"
            />
            <Label htmlFor={`${id}-leave`} className="leading-snug">
              {t('offer.leave')}
            </Label>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Button
              variant="destructive"
              className="h-11"
              disabled={decline.isPending}
              onClick={() => decline.mutate({ leave }, { onSuccess: () => setDeclining(false) })}
            >
              {decline.isPending && <Spinner />}
              {t('offer.declineConfirm')}
            </Button>
            <Button variant="outline" className="h-11" onClick={() => setDeclining(false)}>
              {t('back')}
            </Button>
          </div>
        </div>
      )}
      {(accept.error ?? decline.error) && (
        <p role="alert" className="text-sm text-destructive">
          {tc(problemKey(accept.error ?? decline.error))}
        </p>
      )}
    </div>
  );
}

/**
 * Personal waitlist link `/b/{slug}/m/w/{token}` (spec 10.5, M5B), lazy
 * loaded from main.tsx: the request and its status; when a spot frees up,
 * the offer with its deadline, "Prenota ora" (the held booking continues in
 * the normal checkout) or "Rinuncia"; "Esci dalla lista" while waiting.
 */
export default function WaitlistOfferPage({
  location,
  win = window,
  now = serverNow,
}: WaitlistOfferPageProps) {
  useWaitlistMessages();
  const { t, i18n } = useTranslation(WAITLIST_NS);
  const { t: tc } = useTranslation();
  const { query: est, establishment, env } = usePublicPage(location, win, now);
  const slug = location.slug ?? '';
  const token = location.waitlistToken;
  const entry = useWaitlistEntry(slug, token);
  const leave = useLeaveWaitlist(slug, token ?? '');
  const [left, setLeft] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);

  const invalid =
    !location.slug || !token || (entry.error instanceof ApiProblem && entry.error.status === 404);

  let content: ReactNode;
  if (invalid) {
    content = (
      <Card>
        <CardHeader>
          <CardTitle>
            <h1 className="text-xl">{t('title')}</h1>
          </CardTitle>
          <CardDescription role="alert">{t('linkInvalid')}</CardDescription>
        </CardHeader>
      </Card>
    );
  } else if (entry.isPending || est.isPending || !env) {
    content = (
      <div role="status" className="space-y-4">
        <span className="sr-only">{tc('common.loading')}</span>
        <Skeleton aria-hidden="true" className="h-10 w-1/2" />
        <Skeleton aria-hidden="true" className="h-48 w-full" />
      </div>
    );
  } else if (entry.isError || !entry.data) {
    content = (
      <p role="alert" className="text-center text-muted-foreground">
        {tc(problemMessageKey(entry.error))}
      </p>
    );
  } else {
    const e = entry.data;
    const status = left ? 'removed' : e.status;
    const pendingOffer = e.offer?.status === 'pending' && status === 'offered' ? e.offer : null;
    content = (
      <Card>
        <CardHeader className="gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <HourglassIcon aria-hidden="true" className="size-5 text-primary-on-surface" />
            <CardTitle>
              <h1 className="text-xl">{t('title')}</h1>
            </CardTitle>
            <Badge variant={status === 'converted' ? 'default' : 'secondary'}>
              {t(`status.${status}`)}
            </Badge>
          </div>
          <CardDescription>
            {formatStay(e.starts_on, e.ends_on)}
            {e.slots !== 'FULL' && ` · ${t(`slots.${e.slots}`)}`} ·{' '}
            {[e.pack_name, e.zone_name].filter(Boolean).join(' · ')}
            {(e.quantity ?? 1) > 1 && ` · ×${e.quantity}`}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {pendingOffer ? (
            <OfferPanel
              slug={slug}
              token={token}
              entry={{ ...e, offer: pendingOffer }}
              win={win}
              now={now}
              timeZone={establishment?.timezone}
            />
          ) : (
            <p className="text-sm" role="status">
              {t(`message.${status}`)}
            </p>
          )}
          {status === 'waiting' && !confirmLeave && (
            <Button variant="outline" className="h-11" onClick={() => setConfirmLeave(true)}>
              <LogOutIcon aria-hidden="true" />
              {t('leave')}
            </Button>
          )}
          {status === 'waiting' && confirmLeave && (
            <div className="space-y-3 rounded-xl border p-4">
              <p className="font-medium">{t('leaveQuestion')}</p>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Button
                  variant="destructive"
                  className="h-11"
                  disabled={leave.isPending}
                  onClick={() => leave.mutate(undefined, { onSuccess: () => setLeft(true) })}
                >
                  {leave.isPending && <Spinner />}
                  {t('leaveConfirm')}
                </Button>
                <Button variant="outline" className="h-11" onClick={() => setConfirmLeave(false)}>
                  {t('back')}
                </Button>
              </div>
              {leave.error && (
                <p role="alert" className="text-sm text-destructive">
                  {tc(problemMessageKey(leave.error))}
                </p>
              )}
            </div>
          )}
        </CardContent>
      </Card>
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
                {t('toBooking')}
              </a>
            </Button>
          )}
        </div>
      </header>
      <main className="mx-auto w-full max-w-3xl flex-1 px-4 py-6 sm:py-8">
        {env ? <BookingEnvProvider value={env}>{content}</BookingEnvProvider> : content}
      </main>
    </div>
  );
}
