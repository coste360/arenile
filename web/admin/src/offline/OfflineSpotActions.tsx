import { useMe, type Schemas } from '@arenile/shared/api';
import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import { Field, FieldError, FieldLabel } from '@arenile/shared/ui/field';
import { FormActions, FormDialog, MoneyInput } from '@arenile/shared/ui/patterns';
import { Textarea } from '@arenile/shared/ui/textarea';
import { NotebookPenIcon, WalletIcon } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCurrentEstablishment } from '../establishment/context';
import { grantedPermissions } from '../features/operations/lib/permissions';
import { useOnlineStatus } from '../lib/online';
import { useOfflineRuntime } from './context';
import { spotLabel } from './labels';

type OperationalSpot = Schemas['OperationalSpot'];

export interface OfflineSpotActionsProps {
  establishmentId: string;
  spot: OperationalSpot;
}

/**
 * Offline actions of the spot card (spec 21.2): a note and a cash payment
 * on the booking of the spot, queued and sent on reconnection. Online it
 * renders nothing (the booking detail has the full forms). It also says
 * that creating and moving bookings is not available offline.
 */
export function OfflineSpotActions({ establishmentId, spot }: OfflineSpotActionsProps) {
  const { t } = useTranslation('offline');
  const online = useOnlineStatus();
  const runtime = useOfflineRuntime();
  const me = useMe();
  const { establishment } = useCurrentEstablishment();
  const [dialog, setDialog] = useState<'note' | 'cash' | null>(null);
  const b = spot.booking;

  if (online || !runtime) return null;
  const granted = grantedPermissions(me.data, establishment);
  const canNote = !!b && granted.has('bookings.write');
  const canCash = !!b && granted.has('cash.operate') && b.balance_due > 0;

  return (
    <div className="w-full space-y-2">
      <p className="text-sm text-muted-foreground">{t('actions.unavailable')}</p>
      {(canNote || canCash) && (
        <div className="flex flex-wrap gap-2">
          {canNote && (
            <Button variant="outline" onClick={() => setDialog('note')}>
              <NotebookPenIcon aria-hidden="true" />
              {t('actions.note')}
            </Button>
          )}
          {canCash && (
            <Button variant="outline" onClick={() => setDialog('cash')}>
              <WalletIcon aria-hidden="true" />
              {t('actions.cash')}
            </Button>
          )}
        </div>
      )}
      {b && dialog === 'note' && (
        <NoteDialog
          label={spotLabel(spot)}
          onClose={() => setDialog(null)}
          onSubmit={async (text) => {
            await runtime.enqueue({
              kind: 'note',
              establishmentId,
              bookingId: b.booking_id,
              label: spotLabel(spot),
              text,
            });
            toast.success(t('actions.queuedNote'));
          }}
        />
      )}
      {b && dialog === 'cash' && (
        <CashDialog
          balance={b.balance_due}
          onClose={() => setDialog(null)}
          onSubmit={async (amount) => {
            await runtime.enqueue({
              kind: 'cash_payment',
              establishmentId,
              bookingId: b.booking_id,
              label: spotLabel(spot),
              amount,
            });
            toast.success(t('actions.queuedCash', { amount: formatCents(amount) }));
          }}
        />
      )}
    </div>
  );
}

interface DialogProps<T> {
  onClose: () => void;
  onSubmit: (value: T) => Promise<void>;
}

function NoteDialog({ label, onClose, onSubmit }: DialogProps<string> & { label: string }) {
  const { t } = useTranslation('offline');
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!text.trim()) return setError(t('actions.noteRequired'));
    setPending(true);
    try {
      await onSubmit(text.trim());
      onClose();
    } finally {
      setPending(false);
    }
  };
  return (
    <FormDialog
      open
      onOpenChange={(v) => !v && onClose()}
      title={t('actions.noteTitle')}
      description={t('actions.noteDescription', { label })}
    >
      <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
        <Field data-invalid={!!error || undefined}>
          <FieldLabel htmlFor="offline-note">{t('actions.noteLabel')}</FieldLabel>
          <Textarea
            id="offline-note"
            value={text}
            aria-invalid={!!error || undefined}
            onChange={(e) => {
              setText(e.target.value);
              setError(null);
            }}
          />
          <FieldError>{error}</FieldError>
        </Field>
        <FormActions onCancel={onClose} pending={pending} submitLabel={t('actions.submit')} />
      </form>
    </FormDialog>
  );
}

function CashDialog({ balance, onClose, onSubmit }: DialogProps<number> & { balance: number }) {
  const { t } = useTranslation('offline');
  const [amount, setAmount] = useState<number | null>(balance);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    // Overpayment as credit needs the server-side balance: not offered offline.
    if (amount === null || amount <= 0 || amount > balance) {
      return setError(t('actions.amountInvalid'));
    }
    setPending(true);
    try {
      await onSubmit(amount);
      onClose();
    } finally {
      setPending(false);
    }
  };
  return (
    <FormDialog
      open
      onOpenChange={(v) => !v && onClose()}
      title={t('actions.cashTitle')}
      description={t('actions.cashDescription', { balance: formatCents(balance) })}
    >
      <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
        <Field data-invalid={!!error || undefined}>
          <FieldLabel htmlFor="offline-cash">{t('actions.amount')}</FieldLabel>
          <MoneyInput
            id="offline-cash"
            value={amount}
            aria-invalid={!!error || undefined}
            onChange={(v) => {
              setAmount(v);
              setError(null);
            }}
          />
          <FieldError>{error}</FieldError>
        </Field>
        <FormActions onCancel={onClose} pending={pending} submitLabel={t('actions.submit')} />
      </form>
    </FormDialog>
  );
}
