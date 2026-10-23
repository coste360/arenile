import { formatCents } from '@arenile/shared/lib/money';
import { Label } from '@arenile/shared/ui/label';
import { Switch } from '@arenile/shared/ui/switch';
import { Textarea } from '@arenile/shared/ui/textarea';
import { DatePicker, FormActions, FormDialog } from '@arenile/shared/ui/patterns';
import { useId, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useCurrentEstablishment } from '../../establishment';
import { todayIso } from '../bookings/model';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { newIdempotencyKey } from '../configuration/lib/validation';
import { useAcceptProposal, useDeclineProposal, type RenewalProposal } from './api';
import { zonedDate, zonedInstant } from './lib';
import { describeM5bProblem } from './problems';

interface ProposalDialogProps {
  establishmentId: string;
  proposal: RenewalProposal | null;
  onClose: () => void;
}

/** Desk acceptance (spec 10.4): confirm now (manual driver) or leave it awaiting payment. */
export function AcceptProposalDialog({ establishmentId, proposal, onClose }: ProposalDialogProps) {
  const { t } = useTranslation('renewals');
  return (
    <FormDialog
      open={!!proposal}
      onOpenChange={(o) => !o && onClose()}
      title={t('accept.title')}
      description={
        proposal
          ? t('accept.description', {
              spot: proposal.spot_label,
              customer: proposal.customer_display_name,
              amount: formatCents(proposal.total_amount),
            })
          : undefined
      }
    >
      {proposal && (
        <AcceptForm establishmentId={establishmentId} proposal={proposal} onDone={onClose} />
      )}
    </FormDialog>
  );
}

function AcceptForm({
  establishmentId,
  proposal,
  onDone,
}: {
  establishmentId: string;
  proposal: RenewalProposal;
  onDone: () => void;
}) {
  const { t } = useTranslation('renewals');
  const accept = useAcceptProposal(establishmentId);
  const [key] = useState(newIdempotencyKey);
  const [confirm, setConfirm] = useState(true);
  const [note, setNote] = useState('');
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const id = useId();
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setFailure(null);
    try {
      await accept.mutateAsync({
        proposal,
        body: { confirm, note: note.trim() || undefined },
        idempotencyKey: key,
      });
      toast.success(t('accept.done', { spot: proposal.spot_label }));
      onDone();
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };
  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      <div className="flex items-start gap-3">
        <Switch id={`${id}-confirm`} checked={confirm} onCheckedChange={setConfirm} />
        <div className="space-y-1">
          <Label htmlFor={`${id}-confirm`}>{t('accept.confirmNow')}</Label>
          <p className="text-sm text-muted-foreground">
            {confirm ? t('accept.confirmNowHelp') : t('accept.awaitPaymentHelp')}
          </p>
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${id}-note`}>
          {t('accept.note')}{' '}
          <span className="font-normal text-muted-foreground">{t('optional')}</span>
        </Label>
        <Textarea
          id={`${id}-note`}
          rows={2}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>
      <FormActions onCancel={onDone} pending={accept.isPending} submitLabel={t('accept.submit')} />
    </form>
  );
}

/** Desk decline: the held booking is cancelled and the spot freed (spec 10.4). */
export function DeclineProposalDialog({ establishmentId, proposal, onClose }: ProposalDialogProps) {
  const { t } = useTranslation('renewals');
  return (
    <FormDialog
      open={!!proposal}
      onOpenChange={(o) => !o && onClose()}
      title={t('decline.title')}
      description={
        proposal
          ? t('decline.description', {
              spot: proposal.spot_label,
              customer: proposal.customer_display_name,
            })
          : undefined
      }
    >
      {proposal && (
        <DeclineForm establishmentId={establishmentId} proposal={proposal} onDone={onClose} />
      )}
    </FormDialog>
  );
}

function DeclineForm({
  establishmentId,
  proposal,
  onDone,
}: {
  establishmentId: string;
  proposal: RenewalProposal;
  onDone: () => void;
}) {
  const { t } = useTranslation('renewals');
  const decline = useDeclineProposal(establishmentId);
  const [reason, setReason] = useState('');
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const id = useId();
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setFailure(null);
    try {
      await decline.mutateAsync({ proposal, body: { reason: reason.trim() || undefined } });
      toast.success(t('decline.done', { spot: proposal.spot_label }));
      onDone();
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };
  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      <div className="space-y-2">
        <Label htmlFor={`${id}-reason`}>
          {t('decline.reason')}{' '}
          <span className="font-normal text-muted-foreground">{t('optional')}</span>
        </Label>
        <Textarea
          id={`${id}-reason`}
          rows={2}
          maxLength={500}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
      <FormActions
        onCancel={onDone}
        pending={decline.isPending}
        submitLabel={t('decline.submit')}
      />
    </form>
  );
}

/**
 * New pre-emption deadline as a date (end of that day in the establishment
 * time zone): used for a campaign, a single proposal or a selection.
 */
export function DeadlineDialog({
  open,
  onOpenChange,
  title,
  description,
  current,
  maxDate,
  pending,
  failure,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  /** Current deadline (RFC 3339), preselected. */
  current?: string;
  /** Last allowed date (the day before the target season opens). */
  maxDate?: string;
  pending: boolean;
  failure: ProblemText | null;
  onSubmit: (deadlineAt: string) => void;
}) {
  const { t } = useTranslation('renewals');
  const { establishment } = useCurrentEstablishment();
  const tz = establishment?.timezone ?? 'Europe/Rome';
  const [date, setDate] = useState(current ? zonedDate(current, tz) : '');
  const id = useId();
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (date) onSubmit(zonedInstant(date, tz));
  };
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} title={title} description={description}>
      <form noValidate onSubmit={submit} className="space-y-4">
        <ProblemAlert problem={failure} />
        <div className="space-y-2">
          <Label id={`${id}-date`}>{t('deadline.date')}</Label>
          <DatePicker
            aria-labelledby={`${id}-date`}
            value={date}
            onChange={(v) => setDate(v ?? '')}
            min={todayIso()}
            max={maxDate}
          />
          <p className="text-sm text-muted-foreground">{t('deadline.help')}</p>
        </div>
        <FormActions
          onCancel={() => onOpenChange(false)}
          pending={pending}
          submitLabel={t('deadline.submit')}
        />
      </form>
    </FormDialog>
  );
}
