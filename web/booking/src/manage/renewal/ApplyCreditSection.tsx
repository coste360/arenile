import { Button } from '@arenile/shared/ui/button';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CheckCircle2Icon, WalletIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { newIdempotencyKey, type ManageBooking } from '../../api/public';
import { formatMoney } from '../../flow/format';
import { problemMessageKey } from '../../flow/problems';
import { useApplyCredit } from './api';
import { RENEWAL_NS, useRenewalMessages } from './i18n';

/**
 * "Usa il credito" on the manage page (spec 13.4): the customer's credit
 * balance pays the balance due of this booking (up to the smaller of the
 * two). Only on verified links: the anonymous checkout never shows credit
 * balances (ADR 0017).
 */
export default function ApplyCreditSection({
  slug,
  token,
  booking,
  applied,
  onApplied,
}: {
  slug: string;
  token: string;
  booking: ManageBooking;
  /** Amount applied in this visit (owned by the page: the booking data changes). */
  applied: number | null;
  onApplied: (amount: number) => void;
}) {
  useRenewalMessages();
  const { t, i18n } = useTranslation(RENEWAL_NS);
  const { t: tc } = useTranslation();
  const apply = useApplyCredit(slug, token);
  const [key, setKey] = useState(newIdempotencyKey);
  const currency = booking.summary.currency;
  const money = (c: number) => formatMoney(c, currency, i18n.language);
  const credit = booking.credit_available ?? 0;
  const amount = Math.min(credit, booking.summary.balance_due);

  if (applied !== null) {
    return (
      <p role="status" className="flex items-center gap-2 text-sm">
        <CheckCircle2Icon aria-hidden="true" className="size-4 text-primary-on-surface" />
        {t('credit.applied', { amount: money(applied) })}
      </p>
    );
  }
  if (amount <= 0) return null;
  return (
    <div className="space-y-3 rounded-xl border bg-card p-4">
      <p className="flex items-start gap-2 text-sm">
        <WalletIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
        <span>{t('credit.available', { amount: money(credit) })}</span>
      </p>
      <Button
        variant="outline"
        className="h-11 w-full sm:w-auto"
        disabled={apply.isPending}
        onClick={() =>
          apply.mutate(
            { amount, idempotencyKey: key },
            {
              onSuccess: () => onApplied(amount),
              onError: () => setKey(newIdempotencyKey()),
            },
          )
        }
      >
        {apply.isPending ? <Spinner /> : <WalletIcon aria-hidden="true" />}
        {t('credit.apply', { amount: money(amount) })}
      </Button>
      {apply.error && (
        <p role="alert" className="text-sm text-destructive">
          {apply.error.type === 'credit_insufficient'
            ? t('problem.credit_insufficient')
            : tc(problemMessageKey(apply.error))}
        </p>
      )}
    </div>
  );
}
