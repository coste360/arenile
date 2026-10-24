import { zodResolver } from '@hookform/resolvers/zod';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@arenile/shared/ui/collapsible';
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ChevronDownIcon, CreditCardIcon, ExternalLinkIcon, LockIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { Controller, useForm, type FieldError as RhfFieldError } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  newIdempotencyKey,
  usePayCheckout,
  useUpdateCheckout,
  type CheckoutState,
  type PublicQuote,
} from '../api/public';
import { useBookingEnv } from '../flow/context';
import { formatMoney } from '../flow/format';
import { redirectToPayment } from '../flow/navigate';
import { isHoldExpired, problemMessageKey } from '../flow/problems';
import { PUBLIC_NS } from '../i18n';
import { HoldCountdown } from './who/HoldCountdown';
import { customerSchema, type CustomerValues } from './who/schema';

export interface WhoStepProps {
  session: { bookingId: string; token: string };
  /** Latest checkout state (hold expiry, prefilled customer, quote). */
  state: CheckoutState | undefined;
  /** RFC 3339 expiry of the hold (countdown). */
  holdExpiresAt?: string;
  /** Quote of the held booking (amount on the pay button). */
  quote: PublicQuote | undefined;
  /** Confirmed or sent for approval without an online payment (mode none). */
  onCompleted: () => void;
  /** Hold expired (countdown or a 410 from the server). */
  onExpired: () => void;
}

const PROVIDER_NAMES: Record<string, string> = { stripe: 'Stripe' };

/**
 * "Chi e pagamento" (spec 12.3 passo 4, 10.1 passi 4-5): name, surname,
 * email and phone (no account), the current versioned terms to accept and
 * the separate optional marketing consent (spec 23), the visible hold
 * countdown, then pay or confirm. Payment opens at top level (Stripe
 * Checkout refuses frames); the return URL never confirms (INV-19).
 */
