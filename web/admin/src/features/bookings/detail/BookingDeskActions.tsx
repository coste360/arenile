import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { Spinner } from '@arenile/shared/ui/spinner';
import {
  CopyPlusIcon,
  EllipsisIcon,
  MessageCircleIcon,
  PrinterIcon,
  Undo2Icon,
  UserRoundPenIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useUndoMove } from '../../operations/api/m8';
import { undoTimeLeft } from '../../operations/components/useUndoToast';
import type { BookingDetail } from '../api';
import { ChangeCustomerDialog } from './ChangeCustomerDialog';
import { DuplicateBookingDialog } from './DuplicateBookingDialog';
import { summaryPrintUrl } from './m8api';
import { ShareSummaryDialog } from './ShareSummaryDialog';

export interface BookingDeskActionsProps {
  establishmentId: string;
  booking: BookingDetail;
  /** bookings.write */
  canWrite: boolean;
}

type DialogName = 'duplicate' | 'customer' | 'share' | null;

const ACTIVE = new Set(['draft', 'held', 'awaiting_approval', 'confirmed', 'checked_in']);

/**
 * "Altre azioni" of a booking (spec 30.9): duplicate on new dates, change
 * customer with the price difference, summary via WhatsApp (only with a
 * phone) and the printable summary with the booking QR.
 */
export function BookingDeskActions({
  establishmentId,
  booking,
  canWrite,
}: BookingDeskActionsProps) {
  const { t } = useTranslation('operations');
  const [dialog, setDialog] = useState<DialogName>(null);
  const active = ACTIVE.has(booking.status);
  const hasPhone = !!booking.customer.phone?.trim() && !booking.customer.anonymized_at;
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline">
            <EllipsisIcon aria-hidden="true" />
            {t('bookingActions.more')}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {canWrite && !booking.is_season_pass && (
            <DropdownMenuItem onSelect={() => setDialog('duplicate')}>
              <CopyPlusIcon aria-hidden="true" />
              {t('bookingActions.duplicate')}
            </DropdownMenuItem>
          )}
          {canWrite && active && (
            <DropdownMenuItem onSelect={() => setDialog('customer')}>
              <UserRoundPenIcon aria-hidden="true" />
              {t('bookingActions.changeCustomer')}
            </DropdownMenuItem>
          )}
          {canWrite && hasPhone && (
            <DropdownMenuItem onSelect={() => setDialog('share')}>
              <MessageCircleIcon aria-hidden="true" />
              {t('bookingActions.share')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem asChild>
            <a
              href={summaryPrintUrl(establishmentId, booking.id)}
              target="_blank"
              rel="noopener noreferrer"
            >
              <PrinterIcon aria-hidden="true" />
              {t('bookingActions.printSummary')}
            </a>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DuplicateBookingDialog
        establishmentId={establishmentId}
        booking={booking}
        open={dialog === 'duplicate'}
        onOpenChange={(o) => setDialog(o ? 'duplicate' : null)}
      />
      {dialog === 'customer' && (
        <ChangeCustomerDialog
          establishmentId={establishmentId}
          booking={booking}
          open
          onOpenChange={(o) => setDialog(o ? 'customer' : null)}
        />
      )}
      <ShareSummaryDialog
        establishmentId={establishmentId}
        booking={booking}
        open={dialog === 'share'}
        onOpenChange={(o) => setDialog(o ? 'share' : null)}
      />
    </>
  );
}

/**
 * Notice of the last move of the booking still undoable (spec 30.7, 5
 * minutes): "Annulla spostamento" puts the lines back if the old spots are
 * still free (409 otherwise, nothing changes).
 */
export function LastMoveNotice({ establishmentId, booking, canWrite }: BookingDeskActionsProps) {
  const { t } = useTranslation('operations');
  const problemText = useProblemText();
  const undo = useUndoMove(establishmentId);
  const move = booking.last_move;
  if (!canWrite || !move || undoTimeLeft(move) <= 0) return null;
  return (
    <Alert role="status">
      <Undo2Icon aria-hidden="true" />
      <AlertDescription className="flex flex-wrap items-center gap-3">
        <span>{t('undo.notice')}</span>
        <Button
          size="sm"
          variant="outline"
          disabled={undo.isPending}
          onClick={() =>
            undo.mutate(
              { moveId: move.move_id },
              {
                onSuccess: () => toast.success(t('undo.done')),
                onError: (e) => toast.error(problemText(e)),
              },
            )
          }
        >
          {undo.isPending ? <Spinner /> : <Undo2Icon aria-hidden="true" />}
          {t('undo.action')}
        </Button>
      </AlertDescription>
    </Alert>
  );
}
