import { ApiProblem } from '@arenile/shared/api';
import { formatInstant } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@arenile/shared/ui/collapsible';
import { Label } from '@arenile/shared/ui/label';
import { RadioGroup, RadioGroupItem } from '@arenile/shared/ui/radio-group';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import {
  CheckCircle2Icon,
  ChevronDownIcon,
  CircleAlertIcon,
  RefreshCwIcon,
  XCircleIcon,
} from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { newIdempotencyKey, type ManageBooking } from '../../api/public';
import { useBookingEnv } from '../../flow/context';
import { formatMoney, formatStay, intlLocale } from '../../flow/format';
import { redirectToPayment } from '../../flow/navigate';
import { problemMessageKey } from '../../flow/problems';
import { serverNow } from '../../flow/serverClock';
import {
  useAcceptRenewal,
  useDeclineRenewal,
  useRefreshManage,
  type PublicRenewalProposal,
} from './api';
import { RENEWAL_NS, useRenewalMessages } from './i18n';
import { timeLeft, useTicker } from './time';

export interface RenewalSectionProps {
  slug: string;
  token: string;
  booking: ManageBooking & { renewal: PublicRenewalProposal };
  win: Window;
  now?: () => number;
}

/** Message of a failed renewal call: the renewal problems first, then the public ones. */
function problemKey(error: unknown): string {
  if (error instanceof ApiProblem) {
    if (error.type === 'renewal_expired' || error.status === 410)
      return `${RENEWAL_NS}:problem.renewal_expired`;
    if (error.type === 'credit_insufficient') return `${RENEWAL_NS}:problem.credit_insufficient`;
  }
  return problemMessageKey(error);
}

/**
 * Renewal proposal of a season pass (spec 10.4), on the manage page of the
 * held booking that protects the spot until the deadline: price compared
 * with last season, countdown, accept (deposit or full payment, optional
 * credit, current terms) or decline. Confirmation only ever comes from the
 * payment webhook or the desk (INV-19): the page just follows the redirect.
 */