export default function WhoStep({
  session,
  state,
  holdExpiresAt,
  quote,
  onCompleted,
  onExpired,
}: WhoStepProps) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const env = useBookingEnv();
  const { slug, online, establishment, win, channel } = env;
  const id = useId();
  const update = useUpdateCheckout(slug, session);
  const pay = usePayCheckout(slug, session);
  const [error, setError] = useState<string | null>(null);
  const [manualUrl, setManualUrl] = useState<string | null>(null);
  const terms = online.terms;
  const lang = i18n.language;
  const texts = terms
    ? (terms.texts[lang] ??
      terms.texts[establishment.default_locale] ??
      Object.values(terms.texts)[0])
    : undefined;

  const customer = state?.customer;
  const form = useForm<CustomerValues>({
    resolver: zodResolver(customerSchema({ hasTerms: !!terms })),
    defaultValues: {
      first_name: customer?.first_name ?? '',
      last_name: customer?.last_name ?? '',
      email: customer?.email ?? '',
      phone: customer?.phone ?? '',
      accept_terms: !!terms && customer?.accepted_terms_version === terms.version,
      marketing_consent: customer?.marketing_consent ?? false,
    },
  });
  const { register, control, formState } = form;
  const errors = formState.errors;
  const err = (e: RhfFieldError | undefined) => (e?.message ? t(e.message) : undefined);

  const q = quote ?? state?.quote;
  const dueNow = q?.due_now ?? 0;
  const plan = q?.payment_plan;
  const paysOnline =
    online.payment.online && dueNow > 0 && plan !== 'at_desk' && plan !== 'approval';
  const submitLabel = paysOnline
    ? t('who.pay', { amount: formatMoney(dueNow, q?.currency ?? establishment.currency, lang) })
    : plan === 'approval'
      ? t('who.request')
      : t('who.confirm');

  const onSubmit = form.handleSubmit(async (v) => {
    setError(null);
    setManualUrl(null);
    try {
      await update.mutateAsync({
        first_name: v.first_name,
        last_name: v.last_name,
        email: v.email,
        ...(v.phone ? { phone: v.phone } : {}),
        locale: lang,
        accept_terms_version: terms?.version ?? '',
        marketing_consent: v.marketing_consent,
      });
      const result = await pay.mutateAsync({
        idempotencyKey: newIdempotencyKey(),
        ...(env.location.hostPage ? { body: { return_to: env.location.hostPage } } : {}),
      });
      if (result.mode === 'none') {
        onCompleted();
        return;
      }
      if (!result.redirect_url) throw new Error('missing redirect_url');
      const outcome = redirectToPayment(result.redirect_url, win, channel);
      if (outcome === 'manual' || outcome === 'parent') setManualUrl(result.redirect_url);
    } catch (e) {
      if (isHoldExpired(e)) {
        onExpired();
        return;
      }
      setError(problemMessageKey(e));
    }
  });

  const busy = formState.isSubmitting;
  const redirecting = pay.isSuccess && pay.data.mode === 'redirect';

  return (
    <form noValidate onSubmit={onSubmit} className="space-y-6">
      {holdExpiresAt && <HoldCountdown expiresAt={holdExpiresAt} onExpire={onExpired} />}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field data-invalid={!!errors.first_name || undefined}>
          <FieldLabel htmlFor={`${id}-first`}>{t('who.firstName')}</FieldLabel>
          <Input
            id={`${id}-first`}
            autoComplete="given-name"
            className="h-11"
            aria-invalid={!!errors.first_name || undefined}
            aria-describedby={errors.first_name ? `${id}-first-error` : undefined}
            {...register('first_name')}
          />
          <FieldError id={`${id}-first-error`}>{err(errors.first_name)}</FieldError>
        </Field>
        <Field data-invalid={!!errors.last_name || undefined}>
          <FieldLabel htmlFor={`${id}-last`}>{t('who.lastName')}</FieldLabel>
          <Input
            id={`${id}-last`}
            autoComplete="family-name"
            className="h-11"
            aria-invalid={!!errors.last_name || undefined}
            aria-describedby={errors.last_name ? `${id}-last-error` : undefined}
            {...register('last_name')}
          />
          <FieldError id={`${id}-last-error`}>{err(errors.last_name)}</FieldError>
        </Field>
        <Field data-invalid={!!errors.email || undefined}>
          <FieldLabel htmlFor={`${id}-email`}>{t('who.email')}</FieldLabel>
          <Input
            id={`${id}-email`}
            type="email"
            inputMode="email"
            autoComplete="email"
            className="h-11"
            aria-invalid={!!errors.email || undefined}
            aria-describedby={`${id}-email-hint${errors.email ? ` ${id}-email-error` : ''}`}
            {...register('email')}
          />
          <FieldDescription id={`${id}-email-hint`}>{t('who.emailHint')}</FieldDescription>
          <FieldError id={`${id}-email-error`}>{err(errors.email)}</FieldError>
        </Field>
        <Field data-invalid={!!errors.phone || undefined}>
          <FieldLabel htmlFor={`${id}-phone`}>{t('who.phoneOptional')}</FieldLabel>
          <Input
            id={`${id}-phone`}
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            className="h-11"
            aria-invalid={!!errors.phone || undefined}
            aria-describedby={errors.phone ? `${id}-phone-error` : undefined}
            {...register('phone')}
          />
          <FieldError id={`${id}-phone-error`}>{err(errors.phone)}</FieldError>
        </Field>
      </div>

      {terms && texts && (
        <div className="space-y-3 rounded-xl border bg-muted/30 p-4">
          <Collapsible>
            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
              <CollapsibleTrigger asChild>
                <Button type="button" variant="ghost" className="-ml-2 h-11 px-2">
                  <ChevronDownIcon aria-hidden="true" />
                  {t('who.showTerms')}
                </Button>
              </CollapsibleTrigger>
              <span className="min-w-0 break-words text-xs text-muted-foreground">
                {t('who.termsVersion', { version: terms.version })}
              </span>
            </div>
            <CollapsibleContent>
              <div
                tabIndex={0}
                aria-label={t('who.termsVersion', { version: terms.version })}
                className="mt-2 max-h-48 overflow-y-auto rounded-md border bg-background p-3 text-sm whitespace-pre-line focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
              >
                {texts.terms}
                {texts.privacy && (
                  <>
                    <p className="mt-3 font-medium">{t('who.privacy')}</p>
                    {texts.privacy}
                  </>
                )}
              </div>
            </CollapsibleContent>
          </Collapsible>
          <Controller
            control={control}
            name="accept_terms"
            render={({ field }) => (
              <Field orientation="horizontal" data-invalid={!!errors.accept_terms || undefined}>
                <Checkbox
                  id={`${id}-terms`}
                  ref={field.ref}
                  checked={field.value}
                  onCheckedChange={(v) => field.onChange(v === true)}
                  onBlur={field.onBlur}
                  className="size-5"
                  aria-invalid={!!errors.accept_terms || undefined}
                  aria-describedby={errors.accept_terms ? `${id}-terms-error` : undefined}
                />
                <FieldContent>
                  <FieldLabel htmlFor={`${id}-terms`}>{t('who.terms')}</FieldLabel>
                  <FieldError id={`${id}-terms-error`}>{err(errors.accept_terms)}</FieldError>
                </FieldContent>
              </Field>
            )}
          />
          <Controller
            control={control}
            name="marketing_consent"
            render={({ field }) => (
              <Field orientation="horizontal">
                <Checkbox
                  id={`${id}-marketing`}
                  ref={field.ref}
                  checked={field.value}
                  onCheckedChange={(v) => field.onChange(v === true)}
                  onBlur={field.onBlur}
                  className="size-5"
                />
                <FieldContent>
                  <FieldLabel htmlFor={`${id}-marketing`}>{t('who.marketing')}</FieldLabel>
                  {texts.marketing && <FieldDescription>{texts.marketing}</FieldDescription>}
                </FieldContent>
              </Field>
            )}
          />
        </div>
      )}

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{tc(error)}</AlertDescription>
        </Alert>
      )}

      {manualUrl ? (
        <Alert role="status">
          <CreditCardIcon aria-hidden="true" />
          <AlertDescription className="space-y-3">
            <p>{t('who.paymentBlocked')}</p>
            <div className="flex flex-wrap gap-2">
              <Button asChild className="h-11">
                <a href={manualUrl} target="_top" rel="noopener">
                  <LockIcon aria-hidden="true" />
                  {t('who.openPayment')}
                </a>
              </Button>
              <Button asChild variant="outline" className="h-11">
                <a href={manualUrl} target="_blank" rel="noopener noreferrer">
                  <ExternalLinkIcon aria-hidden="true" />
                  {t('who.openPaymentNewTab')}
                </a>
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : redirecting ? (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner />
          {t('who.redirecting')}
        </p>
      ) : null}

      <div className="space-y-2">
        <Button
          type="submit"
          size="lg"
          className="h-12 w-full sm:w-auto"
          disabled={busy || redirecting}
        >
          {busy ? <Spinner /> : paysOnline ? <LockIcon aria-hidden="true" /> : null}
          {submitLabel}
        </Button>
        <p className="text-xs text-muted-foreground">
          {paysOnline
            ? t('who.securePayment', {
                provider: PROVIDER_NAMES[online.payment.provider] ?? online.payment.provider,
              })
            : plan === 'approval'
              ? t('who.approval')
              : t('who.atDesk')}
          {online.payment.test_mode && <> {t('who.testMode')}</>}
        </p>
      </div>
    </form>
  );
}
