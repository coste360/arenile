import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import {
  DatePicker,
  FormActions,
  FormDialog,
  MoneyInput,
  OptionSelect,
  SegmentedControl,
} from '@arenile/shared/ui/patterns';
import { useId, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useRecordPayment } from '../bookings/api';
import { todayIso } from '../bookings/model';
import type { ProblemText } from '../bookings/problem';
import { ProblemAlert } from '../bookings/ProblemAlert';
import { newIdempotencyKey } from '../configuration/lib/validation';
import { describeM5bProblem } from '../renewals/problems';
import { useCreateCustomerCredit, useInvalidateCredits, type CustomerCreditCreate } from './api';
import type { Schemas } from '@arenile/shared/api';

type BookingSummary = Schemas['BookingSummary'];
type GrantSource = CustomerCreditCreate['source'];

function Field({
  labelId,
  label,
  optional,
  error,
  help,
  children,
}: {
  labelId: string;
  label: string;
  optional?: boolean;
  error?: string;
  help?: string;
  children: ReactNode;
}) {
  const { t } = useTranslation('credits');
  return (
    <div className="min-w-0 space-y-2">
      <Label id={labelId}>
        {label}
        {optional && <span className="font-normal text-muted-foreground"> {t('optional')}</span>}
      </Label>
      {children}
      {help && !error && <p className="text-sm text-muted-foreground">{help}</p>}
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}

/** "Riconosci credito" (spec 13.4, refunds.issue): goodwill, other credit or an adjustment. */
export function GrantCreditDialog({
  establishmentId,
  customerId,
  available,
  open,
  onOpenChange,
}: {
  establishmentId: string;
  customerId: string;
  available: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('credits');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('grant.title')}
      description={t('grant.description')}
    >
      {open && (
        <GrantForm
          establishmentId={establishmentId}
          customerId={customerId}
          available={available}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function GrantForm({
  establishmentId,
  customerId,
  available,
  onDone,
}: {
  establishmentId: string;
  customerId: string;
  available: number;
  onDone: () => void;
}) {
  const { t } = useTranslation('credits');
  const id = useId();
  const create = useCreateCustomerCredit(establishmentId, customerId);
  const [key] = useState(newIdempotencyKey);
  const [source, setSource] = useState<GrantSource>('goodwill');
  const [direction, setDirection] = useState<'add' | 'remove'>('add');
  const [amount, setAmount] = useState<number | null>(null);
  const [expiresOn, setExpiresOn] = useState<string | undefined>();
  const [note, setNote] = useState('');
  const [submitted, setSubmitted] = useState(false);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const removing = source === 'adjustment' && direction === 'remove';
  const amountError =
    amount === null || amount <= 0
      ? t('grant.amountRequired')
      : removing && amount > available
        ? t('grant.amountTooHigh', { available: formatCents(available) })
        : undefined;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    setFailure(null);
    if (amountError || amount === null) return;
    try {
      await create.mutateAsync({
        body: {
          source,
          amount: removing ? -amount : amount,
          expires_on: removing ? undefined : expiresOn,
          note: note.trim() || undefined,
        },
        idempotencyKey: key,
      });
      toast.success(
        removing
          ? t('grant.removed', { amount: formatCents(amount) })
          : t('grant.done', { amount: formatCents(amount) }),
      );
      onDone();
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };

  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      <Field labelId={`${id}-source`} label={t('grant.source')} help={t(`grant.help.${source}`)}>
        <SegmentedControl
          label={t('grant.source')}
          value={source}
          onValueChange={(v) => setSource(v as GrantSource)}
          options={(['goodwill', 'manual', 'adjustment'] as const).map((s) => ({
            value: s,
            label: t(`source.${s}`),
          }))}
          collapseOnNarrow
        />
      </Field>
      {source === 'adjustment' && (
        <Field labelId={`${id}-direction`} label={t('grant.direction')}>
          <SegmentedControl
            label={t('grant.direction')}
            value={direction}
            onValueChange={(v) => setDirection(v as 'add' | 'remove')}
            options={[
              { value: 'add', label: t('grant.add') },
              { value: 'remove', label: t('grant.remove') },
            ]}
          />
        </Field>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <Field
          labelId={`${id}-amount`}
          label={t('grant.amount')}
          error={submitted ? amountError : undefined}
        >
          <MoneyInput
            aria-labelledby={`${id}-amount`}
            value={amount}
            onChange={setAmount}
            aria-invalid={submitted && !!amountError}
          />
        </Field>
        {!removing && (
          <Field
            labelId={`${id}-expires`}
            label={t('grant.expires')}
            optional
            help={t('grant.expiresHelp')}
          >
            <DatePicker
              aria-labelledby={`${id}-expires`}
              value={expiresOn}
              onChange={setExpiresOn}
              min={todayIso()}
              clearable
            />
          </Field>
        )}
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${id}-note`}>
          {t('grant.note')}
          <span className="font-normal text-muted-foreground"> {t('optional')}</span>
        </Label>
        <Input
          id={`${id}-note`}
          value={note}
          maxLength={500}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>
      <FormActions
        onCancel={onDone}
        pending={create.isPending}
        submitLabel={removing ? t('grant.submitRemove') : t('grant.submit')}
      />
    </form>
  );
}

/**
 * Uses the credit as a payment (method `credit`, spec 13.4) on one of the
 * customer's bookings with a balance due; at most the available credit.
 */
export function ApplyCreditDialog({
  establishmentId,
  available,
  bookings,
  open,
  onOpenChange,
}: {
  establishmentId: string;
  available: number;
  bookings: BookingSummary[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('credits');
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('apply.title')}
      description={t('apply.description', { available: formatCents(available) })}
    >
      {open && (
        <ApplyForm
          establishmentId={establishmentId}
          available={available}
          bookings={bookings}
          onDone={() => onOpenChange(false)}
        />
      )}
    </FormDialog>
  );
}

function ApplyForm({
  establishmentId,
  available,
  bookings,
  onDone,
}: {
  establishmentId: string;
  available: number;
  bookings: BookingSummary[];
  onDone: () => void;
}) {
  const { t } = useTranslation('credits');
  const id = useId();
  const record = useRecordPayment(establishmentId);
  const invalidate = useInvalidateCredits(establishmentId);
  const [key] = useState(newIdempotencyKey);
  const first = bookings[0];
  const [bookingId, setBookingId] = useState(first?.id ?? '');
  const booking = bookings.find((b) => b.id === bookingId);
  const [amount, setAmount] = useState<number | null>(
    first ? Math.min(first.balance_due, available) : null,
  );
  const [submitted, setSubmitted] = useState(false);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const max = booking ? Math.min(booking.balance_due, available) : available;
  const amountError =
    amount === null || amount <= 0
      ? t('grant.amountRequired')
      : amount > max
        ? t('apply.amountTooHigh', { max: formatCents(max) })
        : undefined;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    setFailure(null);
    if (!booking || amountError || amount === null) return;
    try {
      await record.mutateAsync({
        bookingId: booking.id,
        input: { amount, method: 'credit', note: t('apply.note') },
        idempotencyKey: key,
      });
      invalidate();
      toast.success(t('apply.done', { amount: formatCents(amount), code: booking.code }));
      onDone();
    } catch (err) {
      setFailure(describeM5bProblem(err, t));
    }
  };

  return (
    <form noValidate onSubmit={(e) => void submit(e)} className="space-y-4">
      <ProblemAlert problem={failure} />
      <Field labelId={`${id}-booking`} label={t('apply.booking')}>
        <OptionSelect
          aria-labelledby={`${id}-booking`}
          value={bookingId}
          onChange={(v) => {
            setBookingId(v);
            const b = bookings.find((x) => x.id === v);
            if (b) setAmount(Math.min(b.balance_due, available));
          }}
          options={bookings.map((b) => ({
            value: b.id,
            label: `${b.code} · ${formatIsoDate(b.starts_on)} · ${t('apply.due', {
              amount: formatCents(b.balance_due),
            })}`,
          }))}
        />
      </Field>
      <Field
        labelId={`${id}-amount`}
        label={t('grant.amount')}
        error={submitted ? amountError : undefined}
        help={t('apply.max', { max: formatCents(max) })}
      >
        <MoneyInput
          aria-labelledby={`${id}-amount`}
          value={amount}
          onChange={setAmount}
          aria-invalid={submitted && !!amountError}
        />
      </Field>
      <FormActions onCancel={onDone} pending={record.isPending} submitLabel={t('apply.submit')} />
    </form>
  );
}
