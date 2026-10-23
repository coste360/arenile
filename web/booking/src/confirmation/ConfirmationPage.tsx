import { confirmationPhase, type ConfirmationPhase } from './phase';
import { useApiClient } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Separator } from '@arenile/shared/ui/separator';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  CalendarPlusIcon,
  CircleCheckIcon,
  CircleXIcon,
  ClockIcon,
  MailIcon,
  RotateCcwIcon,
  type LucideIcon,
} from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  fetchCheckoutDocument,
  newIdempotencyKey,
  useCheckoutState,
  usePayCheckout,
} from '../api/public';
import { useBookingEnv } from '../flow/context';
import { redirectToPayment } from '../flow/navigate';
import { isSessionLost, problemMessageKey } from '../flow/problems';
import { PUBLIC_NS } from '../i18n';
import { BookingDetails } from './BookingDetails';
import { saveBlob, svgDataUrl } from './documents';

export interface ConfirmationPageProps {
  /** Session of this tab; null after a top-level return from an embedded checkout. */
  session: { bookingId: string; token: string } | null;
  onRestart: () => void;
  /** Polling interval while the outcome is pending (ms). */
  pollMs?: number;
  /** After this long without an outcome, say the email will follow (ms). */
  slowAfterMs?: number;
}

const PHASE_ICON: Record<ConfirmationPhase, LucideIcon> = {
  loading: ClockIcon,
  unknown: MailIcon,
  waiting: ClockIcon,
  approval: MailIcon,
  failed: CircleXIcon,
  cancelled: CircleXIcon,
  done: CircleCheckIcon,
  expired: CircleXIcon,
  rejected: CircleXIcon,
};

const PHASE_TITLE: Record<ConfirmationPhase, string> = {
  loading: 'confirmation.waiting',
  unknown: 'confirmation.unknownTitle',
  waiting: 'confirmation.waiting',
  approval: 'confirmation.approvalTitle',
  failed: 'confirmation.failedTitle',
  cancelled: 'confirmation.cancelledTitle',
  done: 'confirmation.title',
  expired: 'confirmation.expiredTitle',
  rejected: 'confirmation.rejectedTitle',
};

/**
 * Outcome of the checkout (spec 12.3 "Conferma", 10.1 passo 7): polls the
 * state after the payment return until a final answer from the webhook or
 * the manual driver, then shows the code, the QR, the summary, "aggiungi al
 * calendario" (.ics) and where the manage link was sent; posts
 * `arenile:booked` (code only, no personal data) to the verified host page.
 */
