import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Spinner } from '@arenile/shared/ui/spinner';
import { cn } from '@arenile/shared/lib/utils';
import { useQueryClient } from '@tanstack/react-query';
import { CheckIcon, ChevronLeftIcon, ChevronRightIcon, EyeIcon, InfoIcon } from 'lucide-react';
import {
  Suspense,
  lazy,
  useCallback,
  useEffect,
  useId,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import {
  newIdempotencyKey,
  publicKeys,
  useAbandonCheckout,
  useCheckoutState,
  useCreateBooking,
  useQuote,
} from '../api/public';
import { checkoutStore, type StoredCheckout } from '../checkout/session';
import { useBookingEnv } from '../flow/context';
import { isResourceProblem, problemMessageKey } from '../flow/problems';
import {
  flowReducer,
  initialFlowState,
  isWhatComplete,
  isWhenComplete,
  isWhereComplete,
  toSelection,
  type FlowState,
} from '../flow/selection';
import { useDebouncedValue } from '../flow/useDebouncedValue';
import { PUBLIC_NS } from '../i18n';
import { PriceBar } from '../summary/PriceBar';
import { PriceSummary } from '../summary/PriceSummary';
import { STEPS, type Step } from './steps';
import { WhenStep } from './WhenStep';
import { serverNow } from '../flow/serverClock';

// Later steps are separate chunks: the first screen only carries "Quando" (spec 12.6).
const WhereStep = lazy(() => import('./WhereStep'));
const WhatStep = lazy(() => import('./WhatStep'));
const WhoStep = lazy(() => import('./WhoStep'));
const ConfirmationPage = lazy(() => import('../confirmation/ConfirmationPage'));

/** Delay of the price quote after the last change (spec 12.3 "con debounce"). */
export const QUOTE_DEBOUNCE_MS = 400;

export interface StepperProps {
  /** Title block above the steps. */
  header?: ReactNode;
  /** Extra cards of the side column (contacts). */
  aside?: ReactNode;
  /**
   * Booking id of `?checkout=<id>` (return from the payment page): opens the
   * outcome with this tab's session when it matches (else the generic
   * "email will follow" message).
   */
  returnedBookingId?: string | null;
  /** Current time in ms (tests). */
  now?: () => number;
}

function resumable(slug: string, now: number): StoredCheckout | null {
  const stored = checkoutStore.load(slug);
  if (!stored?.holdExpiresAt) return null;
  return Date.parse(stored.holdExpiresAt) > now ? stored : null;
}

function canOpen(step: Step, flow: FlowState, session: StoredCheckout | null): boolean {
  switch (step) {
    case 'when':
      return true;
    case 'where':
      return isWhenComplete(flow);
    case 'what':
      return isWhereComplete(flow);
    case 'who':
      return !!session;
  }
}

function StepFallback() {
  const { t } = useTranslation();
  return (
    <div role="status" className="space-y-3">
      <span className="sr-only">{t('common.loading')}</span>
      <Skeleton aria-hidden="true" className="h-8 w-1/2" />
      <Skeleton aria-hidden="true" className="h-40 w-full" />
    </div>
  );
}

/**
 * Imperative shell of the public booking flow (spec 12.3, 10.1): the four
 * steps around the pure flow reducer, the debounced `quote` behind the
 * always-visible summary, the hold (`createPublicBooking`) when leaving
 * "Cosa", its release when going back, its expiry, and the outcome page.
 * Focus moves to the step title on every step change.
 */
export function Stepper({ header, aside, returnedBookingId, now = serverNow }: StepperProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const env = useBookingEnv();
  const { slug, location, establishment, preview } = env;
  const qc = useQueryClient();
  const baseId = useId();

  const [initialSession] = useState<StoredCheckout | null>(() => {
    if (preview) return null;
    if (returnedBookingId) {
      const stored = checkoutStore.load(slug);
      return stored?.bookingId === returnedBookingId ? stored : null;
    }
    return resumable(slug, now());
  });
  const [flow, dispatch] = useReducer(
    flowReducer,
    location.from ? { from: location.from, to: location.to ?? undefined } : undefined,
    initialFlowState,
  );
  const [step, setStep] = useState<Step>(initialSession ? 'who' : 'when');
  const [session, setSession] = useState<StoredCheckout | null>(initialSession);
  const [phase, setPhase] = useState<'flow' | 'outcome'>(returnedBookingId ? 'outcome' : 'flow');
  const [notice, setNotice] = useState<string | null>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const moved = useRef(false);
  const createKey = useRef<{ selection: string; key: string } | null>(null);

  const selection = toSelection(flow);
  const debounced = useDebouncedValue(selection, QUOTE_DEBOUNCE_MS);
  const quote = useQuote(slug, session ? null : debounced);
  const checkout = useCheckoutState(slug, session);
  const create = useCreateBooking(slug);
  const abandon = useAbandonCheckout(slug);

  const currentQuote = session ? (checkout.data?.quote ?? create.data?.quote) : quote.data;
  const updating =
    !session && (JSON.stringify(selection) !== JSON.stringify(debounced) || quote.isFetching);
  const shownQuote = !session && !selection ? undefined : currentQuote;

  useEffect(() => {
    if (!moved.current) return;
    headingRef.current?.focus();
  }, [step, phase]);

  const go = (next: Step) => {
    moved.current = true;
    setStep(next);
  };

  const refreshAvailability = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ['public', slug, 'availability'] });
    void qc.invalidateQueries({ queryKey: ['public', slug, 'calendar'] });
  }, [qc, slug]);

  /** Leaves the held booking (back navigation): releases the hold server side. */
  const releaseHold = () => {
    if (!session) return;
    abandon.mutate({ bookingId: session.bookingId, token: session.token });
    checkoutStore.clear(slug);
    setSession(null);
    refreshAvailability();
  };

  const onExpired = useCallback(() => {
    checkoutStore.clear(slug);
    setSession(null);
    dispatch({ type: 'resetResources' });
    setNotice('public:errors.hold_expired');
    refreshAvailability();
    moved.current = true;
    setStep(isWhenComplete(flow) ? 'where' : 'when');
  }, [slug, refreshAvailability, flow]);

  const reserve = async () => {
    if (!selection || preview) return;
    setNotice(null);
    const key = JSON.stringify(selection);
    if (createKey.current?.selection !== key) {
      createKey.current = { selection: key, key: newIdempotencyKey() };
    }
    try {
      const s = await create.mutateAsync({
        body: { ...selection, locale: env.win.document.documentElement.lang || undefined },
        idempotencyKey: createKey.current.key,
      });
      createKey.current = null;
      setSession({
        bookingId: s.booking_id,
        token: s.checkout_token,
        holdExpiresAt: s.hold_expires_at,
      });
      go('who');
    } catch (e) {
      setNotice(problemMessageKey(e));
      if (isResourceProblem(e)) {
        refreshAvailability();
        go('where');
      }
    }
  };

  const restart = () => {
    checkoutStore.clear(slug);
    if (returnedBookingId) {
      const url = new URL(env.win.location.href);
      url.searchParams.delete('checkout');
      env.win.history.replaceState(null, '', `${url.pathname}${url.search}`);
    }
    setSession(null);
    setPhase('flow');
    dispatch({ type: 'dates', dates: undefined, formula: null });
    setNotice(null);
    go('when');
  };

  const index = STEPS.indexOf(step);
  const next = STEPS[index + 1];
  const canNext =
    step === 'when'
      ? isWhenComplete(flow)
      : step === 'where'
        ? isWhereComplete(flow)
        : step === 'what'
          ? isWhatComplete(flow) && !preview && currentQuote?.available !== false && !updating
          : false;

  const openStep = (target: Step) => {
    if (target === step) return;
    // Leaving "Chi" releases the hold; after a reload the earlier steps may
    // be empty, so fall back to the furthest step that can be opened.
    const leavingHold = step === 'who' && target !== 'who';
    let to: Step | undefined = target;
    if (!canOpen(target, flow, session) || leavingHold) {
      to = [...STEPS]
        .slice(0, STEPS.indexOf(target) + 1)
        .reverse()
        .find((s) => s !== 'who' && canOpen(s, flow, null));
      if (!leavingHold && to !== target) return;
    }
    if (!to) return;
    if (leavingHold) releaseHold();
    setNotice(null);
    go(to);
  };

  const summary = (
    <PriceSummary
      quote={shownQuote}
      updating={updating}
      currency={establishment.currency}
      fallbackLocale={establishment.default_locale}
      dates={
        flow.dates ??
        (checkout.data
          ? { from: checkout.data.summary.starts_on, to: checkout.data.summary.ends_on }
          : undefined)
      }
    />
  );

  const main =
    phase === 'outcome' ? (
      <Suspense fallback={<StepFallback />}>
        <ConfirmationPage session={session} onRestart={restart} />
      </Suspense>
    ) : (
      <section aria-label={tc('booking.steps.label')} className="space-y-4">
        {preview && (
          <Alert role="note">
            <EyeIcon aria-hidden="true" />
            <AlertDescription className="flex flex-wrap items-center gap-2">
              <Badge className="bg-highlight text-highlight-foreground">
                {t('flow.previewBadge')}
              </Badge>
              {t('flow.previewNotice')}
            </AlertDescription>
          </Alert>
        )}
        <ol className="grid grid-cols-4 gap-1.5 sm:gap-2">
          {STEPS.map((s, i) => {
            const active = s === step;
            const done = i < index;
            const available = canOpen(s, flow, session);
            return (
              <li key={s}>
                <Button
                  variant="ghost"
                  onClick={() => openStep(s)}
                  aria-current={active ? 'step' : undefined}
                  aria-controls={`${baseId}-panel`}
                  aria-disabled={available ? undefined : true}
                  className={cn(
                    'h-auto min-h-11 w-full flex-col gap-1.5 rounded-lg px-1 py-2 text-xs font-medium whitespace-normal sm:text-sm',
                    active
                      ? 'bg-primary/10 text-primary-on-surface hover:bg-primary/15'
                      : 'text-muted-foreground',
                    !available && 'cursor-default opacity-60 hover:bg-transparent',
                  )}
                >
                  <span
                    aria-hidden="true"
                    className={cn(
                      'flex size-7 items-center justify-center rounded-full border text-sm font-semibold',
                      active && 'border-primary bg-primary text-primary-foreground',
                      done && 'border-primary text-primary-on-surface',
                    )}
                  >
                    {done ? <CheckIcon className="size-4" /> : i + 1}
                  </span>
                  <span>
                    {tc(`booking.steps.${s}`)}
                    {!available && <span className="sr-only"> ({t('flow.stepUnavailable')})</span>}
                  </span>
                </Button>
              </li>
            );
          })}
        </ol>
        <Card id={`${baseId}-panel`} role="region" aria-labelledby={`${baseId}-title`}>
          <CardHeader>
            <CardTitle>
              <h2
                id={`${baseId}-title`}
                ref={headingRef}
                tabIndex={-1}
                className="text-lg outline-none"
              >
                {index + 1}. {tc(`booking.steps.${step}`)}
              </h2>
            </CardTitle>
            <CardDescription>{tc(`booking.stepText.${step}`)}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            {notice && (
              <Alert variant="destructive">
                <InfoIcon aria-hidden="true" />
                <AlertDescription>{tc(notice)}</AlertDescription>
              </Alert>
            )}
            <Suspense fallback={<StepFallback />}>
              {step === 'when' && <WhenStep state={flow} dispatch={dispatch} />}
              {step === 'where' && <WhereStep state={flow} dispatch={dispatch} />}
              {step === 'what' && (
                <WhatStep state={flow} dispatch={dispatch} promoStatus={quote.data?.promo} />
              )}
              {step === 'who' && session && (
                <WhoStep
                  session={session}
                  state={checkout.data}
                  holdExpiresAt={checkout.data?.hold_expires_at ?? session.holdExpiresAt}
                  quote={currentQuote}
                  onExpired={onExpired}
                  onCompleted={() => {
                    // The outcome page polls the server state from scratch.
                    qc.removeQueries({ queryKey: publicKeys.checkout(slug, session.bookingId) });
                    moved.current = true;
                    setPhase('outcome');
                  }}
                />
              )}
            </Suspense>
          </CardContent>
          <CardFooter className="flex-wrap items-center justify-between gap-3 border-t">
            <div className="lg:hidden">
              <PriceBar quote={shownQuote} updating={updating} currency={establishment.currency} />
            </div>
            <div className="ml-auto flex gap-2">
              <Button
                variant="outline"
                className="h-11"
                disabled={index === 0}
                onClick={() => openStep(STEPS[index - 1] ?? 'when')}
              >
                <ChevronLeftIcon aria-hidden="true" />
                {tc('common.back')}
              </Button>
              {step !== 'who' && (
                <Button
                  className="h-11"
                  disabled={!canNext || create.isPending}
                  onClick={() => {
                    if (step === 'what') void reserve();
                    else if (next) {
                      setNotice(null);
                      go(next);
                    }
                  }}
                >
                  {create.isPending ? <Spinner /> : null}
                  {step === 'what'
                    ? create.isPending
                      ? t('flow.reserving')
                      : t('flow.continue')
                    : tc('common.next')}
                  {!create.isPending && <ChevronRightIcon aria-hidden="true" />}
                </Button>
              )}
            </div>
          </CardFooter>
        </Card>
      </section>
    );

  return (
    <div className="mx-auto grid w-full max-w-5xl flex-1 gap-6 px-4 py-6 sm:py-8 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start">
      <div className="min-w-0 space-y-6">
        {header}
        {main}
      </div>
      <aside className="space-y-4 lg:sticky lg:top-6">
        {phase === 'flow' && summary}
        {aside}
      </aside>
    </div>
  );
}
