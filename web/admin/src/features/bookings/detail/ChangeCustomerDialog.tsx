import { useProblemText } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import { FormDialog } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { CircleAlertIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { Customer } from '../../customers/api';
import { CustomerPicker } from '../../customers/CustomerPicker';
import { usePatchBooking, type BookingChangeResult, type BookingDetail } from '../api';

export interface ChangeCustomerDialogProps {
  establishmentId: string;
  booking: BookingDetail;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * "Cambia cliente" (spec 30.9): search the customer, preview the price
 * difference (a dry run of the existing change, customer_id: the default
 * agreement and the tags of the new customer may change the price), then
 * apply. The booking version is sent as If-Match.
 */
export function ChangeCustomerDialog({
  establishmentId,
  booking,
  open,
  onOpenChange,
}: ChangeCustomerDialogProps) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const patch = usePatchBooking(establishmentId);
  const pickerId = useId();
  const [customer, setCustomer] = useState<Customer | null>(null);
  const [preview, setPreview] = useState<BookingChangeResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = (dryRun: boolean) => {
    if (!customer) return;
    setError(null);
    patch.mutate(
      {
        bookingId: booking.id,
        version: booking.version,
        patch: { customer_id: customer.id },
        dryRun,
      },
      {
        onSuccess: (res) => {
          if (dryRun) {
            setPreview(res);
            return;
          }
          toast.success(t('bookingActions.customerChanged', { name: customer.display_name }));
          onOpenChange(false);
        },
        onError: (e) => setError(problemText(e)),
      },
    );
  };

  const diff = preview?.price_difference ?? 0;
  return (
    <FormDialog
      open={open}
      onOpenChange={(v) => !patch.isPending && onOpenChange(v)}
      title={t('bookingActions.changeCustomerTitle', { code: booking.code })}
      description={t('bookingActions.changeCustomerDescription', {
        name: booking.customer.display_name,
      })}
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor={pickerId}>{t('bookingActions.newCustomer')}</Label>
          <CustomerPicker
            id={pickerId}
            establishmentId={establishmentId}
            value={customer}
            onChange={(c) => {
              setCustomer(c);
              setPreview(null);
            }}
          />
        </div>
        {preview && (
          <p className="text-sm tabular-nums" data-testid="change-customer-price">
            {diff === 0
              ? t('move.samePrice')
              : diff > 0
                ? t('move.toCollect', { amount: formatCents(diff) })
                : t('move.toRefund', { amount: formatCents(-diff) })}
          </p>
        )}
        {error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={patch.isPending}>
            {t('actions.cancel')}
          </Button>
          {preview ? (
            <Button onClick={() => run(false)} disabled={patch.isPending}>
              {patch.isPending && <Spinner />}
              {t('bookingActions.changeCustomerConfirm')}
            </Button>
          ) : (
            <Button
              onClick={() => run(true)}
              disabled={!customer || customer.id === booking.customer.id || patch.isPending}
            >
              {patch.isPending && <Spinner />}
              {t('bookingActions.check')}
            </Button>
          )}
        </DialogFooter>
      </div>
    </FormDialog>
  );
}
