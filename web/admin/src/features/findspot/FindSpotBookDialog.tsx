import { useProblemText } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import { FormDialog } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useNavigate } from '@tanstack/react-router';
import { CircleAlertIcon } from 'lucide-react';
import { useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { newIdempotencyKey } from '../configuration/lib/validation';
import type { Customer } from '../customers/api';
import { CustomerPicker } from '../customers/CustomerPicker';
import { useBookProposal, type FindSpotProposal, type FindSpotRequest } from './api';
import { proposalBooking, segmentsText } from './lib';

export interface FindSpotBookDialogProps {
  establishmentId: string;
  request: FindSpotRequest;
  proposal: FindSpotProposal;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** A half-day was taken meanwhile (409): the panel searches again. */
  onConflict: () => void;
  /** Default: open the new booking. */
  onBooked?: (bookingId: string) => void;
}

/**
 * "Prenota" of a Trova posto proposal (spec 30.6): pick the customer and
 * confirm; the booking is created with one allocation per segment in one
 * transaction. The total shown is sent as expected_total, so a price that
 * changed meanwhile is refused instead of charged silently. The
 * Idempotency-Key stays the same while customer and proposal do not change.
 */
export function FindSpotBookDialog({
  establishmentId,
  request,
  proposal,
  open,
  onOpenChange,
  onConflict,
  onBooked,
}: FindSpotBookDialogProps) {
  const { t } = useTranslation('findspot');
  const problemText = useProblemText();
  const navigate = useNavigate();
  const book = useBookProposal(establishmentId);
  const customerId = useId();
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [error, setError] = useState<string | null>(null);
  // One key per customer and proposal: retries of the same request book once.
  const keys = useRef(new Map<string, string>());
  const keyFor = (id: string) => {
    const k = `${id}:${proposal.id}`;
    let v = keys.current.get(k);
    if (!v) {
      v = newIdempotencyKey();
      keys.current.set(k, v);
    }
    return v;
  };

  const confirm = () => {
    if (!customer) return;
    setError(null);
    book.mutate(
      {
        idempotencyKey: keyFor(customer.id),
        body: {
          booking: proposalBooking(request, proposal, customer.id),
          proposal_id: proposal.id,
          ...(proposal.total !== undefined ? { expected_total: proposal.total } : {}),
        },
      },
      {
        onSuccess: (b) => {
          toast.success(t('book.done', { code: b.code }));
          if (onBooked) onBooked(b.id);
          else
            void navigate({
              to: '/e/$establishmentId/prenotazioni/$bookingId',
              params: { establishmentId, bookingId: b.id },
            });
        },
        onError: (e) => {
          if (e.status === 409 && e.type !== 'price_changed') {
            toast.error(t('book.taken'));
            onConflict();
            return;
          }
          setError(problemText(e));
        },
      },
    );
  };

  return (
    <FormDialog
      open={open}
      onOpenChange={(v) => !book.isPending && onOpenChange(v)}
      title={t('book.title', { rank: proposal.rank })}
      description={t('book.description', {
        from: formatIsoDate(request.starts_on),
        to: formatIsoDate(request.ends_on),
      })}
    >
      <div className="space-y-4">
        <ul className="space-y-1 text-sm" aria-label={t('book.segments')}>
          {proposal.lines.map((l, i) => (
            <li key={i}>{segmentsText(l.segments, t)}</li>
          ))}
        </ul>
        {proposal.total !== undefined && (
          <p className="text-sm">
            {t('book.total')}{' '}
            <span className="font-semibold tabular-nums">{formatCents(proposal.total)}</span>
          </p>
        )}
        <div className="space-y-1.5">
          <Label htmlFor={customerId}>{t('book.customer')}</Label>
          <CustomerPicker
            id={customerId}
            establishmentId={establishmentId}
            value={customer}
            onChange={setCustomer}
          />
        </div>
        {error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={book.isPending}>
            {t('book.cancel')}
          </Button>
          <Button onClick={confirm} disabled={!customer || book.isPending}>
            {book.isPending && <Spinner />}
            {t('book.confirm')}
          </Button>
        </DialogFooter>
      </div>
    </FormDialog>
  );
}