export default function RenewalSection({
  slug,
  token,
  booking,
  win,
  now = serverNow,
}: RenewalSectionProps) {
  useRenewalMessages();
  const { t, i18n } = useTranslation(RENEWAL_NS);
  const { t: tc } = useTranslation();
  const env = useBookingEnv();
  const r = booking.renewal;
  const currency = booking.summary.currency;
  const money = (c: number) => formatMoney(c, currency, i18n.language);
  const id = useId();
  const ticked = useTicker(now, 30_000);
  const left = timeLeft(r.deadline_at, ticked);
  const accept = useAcceptRenewal(slug, token);
  const decline = useDeclineRenewal(slug, token);
  const refresh = useRefreshManage(slug, token);
  const [payment, setPayment] = useState<'deposit' | 'full'>(
    r.allowed.pay_deposit && r.deposit_due > 0 ? 'deposit' : 'full',
  );
  const [useCredit, setUseCredit] = useState(r.allowed.use_credit && (r.credit_available ?? 0) > 0);
  const [terms, setTerms] = useState(false);
  const [termsError, setTermsError] = useState(false);
  const [declining, setDeclining] = useState(false);
  const [reason, setReason] = useState('');
  const [acceptKey, setAcceptKey] = useState(newIdempotencyKey);
  const [done, setDone] = useState<'accepted' | null>(null);
  const terms_ = env.establishment.online?.terms;
  const termsTexts =
    terms_ && terms_.version === r.terms_version
      ? (terms_.texts[i18n.language] ??
        terms_.texts[env.establishment.default_locale] ??
        Object.values(terms_.texts)[0])
      : undefined;

  const deadline = formatInstant(r.deadline_at, {
    locale: intlLocale(i18n.language),
    timeZone: env.establishment.timezone,
  });
  const credit = r.credit_available ?? 0;
  const due = payment === 'deposit' ? r.deposit_due : r.total_amount - r.paid_amount;
  const toPay = Math.max(0, due - (useCredit ? credit : 0));

  const submit = () => {
    if (r.terms_version && !terms) {
      setTermsError(true);
      return;
    }
    accept.mutate(
      {
        body: {
          payment,
          use_credit: useCredit || undefined,
          accept_terms_version: r.terms_version ?? '',
        },
        idempotencyKey: acceptKey,
      },
      {
        onSuccess: (res) => {
          if (res.pay.mode !== 'none' && res.pay.redirect_url) {
            redirectToPayment(res.pay.redirect_url, win, env.channel);
            return;
          }
          setDone('accepted');
          void refresh();
        },
        onError: (err) => {
          // A different attempt after a failure gets a new key.
          setAcceptKey(newIdempotencyKey());
          if (err instanceof ApiProblem && err.status === 410) void refresh();
        },
      },
    );
  };

  const header = (
    <CardHeader className="gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <RefreshCwIcon aria-hidden="true" className="size-5 text-primary-on-surface" />
        <CardTitle>
          <h2 className="text-lg">{t('title', { year: r.season_year })}</h2>
        </CardTitle>
        <Badge variant={r.status === 'converted' ? 'default' : 'secondary'}>
          {t(`status.${r.status}`)}
        </Badge>
      </div>
      <CardDescription>
        {[r.spot_label, r.zone_name, r.pack_name].filter(Boolean).join(' · ')} ·{' '}
        {formatStay(r.starts_on, r.ends_on)}
        {r.slots !== 'FULL' && ` · ${t(`slots.${r.slots}`)}`}
      </CardDescription>
    </CardHeader>
  );

  const prices = (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
      <div className="min-w-0">
        <dt className="text-muted-foreground">{t('price')}</dt>
        <dd className="text-lg font-semibold tabular-nums">{money(r.total_amount)}</dd>
      </div>
      <div className="min-w-0">
        <dt className="text-muted-foreground">{t('previous')}</dt>
        <dd className="tabular-nums">{money(r.previous_amount)}</dd>
      </div>
      {r.paid_amount > 0 && (
        <div className="min-w-0">
          <dt className="text-muted-foreground">{t('paid')}</dt>
          <dd className="tabular-nums">{money(r.paid_amount)}</dd>
        </div>
      )}
    </dl>
  );

  if (r.status !== 'proposed' || done) {
    const status = done ?? r.status;
    const tone =
      status === 'converted' || status === 'accepted'
        ? 'success'
        : status === 'declined'
          ? 'default'
          : 'warning';
    return (
      <Card>
        {header}
        <CardContent className="space-y-4">
          {prices}
          <Alert variant={tone} role="status">
            {status === 'converted' || status === 'accepted' ? (
              <CheckCircle2Icon aria-hidden="true" />
            ) : (
              <CircleAlertIcon aria-hidden="true" />
            )}
            <AlertDescription>{t(`outcome.${status}`)}</AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      {header}
      <CardContent className="space-y-5">
        {prices}
        <p className={left.expired ? 'text-sm text-destructive' : 'text-sm'}>
          {left.expired ? t('deadlinePassed') : t('deadline', { date: deadline })}{' '}
          {!left.expired && (
            <span className="font-medium">
              {left.days > 0
                ? t('left.days', { count: left.days })
                : t('left.hours', { hours: left.hours, minutes: left.minutes })}
            </span>
          )}
        </p>

        {r.allowed.accept && !left.expired && !declining && (
          <div className="space-y-4">
            {r.allowed.pay_deposit && r.allowed.pay_full && r.deposit_due > 0 && (
              <fieldset className="space-y-2">
                <legend className="text-sm font-medium">{t('payment.legend')}</legend>
                <RadioGroup
                  value={payment}
                  onValueChange={(v) => setPayment(v as 'deposit' | 'full')}
                  className="gap-2"
                >
                  <div className="flex items-center gap-3">
                    <RadioGroupItem id={`${id}-deposit`} value="deposit" />
                    <Label htmlFor={`${id}-deposit`}>
                      {t('payment.deposit', { amount: money(r.deposit_due) })}
                    </Label>
                  </div>
                  <div className="flex items-center gap-3">
                    <RadioGroupItem id={`${id}-full`} value="full" />
                    <Label htmlFor={`${id}-full`}>
                      {t('payment.full', { amount: money(r.total_amount - r.paid_amount) })}
                    </Label>
                  </div>
                </RadioGroup>
              </fieldset>
            )}
            {r.allowed.use_credit && credit > 0 && (
              <div className="flex items-start gap-3">
                <Checkbox
                  id={`${id}-credit`}
                  checked={useCredit}
                  onCheckedChange={(v) => setUseCredit(v === true)}
                  className="size-5"
                />
                <Label htmlFor={`${id}-credit`} className="leading-snug">
                  {t('useCredit', { amount: money(credit) })}
                </Label>
              </div>
            )}
            {r.terms_version && (
              <div className="space-y-2 rounded-xl border bg-muted/30 p-4">
                {termsTexts && (
                  <Collapsible>
                    <CollapsibleTrigger asChild>
                      <Button type="button" variant="ghost" className="-ml-2 h-11 px-2">
                        <ChevronDownIcon aria-hidden="true" />
                        {t('showTerms')}
                      </Button>
                    </CollapsibleTrigger>
                    <CollapsibleContent>
                      <div
                        tabIndex={0}
                        aria-label={t('termsVersion', { version: r.terms_version })}
                        className="mt-2 max-h-48 overflow-y-auto rounded-md border bg-background p-3 text-sm whitespace-pre-line focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
                      >
                        {termsTexts.terms}
                      </div>
                    </CollapsibleContent>
                  </Collapsible>
                )}
                <div className="flex items-start gap-3">
                  <Checkbox
                    id={`${id}-terms`}
                    checked={terms}
                    onCheckedChange={(v) => {
                      setTerms(v === true);
                      setTermsError(false);
                    }}
                    className="size-5"
                    aria-invalid={termsError || undefined}
                    aria-describedby={termsError ? `${id}-terms-error` : undefined}
                  />
                  <Label htmlFor={`${id}-terms`} className="leading-snug">
                    {t('acceptTerms', { version: r.terms_version })}
                  </Label>
                </div>
                {termsError && (
                  <p id={`${id}-terms-error`} className="text-sm text-destructive">
                    {t('termsRequired')}
                  </p>
                )}
              </div>
            )}
            <div className="flex flex-col gap-2 sm:flex-row">
              <Button className="h-11" disabled={accept.isPending} onClick={submit}>
                {accept.isPending ? <Spinner /> : <CheckCircle2Icon aria-hidden="true" />}
                {toPay > 0 ? t('acceptPay', { amount: money(toPay) }) : t('accept')}
              </Button>
              {r.allowed.decline && (
                <Button variant="outline" className="h-11" onClick={() => setDeclining(true)}>
                  <XCircleIcon aria-hidden="true" />
                  {t('decline')}
                </Button>
              )}
            </div>
            {accept.error && (
              <p role="alert" className="text-sm text-destructive">
                {tc(problemKey(accept.error))}
              </p>
            )}
          </div>
        )}

        {declining && (
          <div className="space-y-3 rounded-xl border p-4">
            <p className="font-medium">{t('declineQuestion', { spot: r.spot_label })}</p>
            <p className="text-sm text-muted-foreground">{t('declineHint')}</p>
            <div className="space-y-2">
              <Label htmlFor={`${id}-reason`}>{t('declineReason')}</Label>
              <Textarea
                id={`${id}-reason`}
                rows={2}
                maxLength={500}
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Button
                variant="destructive"
                className="h-11"
                disabled={decline.isPending}
                onClick={() =>
                  decline.mutate(
                    { reason: reason.trim() || undefined },
                    { onSuccess: () => setDeclining(false) },
                  )
                }
              >
                {decline.isPending && <Spinner />}
                {t('declineConfirm')}
              </Button>
              <Button variant="outline" className="h-11" onClick={() => setDeclining(false)}>
                {t('declineBack')}
              </Button>
            </div>
            {decline.error && (
              <p role="alert" className="text-sm text-destructive">
                {tc(problemKey(decline.error))}
              </p>
            )}
          </div>
        )}
        {left.expired && (
          <Alert variant="warning" role="status">
            <CircleAlertIcon aria-hidden="true" />
            <AlertTitle>{t('outcome.expired')}</AlertTitle>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
}
