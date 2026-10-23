import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  ConfirmDialog,
  EmptyState,
  FieldRow,
  PageHeader,
  QueryState,
  StatusBadge,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link, useParams } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  BanIcon,
  CalendarRangeIcon,
  CheckIcon,
  HistoryIcon,
  LogInIcon,
  MoveIcon,
  ThumbsDownIcon,
  UserXIcon,
  WalletIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useRequiredEstablishmentId } from '../../establishment';
import {
  useAddBookingNote,
  useBooking,
  useBookingTransition,
  type BookingDetail,
  type TransitionVars,
} from './api';
import { noteSchema, type NoteValues } from './booking.schema';
import { CancelBookingDialog } from './detail/CancelBookingDialog';
import { ChangeDatesDialog, MoveBookingDialog } from './detail/ChangeDialogs';
import { RecordPaymentDialog, RefundDialog } from './detail/PaymentDialogs';
import {
  ACTIVE_STATUSES,
  availableActions,
  STATUS_TONE,
  useOperationalPermissions,
  type BookingAction,
} from './model';
import { describeProblem, type ProblemText } from './problem';
import { ProblemAlert } from './ProblemAlert';
import { useEstablishmentSpots } from './spots';
import { useCatalogNames } from './usePackName';

type DialogName = 'cancel' | 'pay' | 'refund' | 'dates' | 'move' | null;

const ACTION_ICONS: Record<BookingAction, typeof CheckIcon> = {
  confirm: CheckIcon,
  approve: CheckIcon,
  reject: ThumbsDownIcon,
  'check-in': LogInIcon,
  'no-show': UserXIcon,
  cancel: BanIcon,
};

