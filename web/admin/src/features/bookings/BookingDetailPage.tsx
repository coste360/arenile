import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Form } from '@arenile/shared/ui/form';
import { Input } from '@arenile/shared/ui/input';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  BackLink,
  ConfirmDialog,
  DataTable,
  EmptyState,
  FieldRow,
  Page,
  QueryState,
  SectionCard,
  StatCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link, useParams } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  BanIcon,
  CalendarRangeIcon,
  CheckIcon,
  HistoryIcon,
  LayersIcon,
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
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
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
import { RefundRequestsCard } from './detail/RefundRequestsCard';
import {
  ACTIVE_STATUSES,
  availableActions,
  STATUS_TONE,
  todayIso,
  useOperationalPermissions,
  type BookingAction,
} from './model';
import { describeProblem, type ProblemText } from './problem';
import { ProblemAlert } from './ProblemAlert';
import { useEstablishmentSpots } from './spots';
import { useCatalogNames } from './usePackName';

type DialogName = 'cancel' | 'pay' | 'refund' | 'dates' | 'move' | null;
type Line = BookingDetail['lines'][number];
type BookingExtra = BookingDetail['extras'][number];
type Payment = BookingDetail['payments'][number];
type Allocation = BookingDetail['allocations'][number];

/** One row of the "Righe" table: a spot/pool line or an extra on the whole booking. */
type LineRow = { kind: 'line'; line: Line } | { kind: 'extra'; extra: BookingExtra };

const ACTION_ICONS: Record<BookingAction, typeof CheckIcon> = {
  confirm: CheckIcon,
  approve: CheckIcon,
  reject: ThumbsDownIcon,
  'check-in': LogInIcon,
  'no-show': UserXIcon,
  cancel: BanIcon,
};

const PRIMARY_ACTIONS: BookingAction[] = ['confirm', 'approve', 'check-in'];

/** Header order (UI standard): destructive first, secondary outline, primary last. */
function actionRank(a: BookingAction): number {
  if (a === 'cancel') return 0;
  return PRIMARY_ACTIONS.includes(a) ? 2 : 1;
}