export default function ConfirmationPage({
  session,
  onRestart,
  pollMs = 2000,
  slowAfterMs = 90_000,
}: ConfirmationPageProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const { slug, establishment, channel, win, location } = useBookingEnv();
  const client = useApiClient();
  const query = useCheckoutState(slug, session, pollMs);
  const pay = usePayCheckout(slug, session ?? { bookingId: '', token: '' });
  const [slow, setSlow] = useState(false);
  const [docError, setDocError] = useState<string | null>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const announced = useRef<string | null>(null);

  const lost = !session || isSessionLost(query.error);
  const phase: ConfirmationPhase = lost ? 'unknown' : confirmationPhase(query.data);
  const state = query.data;
  const confirmation = state?.confirmation;

  useEffect(() => {
    if (phase !== 'waiting' && phase !== 'loading') return;
    const id = setTimeout(() => setSlow(true), slowAfterMs);
    return () => clearTimeout(id);
  }, [phase, slowAfterMs]);

  // Move focus to the outcome heading so screen readers announce it.
  useEffect(() => {
    if (phase !== 'loading') headingRef.current?.focus();
  }, [phase]);

  useEffect(() => {
    if (phase === 'done' && confirmation?.code && announced.current !== confirmation.code) {
      announced.current = confirmation.code;
      channel?.post({ type: 'arenile:booked', code: confirmation.code });
    }
  }, [phase, confirmation?.code, channel]);

  const download = async () => {
    if (!session) return;
    setDocError(null);
    try {
      const blob = await fetchCheckoutDocument(client, slug, session, 'ics');
      saveBlob(blob, `${confirmation?.code ?? 'prenotazione'}.ics`, win.document);
    } catch (e) {
      setDocError(problemMessageKey(e));
    }
  };

  const retryPay = async () => {
    try {
      const result = await pay.mutateAsync({
        idempotencyKey: newIdempotencyKey(),
        ...(location.hostPage ? { body: { return_to: location.hostPage } } : {}),
      });
      if (result.mode === 'redirect' && result.redirect_url) {
        redirectToPayment(result.redirect_url, win, channel);
      } else {
        void query.refetch();
      }
    } catch {
      // shown through pay.error
    }
  };

  const Icon = PHASE_ICON[phase];
  const good = phase === 'done' || phase === 'approval';
  const bad =
    phase === 'failed' || phase === 'cancelled' || phase === 'expired' || phase === 'rejected';

  return (
    <Card aria-busy={phase === 'loading' || phase === 'waiting' || undefined}>
      <CardHeader className="items-center text-center">
        <span
          aria-hidden="true"
          className={
            good
              ? 'mx-auto flex size-14 items-center justify-center rounded-full bg-primary text-primary-foreground'
              : bad
                ? 'mx-auto flex size-14 items-center justify-center rounded-full bg-destructive/10 text-destructive'
                : 'mx-auto flex size-14 items-center justify-center rounded-full bg-muted text-muted-foreground'
          }
        >
          {phase === 'loading' || phase === 'waiting' ? (
            <Spinner className="size-6" />
          ) : (
            <Icon className="size-7" />
          )}
        </span>
        <CardTitle>
          <h2 ref={headingRef} tabIndex={-1} className="text-xl outline-none">
            {t(PHASE_TITLE[phase])}
          </h2>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <div role="status" aria-live="polite" className="text-center text-sm text-muted-foreground">
          {phase === 'waiting' && (
            <p>{slow ? t('confirmation.slow') : t('confirmation.waitingHint')}</p>
          )}
          {phase === 'approval' && <p>{t('confirmation.approval')}</p>}
          {phase === 'failed' && <p>{t('confirmation.failed')}</p>}
          {phase === 'cancelled' && <p>{t('confirmation.cancelled')}</p>}
          {phase === 'expired' && <p>{t('confirmation.expired')}</p>}
          {phase === 'rejected' && <p>{t('confirmation.rejected')}</p>}
          {phase === 'unknown' && <p>{t('confirmation.unknown')}</p>}
        </div>

        {phase === 'done' && confirmation && (
          <div className="space-y-5">
            <div className="flex flex-col items-center gap-3 rounded-xl border bg-muted/30 p-4 text-center">
              <p className="text-sm text-muted-foreground">{t('confirmation.code')}</p>
              <p
                className="font-mono text-2xl font-semibold tracking-widest"
                data-testid="booking-code"
              >
                {confirmation.code}
              </p>
              {confirmation.qr_svg && (
                <img
                  src={svgDataUrl(confirmation.qr_svg)}
                  alt={t('confirmation.qr')}
                  className="size-48 rounded-lg bg-white p-2"
                />
              )}
            </div>
            <div className="flex flex-wrap justify-center gap-2">
              <Button variant="outline" className="h-11" onClick={() => void download()}>
                <CalendarPlusIcon aria-hidden="true" />
                {t('confirmation.addToCalendar')}
              </Button>
            </div>
            {docError && (
              <p role="alert" className="text-center text-sm text-destructive">
                {tc(docError)}
              </p>
            )}
            <p className="flex items-start justify-center gap-2 text-center text-sm text-muted-foreground">
              <MailIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
              {confirmation.email_masked
                ? t('confirmation.emailSent', { email: confirmation.email_masked })
                : t('confirmation.emailSentGeneric')}
            </p>
          </div>
        )}

        {state && (phase === 'done' || phase === 'approval' || phase === 'waiting') && (
          <>
            <Separator />
            <section aria-label={t('confirmation.summary')}>
              <BookingDetails
                summary={state.summary}
                fallbackLocale={establishment.default_locale}
              />
            </section>
          </>
        )}

        {pay.error && (
          <Alert variant="destructive">
            <AlertDescription>{tc(problemMessageKey(pay.error))}</AlertDescription>
          </Alert>
        )}
      </CardContent>
      <CardFooter className="flex-wrap justify-center gap-2 border-t">
        {(phase === 'failed' || phase === 'cancelled') && (
          <Button className="h-11" onClick={() => void retryPay()} disabled={pay.isPending}>
            {pay.isPending && <Spinner />}
            {t('confirmation.retryPay')}
          </Button>
        )}
        {phase !== 'waiting' && phase !== 'loading' && (
          <Button variant="outline" className="h-11" onClick={onRestart}>
            <RotateCcwIcon aria-hidden="true" />
            {t('confirmation.newBooking')}
          </Button>
        )}
      </CardFooter>
    </Card>
  );
}