/** Prenotazione: header with status and actions, lines, allocations, payments, history, notes. */
export function BookingDetailPage() {
  const { t, i18n } = useTranslation('bookings');
  const establishmentId = useRequiredEstablishmentId();
  const { bookingId } = useParams({ strict: false }) as { bookingId?: string };
  const query = useBooking(establishmentId, bookingId);
  const inventory = useEstablishmentSpots(establishmentId, i18n.language);
  const perms = useOperationalPermissions();
  const transition = useBookingTransition(establishmentId);
  const [dialog, setDialog] = useState<DialogName>(null);
  const [moveLine, setMoveLine] = useState<string | undefined>();
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const reload = () => {
    setFailure(null);
    void query.refetch();
  };

  const back = (
    <Button variant="ghost" size="sm" asChild>
      <Link to="/e/$establishmentId/prenotazioni" params={{ establishmentId }}>
        <ArrowLeftIcon aria-hidden="true" />
        {t('detail.back')}
      </Link>
    </Button>
  );

  if (!query.data) {
    return (
      <section aria-labelledby="page-title" className="space-y-6">
        {back}
        <PageHeader title={t('detail.title')} />
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
          rows={6}
        />
      </section>
    );
  }

  const b = query.data;
  const actions = availableActions(b.status, b.next_statuses).filter((a) =>
    a === 'check-in' || a === 'no-show' ? perms.canOperate : perms.canWrite,
  );
  const active = ACTIVE_STATUSES.includes(b.status);
  const run = async (action: TransitionVars['action']) => {
    setFailure(null);
    try {
      await transition.mutateAsync({ action, bookingId: b.id, version: b.version });
      toast.success(t(`actions.done.${action}`, { code: b.code }));
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  };

  return (
    <section aria-labelledby="page-title" className="space-y-6">
      {back}
      <PageHeader
        eyebrow={b.is_season_pass ? t('detail.pass') : t(`channel.${b.channel}`)}
        title={
          <span className="flex flex-wrap items-center gap-3">
            {b.code}
            <StatusBadge tone={STATUS_TONE[b.status]}>{t(`status.${b.status}`)}</StatusBadge>
          </span>
        }
        description={
          <>
            <Link
              to="/e/$establishmentId/clienti/$customerId"
              params={{ establishmentId, customerId: b.customer.id }}
              className="font-medium text-primary-on-surface underline-offset-4 hover:underline"
            >
              {b.customer.display_name}
            </Link>
            {' · '}
            {formatIsoDate(b.starts_on)}
            {b.ends_on !== b.starts_on && ` → ${formatIsoDate(b.ends_on)}`}
            {' · '}
            {t(`slots.${b.slots}`)} · {t(`formula.${b.formula}`)}
          </>
        }
        actions={
          <div className="flex flex-wrap gap-2">
            {actions.map((a) => {
              const Icon = ACTION_ICONS[a];
              if (a === 'cancel') {
                return (
                  <Button key={a} variant="destructive" onClick={() => setDialog('cancel')}>
                    <Icon aria-hidden="true" />
                    {t('actions.cancel')}
                  </Button>
                );
              }
              const destructive = a === 'reject' || a === 'no-show';
              return (
                <ConfirmDialog
                  key={a}
                  title={t(`actions.confirmTitle.${a}`, { code: b.code })}
                  description={t(`actions.confirmText.${a}`)}
                  confirmLabel={t(`actions.${a}`)}
                  destructive={destructive}
                  onConfirm={() => run(a)}
                  trigger={
                    <Button
                      variant={
                        a === 'confirm' || a === 'approve' || a === 'check-in'
                          ? 'default'
                          : 'outline'
                      }
                    >
                      <Icon aria-hidden="true" />
                      {t(`actions.${a}`)}
                    </Button>
                  }
                />
              );
            })}
            {perms.canWrite && active && !b.is_season_pass && (
              <Button variant="outline" onClick={() => setDialog('dates')}>
                <CalendarRangeIcon aria-hidden="true" />
                {t('actions.dates')}
              </Button>
            )}
          </div>
        }
      />
      <ProblemAlert problem={failure} onReload={reload} />
      {b.hold_expires_at && (
        <p className="text-sm text-muted-foreground">
          {t('detail.holdExpires', {
            at: new Date(b.hold_expires_at).toLocaleString(i18n.language),
          })}
        </p>
      )}

      <div className="grid gap-4 sm:grid-cols-4">
        <Amount label={t('detail.total')} value={b.total_amount} />
        <Amount label={t('detail.paid')} value={b.paid_amount} />
        <Amount label={t('detail.balance')} value={b.balance_due} emphasis={b.balance_due > 0} />
        <Amount label={t('detail.deposit')} value={b.deposit_due} />
      </div>

      <div className="grid items-start gap-6 xl:grid-cols-3">
        <div className="space-y-6 xl:col-span-2">
          <LinesCard
            establishmentId={establishmentId}
            booking={b}
            canMove={perms.canOperate && active}
            onMove={(lineId) => {
              setMoveLine(lineId);
              setDialog('move');
            }}
          />
          <PaymentsCard
            booking={b}
            canPay={perms.canTakePayments && b.balance_due > 0 && b.status !== 'cancelled'}
            canRefund={perms.canRefund && b.paid_amount > 0}
            onPay={() => setDialog('pay')}
            onRefund={() => setDialog('refund')}
          />
          <AllocationsCard booking={b} spotLabel={(id) => inventory.spotById.get(id)?.label} />
        </div>
        <div className="space-y-6">
          <NotesCard establishmentId={establishmentId} booking={b} canWrite={perms.canWrite} />
          <HistoryCard booking={b} />
        </div>
      </div>

      <CancelBookingDialog
        establishmentId={establishmentId}
        booking={b}
        open={dialog === 'cancel'}
        onOpenChange={(o) => setDialog(o ? 'cancel' : null)}
        canRefund={perms.canRefund}
        onReload={reload}
      />
      <RecordPaymentDialog
        establishmentId={establishmentId}
        booking={b}
        open={dialog === 'pay'}
        onOpenChange={(o) => setDialog(o ? 'pay' : null)}
      />
      <RefundDialog
        establishmentId={establishmentId}
        booking={b}
        open={dialog === 'refund'}
        onOpenChange={(o) => setDialog(o ? 'refund' : null)}
      />
      <ChangeDatesDialog
        establishmentId={establishmentId}
        booking={b}
        inventory={inventory}
        open={dialog === 'dates'}
        onOpenChange={(o) => setDialog(o ? 'dates' : null)}
        onReload={reload}
      />
      <MoveBookingDialog
        establishmentId={establishmentId}
        booking={b}
        inventory={inventory}
        lineId={moveLine}
        open={dialog === 'move'}
        onOpenChange={(o) => setDialog(o ? 'move' : null)}
        onReload={reload}
      />
    </section>
  );
}

function Amount({ label, value, emphasis }: { label: string; value: number; emphasis?: boolean }) {
  return (
    <Card className="gap-1 py-4">
      <CardContent className="px-4">
        <p className="text-sm text-muted-foreground">{label}</p>
        <p
          className={
            emphasis
              ? 'text-xl font-semibold text-destructive tabular-nums'
              : 'text-xl font-semibold tabular-nums'
          }
        >
          {formatCents(value)}
        </p>
      </CardContent>
    </Card>
  );
}

function SectionCard({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle className="text-base">{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      {children}
    </Card>
  );
}

function LinesCard({
  establishmentId,
  booking,
  canMove,
  onMove,
}: {
  establishmentId: string;
  booking: BookingDetail;
  canMove: boolean;
  onMove: (lineId: string) => void;
}) {
  const { t } = useTranslation('bookings');
  const names = useCatalogNames(establishmentId);
  return (
    <SectionCard title={t('lines.title')}>
      <Table>
        <caption className="sr-only">{t('lines.title')}</caption>
        <TableHeader>
          <TableRow>
            <TableHead className="pl-6">{t('lines.spot')}</TableHead>
            <TableHead>{t('lines.pack')}</TableHead>
            <TableHead>{t('lines.extras')}</TableHead>
            <TableHead className="text-right">{t('lines.total')}</TableHead>
            <TableHead className="pr-6">
              <span className="sr-only">{t('lines.actions')}</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {booking.lines.map((l) => (
            <TableRow key={l.id}>
              <TableCell className="pl-6 font-medium">
                {l.spot_label ??
                  (l.assignment_status === 'pending' ? (
                    <StatusBadge tone="warning">{t('lines.pending')}</StatusBadge>
                  ) : (
                    t('lines.quantity', { count: l.quantity })
                  ))}
              </TableCell>
              <TableCell>{names.pack(l.pack_code) || '—'}</TableCell>
              <TableCell className="text-muted-foreground">
                {l.extras.map((x) => `${names.extra(x.extra_code)} ×${x.quantity}`).join(', ') ||
                  '—'}
              </TableCell>
              <TableCell className="text-right tabular-nums">{formatCents(l.line_total)}</TableCell>
              <TableCell className="pr-6 text-right">
                {canMove && l.assignment_status !== 'not_applicable' && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => onMove(l.id)}
                    aria-label={t('move.aria', { label: l.spot_label ?? t('lines.pending') })}
                  >
                    <MoveIcon aria-hidden="true" />
                    {l.assignment_status === 'pending' ? t('move.assign') : t('move.short')}
                  </Button>
                )}
              </TableCell>
            </TableRow>
          ))}
          {booking.extras.map((x) => (
            <TableRow key={x.id}>
              <TableCell className="pl-6 text-muted-foreground" colSpan={3}>
                {t('lines.bookingExtra', { code: x.extra_code ?? '', qty: x.quantity })}
              </TableCell>
              <TableCell className="text-right tabular-nums">{formatCents(x.total)}</TableCell>
              <TableCell className="pr-6" />
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </SectionCard>
  );
}

function PaymentsCard({
  booking,
  canPay,
  canRefund,
  onPay,
  onRefund,
}: {
  booking: BookingDetail;
  canPay: boolean;
  canRefund: boolean;
  onPay: () => void;
  onRefund: () => void;
}) {
  const { t, i18n } = useTranslation('bookings');
  return (
    <SectionCard
      title={t('payments.title')}
      action={
        <div className="flex gap-2">
          {canRefund && (
            <Button size="sm" variant="outline" onClick={onRefund}>
              {t('payments.refund')}
            </Button>
          )}
          {canPay && (
            <Button size="sm" onClick={onPay}>
              <WalletIcon aria-hidden="true" />
              {t('payments.record')}
            </Button>
          )}
        </div>
      }
    >
      {booking.payments.length === 0 ? (
        <EmptyState icon={WalletIcon} bordered={false} title={t('payments.empty')} />
      ) : (
        <Table>
          <caption className="sr-only">{t('payments.title')}</caption>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-6">{t('payments.when')}</TableHead>
              <TableHead>{t('fields.method')}</TableHead>
              <TableHead>{t('payments.status')}</TableHead>
              <TableHead className="pr-6 text-right">{t('fields.amount')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {booking.payments.map((p) => (
              <TableRow key={p.id}>
                <TableCell className="pl-6 tabular-nums">
                  {new Date(p.created_at).toLocaleString(i18n.language, {
                    dateStyle: 'short',
                    timeStyle: 'short',
                  })}
                </TableCell>
                <TableCell>
                  {t(`method.${p.method}`)}
                  {p.direction === 'refund' && (
                    <Badge variant="outline" className="ml-2">
                      {t('payments.refundBadge')}
                    </Badge>
                  )}
                </TableCell>
                <TableCell>
                  <StatusBadge
                    tone={
                      p.status === 'succeeded'
                        ? 'success'
                        : p.status === 'pending'
                          ? 'warning'
                          : 'danger'
                    }
                  >
                    {t(`payments.statuses.${p.status}`)}
                  </StatusBadge>
                </TableCell>
                <TableCell className="pr-6 text-right tabular-nums">
                  {p.direction === 'refund' ? `−${formatCents(p.amount)}` : formatCents(p.amount)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </SectionCard>
  );
}

function AllocationsCard({
  booking,
  spotLabel,
}: {
  booking: BookingDetail;
  spotLabel: (id: string) => string | undefined;
}) {
  const { t } = useTranslation('bookings');
  const rows = [...booking.allocations].sort(
    (a, b) => a.date.localeCompare(b.date) || a.slot.localeCompare(b.slot),
  );
  return (
    <SectionCard
      title={t('allocations.title')}
      description={t('allocations.description', { count: rows.length })}
    >
      {rows.length === 0 ? (
        <EmptyState bordered={false} title={t('allocations.empty')} />
      ) : (
        <div className="max-h-80 overflow-y-auto">
          <Table>
            <caption className="sr-only">{t('allocations.title')}</caption>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-6">{t('allocations.date')}</TableHead>
                <TableHead>{t('allocations.slot')}</TableHead>
                <TableHead>{t('allocations.resource')}</TableHead>
                <TableHead className="pr-6">{t('allocations.kind')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((a, i) => (
                <TableRow key={`${a.date}-${a.slot}-${a.spot_id ?? a.pool_id}-${i}`}>
                  <TableCell className="pl-6 tabular-nums">{formatIsoDate(a.date)}</TableCell>
                  <TableCell>{t(`slots.${a.slot}`)}</TableCell>
                  <TableCell>
                    {a.spot_id
                      ? (spotLabel(a.spot_id) ?? '—')
                      : t('allocations.pool', { count: a.quantity ?? 1 })}
                  </TableCell>
                  <TableCell className="pr-6">{t(`allocations.kinds.${a.kind}`)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </SectionCard>
  );
}

function NotesCard({
  establishmentId,
  booking,
  canWrite,
}: {
  establishmentId: string;
  booking: BookingDetail;
  canWrite: boolean;
}) {
  const { t } = useTranslation('bookings');
  const add = useAddBookingNote(establishmentId);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const form = useForm<NoteValues>({
    resolver: zodResolver(noteSchema),
    defaultValues: { text: '', visibility: 'internal' },
  });
  const visibility = useWatch({ control: form.control, name: 'visibility' });
  const onSubmit = form.handleSubmit(async (v) => {
    setFailure(null);
    try {
      await add.mutateAsync({ bookingId: booking.id, note: v });
      form.reset({ text: '', visibility: v.visibility });
    } catch (err) {
      setFailure(describeProblem(err, t));
    }
  });
  return (
    <SectionCard title={t('notes.title')}>
      <CardContent className="space-y-4 py-4">
        <div className="space-y-1 text-sm">
          <p className="font-medium">{t('notes.internal')}</p>
          <p className="whitespace-pre-line text-muted-foreground">
            {booking.notes_internal || '—'}
          </p>
        </div>
        <div className="space-y-1 text-sm">
          <p className="font-medium">{t('notes.customer')}</p>
          <p className="whitespace-pre-line text-muted-foreground">
            {booking.notes_customer || '—'}
          </p>
        </div>
        {canWrite && (
          <Form {...form}>
            <form noValidate onSubmit={(e) => void onSubmit(e)} className="space-y-3">
              <ProblemAlert problem={failure} />
              <FieldRow control={form.control} name="text" label={t('notes.add')}>
                {(field) => <Input {...field} />}
              </FieldRow>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <ToggleGroup
                  type="single"
                  size="sm"
                  variant="outline"
                  aria-label={t('notes.visibility')}
                  value={visibility}
                  onValueChange={(v) =>
                    v && form.setValue('visibility', v as NoteValues['visibility'])
                  }
                >
                  <ToggleGroupItem value="internal">{t('notes.internal')}</ToggleGroupItem>
                  <ToggleGroupItem value="customer">{t('notes.customer')}</ToggleGroupItem>
                </ToggleGroup>
                <Button type="submit" size="sm" disabled={add.isPending}>
                  {t('notes.save')}
                </Button>
              </div>
            </form>
          </Form>
        )}
      </CardContent>
    </SectionCard>
  );
}

function HistoryCard({ booking }: { booking: BookingDetail }) {
  const { t, i18n } = useTranslation('bookings');
  const entries = [...booking.history].sort((a, b) => b.at.localeCompare(a.at));
  return (
    <SectionCard title={t('history.title')}>
      {entries.length === 0 ? (
        <EmptyState icon={HistoryIcon} bordered={false} title={t('history.empty')} />
      ) : (
        <CardContent className="py-4">
          <ol className="space-y-4 border-l pl-4" aria-label={t('history.title')}>
            {entries.map((e, i) => (
              <li key={`${e.at}-${i}`} className="relative">
                <span
                  aria-hidden="true"
                  className="absolute top-1.5 -left-5.5 size-3 rounded-full border-2 border-background bg-primary"
                />
                <p className="text-sm font-medium">
                  {t(`history.actions.${e.action}`, { defaultValue: e.action })}
                </p>
                <p className="text-xs text-muted-foreground">
                  {new Date(e.at).toLocaleString(i18n.language, {
                    dateStyle: 'short',
                    timeStyle: 'short',
                  })}
                  {' · '}
                  {e.actor_name ?? t(`history.actor.${e.actor_type}`)}
                </p>
                {typeof e.details?.text === 'string' && (
                  <p className="mt-1 text-sm whitespace-pre-line text-muted-foreground">
                    {e.details.text}
                  </p>
                )}
              </li>
            ))}
          </ol>
        </CardContent>
      )}
    </SectionCard>
  );
}
