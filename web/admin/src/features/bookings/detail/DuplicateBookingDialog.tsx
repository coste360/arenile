import { useProblemText } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import { DateRangePicker, FormDialog } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { useNavigate } from '@tanstack/react-router';
import { CircleAlertIcon } from 'lucide-react';
import { useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { newIdempotencyKey } from '../../configuration/lib/validation';
import type { BookingDetail } from '../api';
import { useDuplicateBooking, type BookingDuplicateResult } from './m8api';

export interface DuplicateBookingDialogProps {
  establishmentId: string;
  booking: BookingDetail;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * "Duplica" (spec 30.9): same spot, customer, extras, agreement and group,
 * new dates. "Verifica" runs a dry run (new price, conflicts); "Crea" makes
 * the booking, the database guaranteeing the spot is free (409 otherwise).
 */
export function DuplicateBookingDialog({
  establishmentId,
  booking,
  open,
  onOpenChange,
}: DuplicateBookingDialogProps) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const navigate = useNavigate();
  const dup = useDuplicateBooking(establishmentId);
  const datesId = useId();
  const [range, setRange] = useState<{ from: string; to?: string } | undefined>();
  const [preview, setPreview] = useState<BookingDuplicateResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const keys = useRef(new Map<string, string>());
  const body = range ? { starts_on: range.from, ends_on: range.to ?? range.from } : null;
  const keyFor = (k: string) => {
    let v = keys.current.get(k);
    if (!v) {
      v = newIdempotencyKey();
      keys.current.set(k, v);
    }
    return v;
  };

  const run = (dryRun: boolean) => {
    if (!body) return;
    setError(null);
    dup.mutate(
      {
        bookingId: booking.id,
        body: { ...body, dry_run: dryRun },
        idempotencyKey: keyFor(`${dryRun}:${body.starts_on}:${body.ends_on}`),
      },
      {
        onSuccess: (res) => {
          if (dryRun) {
            setPreview(res);
            return;
          }
          toast.success(t('bookingActions.duplicated', { code: res.booking?.code ?? '' }));
          onOpenChange(false);
          if (res.booking) {
            void navigate({
              to: '/e/$establishmentId/prenotazioni/$bookingId',
              params: { establishmentId, bookingId: res.booking.id },
            });
          }
        },
        onError: (e) => setError(problemText(e)),
      },
    );
  };

  const conflicts = preview?.conflicts ?? [];
  return (
    <FormDialog
      open={open}
      onOpenChange={(v) => !dup.isPending && onOpenChange(v)}
      title={t('bookingActions.duplicateTitle', { code: booking.code })}
      description={t('bookingActions.duplicateDescription')}
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <Label id={datesId}>{t('bookingActions.newDates')}</Label>
          <DateRangePicker
            aria-labelledby={datesId}
            value={range ?? null}
            onChange={(v) => {
              setRange(v);
              setPreview(null);
            }}
          />
        </div>
        {preview && conflicts.length > 0 && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertTitle>{t('move.conflicts', { count: conflicts.length })}</AlertTitle>
            <AlertDescription>
              <ul className="list-disc pl-4">
                {conflicts.slice(0, 5).map((c) => (
                  <li key={`${c.date}-${c.slot}-${c.reason}`}>
                    {formatIsoDate(c.date)} {t(`slot.${c.slot}`)}: {t(`conflict.${c.reason}`)}
                  </li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        )}
        {preview && conflicts.length === 0 && (
          <p className="text-sm" data-testid="duplicate-total">
            {t('bookingActions.newTotal')}{' '}
            <span className="font-semibold tabular-nums">{formatCents(preview.total)}</span>
          </p>
        )}
        {error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={dup.isPending}>
            {t('actions.cancel')}
          </Button>
          {preview && conflicts.length === 0 ? (
            <Button onClick={() => run(false)} disabled={dup.isPending}>
              {dup.isPending && <Spinner />}
              {t('bookingActions.duplicateConfirm')}
            </Button>
          ) : (
            <Button onClick={() => run(true)} disabled={!body || dup.isPending}>
              {dup.isPending && <Spinner />}
              {t('bookingActions.check')}
            </Button>
          )}
        </DialogFooter>
      </div>
    </FormDialog>
  );
}
