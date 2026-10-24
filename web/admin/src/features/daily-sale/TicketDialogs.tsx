import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import { FormDialog, OptionSelect } from '@arenile/shared/ui/patterns';
import { RadioGroup, RadioGroupItem } from '@arenile/shared/ui/radio-group';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Textarea } from '@arenile/shared/ui/textarea';
import { CircleAlertIcon } from 'lucide-react';
import { useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useOperationsDay } from '../operations/api/operations';
import { isBookable, slotStatus } from '../operations/lib/status';
import { useAssignTicket, useMoveTicket, useVoidTicket, type Ticket } from './api';
import { StableKey } from './lib';
import { useSaleProblemText } from './useSaleProblemText';

interface TicketDialogProps {
  establishmentId: string;
  /** Mounted only while a ticket is picked (closing unmounts it). */
  ticket: Ticket;
  onOpenChange: (open: boolean) => void;
}

/**
 * Move an assigned ticket to another free spot, or place a "da assegnare"
 * ticket on a free spot of its zone. The server allocates atomically and
 * answers 409 when the spot was taken in the meantime.
 */
export function MoveTicketDialog({ establishmentId, ticket, onOpenChange }: TicketDialogProps) {
  const { t } = useTranslation('dailySale');
  const id = useId();
  const problemText = useSaleProblemText();
  const [spotId, setSpotId] = useState('');
  const assign = useAssignTicket(establishmentId);
  const move = useMoveTicket(establishmentId);
  const day = useOperationsDay(establishmentId, ticket.ticket_date, ticket.slots);
  const toAssign = ticket.needs_assignment;
  const options = (day.data?.spots ?? [])
    .filter(
      (s) => isBookable(slotStatus(s, ticket.slots)) && (!toAssign || s.zone_id === ticket.zone_id),
    )
    .sort((a, b) => a.label.localeCompare(b.label, undefined, { numeric: true }))
    .map((s) => ({ value: s.spot_id, label: s.label }));
  const mutation = toAssign ? assign : move;

  const close = (open: boolean) => {
    if (!open) {
      setSpotId('');
      assign.reset();
      move.reset();
    }
    onOpenChange(open);
  };

  const submit = () => {
    if (!spotId) return;
    const label = options.find((o) => o.value === spotId)?.label ?? '';
    const done = () => {
      toast.success(t('move.done', { number: ticket.number_label, label }));
      close(false);
    };
    const vars = { ticketId: ticket.id, spotId, version: ticket.version };
    if (toAssign) assign.mutate(vars, { onSuccess: done });
    else move.mutate(vars, { onSuccess: done });
  };

  return (
    <FormDialog
      open
      onOpenChange={close}
      title={t(toAssign ? 'move.assignTitle' : 'move.title', { number: ticket.number_label })}
      description={toAssign ? t('move.assignDescription') : t('move.description')}
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <Label id={`${id}-spot`}>{t('move.spot')}</Label>
          <OptionSelect
            aria-labelledby={`${id}-spot`}
            value={spotId}
            onChange={setSpotId}
            options={options}
            placeholder={
              day.isSuccess && options.length === 0
                ? t('sale.noFreeSpots')
                : t('sale.spotPlaceholder')
            }
            className="w-full"
          />
        </div>
        {mutation.error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{problemText(mutation.error)}</AlertDescription>
          </Alert>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => close(false)}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </Button>
          <Button onClick={submit} disabled={!spotId || mutation.isPending}>
            {mutation.isPending && <Spinner />}
            {toAssign ? t('actions.assign') : t('actions.move')}
          </Button>
        </div>
      </div>
    </FormDialog>
  );
}

type RefundChoice = 'refund' | 'credit' | 'none';

/**
 * Void a ticket: the booking is cancelled through the booking service, the
 * spot or zone capacity is freed, the amount is refunded (or credited) and
 * the ticket keeps its number, marked "annullato".
 */
export function VoidTicketDialog({ establishmentId, ticket, onOpenChange }: TicketDialogProps) {
  const { t } = useTranslation('dailySale');
  const id = useId();
  const problemText = useSaleProblemText();
  const [reason, setReason] = useState('');
  const [refund, setRefund] = useState<RefundChoice>('refund');
  const [touched, setTouched] = useState(false);
  const voidTicket = useVoidTicket(establishmentId);
  const keyRef = useRef(new StableKey());

  const close = (open: boolean) => {
    if (!open) {
      setReason('');
      setRefund('refund');
      setTouched(false);
      voidTicket.reset();
    }
    onOpenChange(open);
  };

  const submit = () => {
    setTouched(true);
    if (!reason.trim()) return;
    const body = { reason: reason.trim(), refund };
    voidTicket.mutate(
      {
        ticketId: ticket.id,
        version: ticket.version,
        body,
        idempotencyKey: keyRef.current.for({ id: ticket.id, version: ticket.version, ...body }),
      },
      {
        onSuccess: () => {
          keyRef.current.reset();
          toast.success(t('void.done', { number: ticket.number_label }));
          close(false);
        },
      },
    );
  };

  const missing = touched && !reason.trim();
  const choices: RefundChoice[] = ticket.holder_name
    ? ['refund', 'credit', 'none']
    : ['refund', 'none'];

  return (
    <FormDialog
      open
      onOpenChange={close}
      title={t('void.title', { number: ticket.number_label })}
      description={t('void.description')}
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-reason`}>{t('void.reason')}</Label>
          <Textarea
            id={`${id}-reason`}
            value={reason}
            maxLength={500}
            aria-invalid={missing}
            aria-describedby={missing ? `${id}-reason-error` : undefined}
            onChange={(e) => setReason(e.target.value)}
          />
          {missing && (
            <p id={`${id}-reason-error`} className="text-sm text-destructive">
              {t('void.reasonRequired')}
            </p>
          )}
        </div>
        <div className="space-y-2">
          <Label id={`${id}-refund`}>{t('void.refund')}</Label>
          <RadioGroup
            aria-labelledby={`${id}-refund`}
            value={refund}
            onValueChange={(v) => setRefund(v as RefundChoice)}
          >
            {choices.map((c) => (
              <div key={c} className="flex items-center gap-2">
                <RadioGroupItem id={`${id}-${c}`} value={c} />
                <Label htmlFor={`${id}-${c}`} className="font-normal">
                  {t(`void.refund_${c}`)}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>
        {voidTicket.error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{problemText(voidTicket.error)}</AlertDescription>
          </Alert>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => close(false)}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </Button>
          <Button variant="destructive" onClick={submit} disabled={voidTicket.isPending}>
            {voidTicket.isPending && <Spinner />}
            {t('actions.void')}
          </Button>
        </div>
      </div>
    </FormDialog>
  );
}
