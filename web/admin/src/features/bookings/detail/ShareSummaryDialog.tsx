import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Label } from '@arenile/shared/ui/label';
import { FormDialog } from '@arenile/shared/ui/patterns';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { Textarea } from '@arenile/shared/ui/textarea';
import { CircleAlertIcon, CopyIcon, MessageCircleIcon } from 'lucide-react';
import { useEffect, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { BookingDetail } from '../api';
import { useShareSummary } from './m8api';

export interface ShareSummaryDialogProps {
  establishmentId: string;
  booking: BookingDetail;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * "Riepilogo via WhatsApp" (spec 30.9): the ready text (dates, spots,
 * amount, manage link) and a wa.me link to the customer's number. Arenile
 * sends nothing to third parties: the operator opens WhatsApp and confirms
 * the message there. Offered only when the customer has a phone; the text
 * can also be copied.
 */
export function ShareSummaryDialog({
  establishmentId,
  booking,
  open,
  onOpenChange,
}: ShareSummaryDialogProps) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const summary = useShareSummary(establishmentId);
  const textId = useId();
  const { mutate, reset } = summary;

  // A fresh text (and manage link) each time the dialog opens.
  useEffect(() => {
    if (open) mutate({ bookingId: booking.id });
    else reset();
  }, [open, booking.id, mutate, reset]);

  const copy = async () => {
    if (!summary.data) return;
    try {
      await navigator.clipboard.writeText(summary.data.text);
      toast.success(t('bookingActions.copied'));
    } catch {
      toast.error(t('bookingActions.copyFailed'));
    }
  };

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('bookingActions.shareTitle', { code: booking.code })}
      description={t('bookingActions.shareDescription')}
    >
      <div className="space-y-4">
        {summary.isPending && <Skeleton className="h-32 w-full" />}
        {summary.error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{problemText(summary.error)}</AlertDescription>
          </Alert>
        )}
        {summary.data && (
          <div className="space-y-1.5">
            <Label htmlFor={textId}>{t('bookingActions.shareText')}</Label>
            <Textarea id={textId} readOnly rows={8} value={summary.data.text} />
          </div>
        )}
        <DialogFooter className="border-t pt-4">
          <Button variant="outline" onClick={() => void copy()} disabled={!summary.data}>
            <CopyIcon aria-hidden="true" />
            {t('bookingActions.copy')}
          </Button>
          {summary.data?.whatsapp_url ? (
            <Button asChild>
              <a href={summary.data.whatsapp_url} target="_blank" rel="noopener noreferrer">
                <MessageCircleIcon aria-hidden="true" />
                {t('bookingActions.openWhatsApp')}
              </a>
            </Button>
          ) : (
            <Button disabled>
              <MessageCircleIcon aria-hidden="true" />
              {t('bookingActions.openWhatsApp')}
            </Button>
          )}
        </DialogFooter>
      </div>
    </FormDialog>
  );
}