/** Prenotazione: header with status and actions, lines, allocations, payments, history, notes. */
export function BookingDetailPage() {
  const { t, i18n } = useTranslation('bookings');
  const establishmentId = useRequiredEstablishmentId();
  const { bookingId } = useParams({ strict: false }) as { bookingId?: string };
  const query = useBooking(establishmentId, bookingId);
  const inventory = useEstablishmentSpots(establishmentId, i18n.language);
  const fmt = useInstantFormat();
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
    <BackLink>
      <Link to="/e/$establishmentId/prenotazioni" params={{ establishmentId }}>
        <ArrowLeftIcon aria-hidden="true" />
        {t('detail.back')}
      </Link>
    </BackLink>
  );

  if (!query.data) {
    return (
      <Page eyebrow={back} title={t('detail.title')}>
        <QueryState
          isLoading={query.isPending}
          error={query.error}
          onRetry={() => void query.refetch()}
          rows={6}
        />
      </Page>
    );
  }

  const b = query.data;
  const actions = availableActions(b.status, b.next_statuses, {
    startsOn: b.starts_on,
    endsOn: b.ends_on,
    today: todayIso(),
  })
    .filter((a) => {
      if (a === 'check-in' || a === 'no-show') return perms.canOperate;
      // Approval rule R3: approving, and confirming a booking awaiting
      // approval, is the manager's decision (establishment.configure).
      if (a === 'approve' || (a === 'confirm' && b.status === 'awaiting_approval')) {
        return perms.canApprove;
      }
      return perms.canWrite;
    })
    .sort((x, y) => actionRank(x) - actionRank(y));
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
  const canChangeDates = perms.canWrite && active && !b.is_season_pass;

  const actionButton = (a: BookingAction) => {
    const Icon = ACTION_ICONS[a];
    if (a === 'cancel') {
      return (
        <Button key={a} variant="destructive" onClick={() => setDialog('cancel')}>
          <Icon aria-hidden="true" />
          {t('actions.cancel')}
        </Button>
      );
    }
    return (
      <ConfirmDialog
        key={a}
        title={t(`actions.confirmTitle.${a}`, { code: b.code })}
        description={t(`actions.confirmText.${a}`)}
        confirmLabel={t(`actions.${a}`)}
        destructive={a === 'reject' || a === 'no-show'}
        onConfirm={() => run(a)}
        trigger={
          <Button variant={PRIMARY_ACTIONS.includes(a) ? 'default' : 'outline'}>
            <Icon aria-hidden="true" />
            {t(`actions.${a}`)}
          </Button>
        }
      />
    );
  };

  return (
    <Page
      eyebrow={back}
      title={
        <span className="flex flex-wrap items-center gap-3">
          {b.code}
          <StatusBadge tone={STATUS_TONE[b.status]}>{t(`status.${b.status}`)}</StatusBadge>
          <Badge variant="outline">
            {b.is_season_pass ? t('detail.pass') : t(`channel.${b.channel}`)}
          </Badge>
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
        (actions.length > 0 || canChangeDates) && (
          <>
            {actions.filter((a) => actionRank(a) < 2).map(actionButton)}
            {canChangeDates && (
              <Button variant="outline" onClick={() => setDialog('dates')}>
                <CalendarRangeIcon aria-hidden="true" />
                {t('actions.dates')}
              </Button>
            )}
            {actions.filter((a) => actionRank(a) === 2).map(actionButton)}
          </>
        )
      }
      notice={
        <>
          <ProblemAlert problem={failure} onReload={reload} />
          {b.hold_expires_at && (
            <p className="text-sm text-muted-foreground">
              {t('detail.holdExpires', {
                at: fmt.dateTime(b.hold_expires_at),
              })}
            </p>
          )}
        </>
      }
    >
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label={t('detail.total')} value={formatCents(b.total_amount)} />
        <StatCard label={t('detail.paid')} value={formatCents(b.paid_amount)} />
        <StatCard
          label={t('detail.balance')}
          value={formatCents(b.balance_due)}
          tone={b.balance_due > 0 ? 'danger' : 'default'}
        />
        <StatCard label={t('detail.deposit')} value={formatCents(b.deposit_due)} />
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-6">
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
          <RefundRequestsCard
            establishmentId={establishmentId}
            booking={b}
            canRetry={perms.canRefund}
          />
          <AllocationsCard booking={b} spotLabel={(id) => inventory.spotById.get(id)?.label} />
        </div>
        <div className="min-w-0 space-y-6">
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
    </Page>
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
  const rows: LineRow[] = [
    ...booking.lines.map((line): LineRow => ({ kind: 'line', line })),
    ...booking.extras.map((extra): LineRow => ({ kind: 'extra', extra })),
  ];
  const columns: ColumnDef<LineRow>[] = [
    {
      id: 'spot',
      header: t('lines.spot'),
      meta: { label: t('lines.spot') },
      cell: ({ row: { original: r } }) => {
        if (r.kind === 'extra') {
          return (
            <span className="text-muted-foreground">
              {t('lines.bookingExtra', { code: r.extra.extra_code ?? '', qty: r.extra.quantity })}
            </span>
          );
        }
        const l = r.line;
        return (
          <span className="font-medium">
            {l.spot_label ??
              (l.assignment_status === 'pending' ? (
                <StatusBadge tone="warning">{t('lines.pending')}</StatusBadge>
              ) : (
                t('lines.quantity', { count: l.quantity })
              ))}
          </span>
        );
      },
    },
    {
      id: 'pack',
      header: t('lines.pack'),
      meta: { label: t('lines.pack') },
      cell: ({ row: { original: r } }) =>
        r.kind === 'line' ? names.pack(r.line.pack_code) || '—' : null,
    },
    {
      id: 'extras',
      header: t('lines.extras'),
      meta: { label: t('lines.extras') },
      cell: ({ row: { original: r } }) =>
        r.kind === 'line' ? (
          <span className="text-muted-foreground">
            {r.line.extras.map((x) => `${names.extra(x.extra_code)} ×${x.quantity}`).join(', ') ||
              '—'}
          </span>
        ) : null,
    },
    {
      id: 'total',
      header: t('lines.total'),
      meta: { label: t('lines.total'), align: 'right' },
      cell: ({ row: { original: r } }) =>
        formatCents(r.kind === 'line' ? r.line.line_total : r.extra.total),
    },
    {
      id: 'actions',
      header: () => <span className="sr-only">{t('lines.actions')}</span>,
      meta: { label: t('lines.actions'), align: 'right' },
      cell: ({ row: { original: r } }) => {
        if (r.kind !== 'line') return null;
        const l = r.line;
        return canMove && l.assignment_status !== 'not_applicable' ? (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onMove(l.id)}
            aria-label={t('move.aria', { label: l.spot_label ?? t('lines.pending') })}
          >
            <MoveIcon aria-hidden="true" />
            {l.assignment_status === 'pending' ? t('move.assign') : t('move.short')}
          </Button>
        ) : null;
      },
    },
  ];
  return (
    <SectionCard flush title={t('lines.title')}>
      <DataTable
        embedded
        caption={t('lines.title')}
        columns={columns}
        data={rows}
        getRowId={(r) => (r.kind === 'line' ? r.line.id : `extra-${r.extra.id}`)}
        columnToggle={false}
        pageSize={0}
        empty={
          <EmptyState
            icon={LayersIcon}
            bordered={false}
            title={t('ui.table.empty', { ns: 'translation' })}
          />
        }
      />
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
  const { t } = useTranslation('bookings');
  const fmt = useInstantFormat();
  const columns: ColumnDef<Payment>[] = [
    {
      id: 'when',
      header: t('payments.when'),
      meta: { label: t('payments.when') },
      cell: ({ row }) => (
        <span className="tabular-nums">{fmt.dateTime(row.original.created_at)}</span>
      ),
    },
    {
      id: 'method',
      header: t('fields.method'),
      meta: { label: t('fields.method') },
      cell: ({ row: { original: p } }) => (
        <span className="flex items-center gap-2">
          {t(`method.${p.method}`)}
          {p.direction === 'refund' && <Badge variant="outline">{t('payments.refundBadge')}</Badge>}
        </span>
      ),
    },
    {
      id: 'status',
      header: t('payments.status'),
      meta: { label: t('payments.status') },
      cell: ({ row: { original: p } }) => (
        <StatusBadge
          tone={
            p.status === 'succeeded' ? 'success' : p.status === 'pending' ? 'warning' : 'danger'
          }
        >
          {t(`payments.statuses.${p.status}`)}
        </StatusBadge>
      ),
    },
    {
      id: 'amount',
      header: t('fields.amount'),
      meta: { label: t('fields.amount'), align: 'right' },
      cell: ({ row: { original: p } }) =>
        p.direction === 'refund' ? `−${formatCents(p.amount)}` : formatCents(p.amount),
    },
  ];
  return (
    <SectionCard
      flush
      title={t('payments.title')}
      actions={
        (canRefund || canPay) && (
          <>
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
          </>
        )
      }
    >
      <DataTable
        embedded
        caption={t('payments.title')}
        columns={columns}
        data={booking.payments}
        getRowId={(p) => p.id}
        columnToggle={false}
        pageSize={0}
        empty={<EmptyState icon={WalletIcon} bordered={false} title={t('payments.empty')} />}
      />
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
  const columns: ColumnDef<Allocation>[] = [
    {
      id: 'date',
      header: t('allocations.date'),
      meta: { label: t('allocations.date') },
      cell: ({ row }) => <span className="tabular-nums">{formatIsoDate(row.original.date)}</span>,
    },
    {
      id: 'slot',
      header: t('allocations.slot'),
      meta: { label: t('allocations.slot') },
      cell: ({ row }) => t(`slots.${row.original.slot}`),
    },
    {
      id: 'resource',
      header: t('allocations.resource'),
      meta: { label: t('allocations.resource') },
      cell: ({ row: { original: a } }) =>
        a.spot_id
          ? (spotLabel(a.spot_id) ?? '—')
          : t('allocations.pool', { count: a.quantity ?? 1 }),
    },
    {
      id: 'kind',
      header: t('allocations.kind'),
      meta: { label: t('allocations.kind') },
      cell: ({ row }) => t(`allocations.kinds.${row.original.kind}`),
    },
  ];
  return (
    <SectionCard
      flush
      title={t('allocations.title')}
      description={t('allocations.description', { count: rows.length })}
    >
      {/* Long stays (season passes: ~300 rows) scroll inside the card; the
          region is focusable so the keyboard can scroll it too (WCAG 2.1.1). */}
      <div
        tabIndex={0}
        role="region"
        aria-label={t('allocations.title')}
        className="max-h-80 overflow-y-auto rounded-b-xl focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
      >
        <DataTable
          embedded
          caption={t('allocations.title')}
          columns={columns}
          data={rows}
          getRowId={(a, i) => `${a.date}-${a.slot}-${a.spot_id ?? a.pool_id}-${i}`}
          columnToggle={false}
          pageSize={0}
          empty={<EmptyState bordered={false} title={t('allocations.empty')} />}
        />
      </div>
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
      <div className="space-y-4">
        <NoteText label={t('notes.internal')}>{booking.notes_internal || '—'}</NoteText>
        <NoteText label={t('notes.customer')}>{booking.notes_customer || '—'}</NoteText>
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
      </div>
    </SectionCard>
  );
}

function NoteText({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1 text-sm">
      <p className="font-medium">{label}</p>
      <p className="whitespace-pre-line text-muted-foreground">{children}</p>
    </div>
  );
}

function HistoryCard({ booking }: { booking: BookingDetail }) {
  const { t } = useTranslation('bookings');
  const fmt = useInstantFormat();
  const entries = [...booking.history].sort((a, b) => b.at.localeCompare(a.at));
  return (
    <SectionCard title={t('history.title')}>
      {entries.length === 0 ? (
        <EmptyState icon={HistoryIcon} bordered={false} title={t('history.empty')} />
      ) : (
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
                {fmt.dateTime(e.at)}
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
      )}
    </SectionCard>
  );
}
