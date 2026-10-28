import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import {
  BackLink,
  ConfirmDialog,
  DataTable,
  EmptyState,
  Page,
  QueryState,
  SectionCard,
  StatCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link, useNavigate, useParams } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  BanIcon,
  CalendarIcon,
  DownloadIcon,
  FileSpreadsheetIcon,
  HandshakeIcon,
  PencilIcon,
  PrinterIcon,
  ReceiptTextIcon,
  Trash2Icon,
  WalletIcon,
} from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  useCurrentEstablishment,
  useInstantFormat,
  useRequiredEstablishmentId,
} from '../../establishment';
import { STATUS_TONE } from '../bookings/model';
import { todayIn } from '../operations/lib/dates';
import { agreements as agreementsResource, type Agreement } from '../pricing/api';
import {
  partners,
  statementUrl,
  usePartner,
  usePartnerStatement,
  type Partner,
  type PartnerPayment,
  type PartnerStatementBooking,
} from './api';
import { describeM8Problem, usePartnerPermissions } from './lib';
import { PartnerFormSheet } from './PartnerForm';
import { RecordPartnerPaymentDialog, VoidPartnerPaymentDialog } from './PaymentDialogs';
import { usePeriodPicker } from './PeriodPicker';

/**
 * Scheda partner (spec 30.2.4): profile, agreements addressed to the partner
 * and the partner account: statement of the period (bookings charged to the
 * partner, payments received, opening and closing balance, INV-104), record
 * and void payments, Excel and printable (PDF) export for the invoice, which
 * stays outside Arenile.
 */
export function PartnerDetailPage() {
  const { t } = useTranslation('partners');
  const establishmentId = useRequiredEstablishmentId();
  const { partnerId } = useParams({ strict: false }) as { partnerId?: string };
  const navigate = useNavigate();
  const perms = usePartnerPermissions();
  const query = usePartner(establishmentId, partnerId);
  const remove = partners.useRemove(establishmentId);
  const { establishment } = useCurrentEstablishment();
  const today = todayIn(establishment?.timezone);
  const [editing, setEditing] = useState(false);
  const [recording, setRecording] = useState(false);

  const back = (
    <BackLink>
      <Link to="/e/$establishmentId/convenzioni" params={{ establishmentId }}>
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
  const p = query.data;

  return (
    <Page
      eyebrow={back}
      title={
        <span className="flex flex-wrap items-center gap-3">
          {p.name}
          <Badge variant="outline">{t(`kinds.${p.kind}`)}</Badge>
          {!p.active && <StatusBadge tone="neutral">{t('inactive')}</StatusBadge>}
        </span>
      }
      description={p.municipality || undefined}
      actions={
        <>
          {perms.canConfigure && (
            <Button variant="outline" onClick={() => setEditing(true)}>
              <PencilIcon aria-hidden="true" />
              {t('common.actions.edit', { ns: 'translation' })}
            </Button>
          )}
          {perms.canTakePayments && (
            <Button onClick={() => setRecording(true)}>
              <WalletIcon aria-hidden="true" />
              {t('payments.record')}
            </Button>
          )}
          {perms.canConfigure && (
            <ConfirmDialog
              title={t('common.actions.confirmDelete', { ns: 'translation' })}
              description={t('detail.deleteConfirm', { name: p.name })}
              confirmLabel={t('common.actions.delete', { ns: 'translation' })}
              destructive
              errorText={(err) => describeM8Problem(err, t).title}
              onConfirm={async () => {
                await remove.mutateAsync({ id: p.id, version: p.version });
                toast.success(t('detail.deleted', { name: p.name }));
                void navigate({
                  to: '/e/$establishmentId/convenzioni',
                  params: { establishmentId },
                });
              }}
              trigger={
                <Button variant="ghost" className="text-destructive">
                  <Trash2Icon aria-hidden="true" />
                  {t('common.actions.delete', { ns: 'translation' })}
                </Button>
              }
            />
          )}
        </>
      }
    >
      <div className="grid items-start gap-6 2xl:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-6">
          {perms.canReport ? (
            <StatementSection establishmentId={establishmentId} partner={p} />
          ) : (
            <EmptyState
              icon={ReceiptTextIcon}
              title={t('detail.statement')}
              description={t('statement.forbidden')}
            />
          )}
        </div>
        <div className="min-w-0 space-y-6">
          <ProfileCard partner={p} />
          <PartnerAgreementsCard establishmentId={establishmentId} partnerId={p.id} />
        </div>
      </div>
      <PartnerFormSheet
        establishmentId={establishmentId}
        partner={p}
        open={editing}
        onOpenChange={setEditing}
      />
      <RecordPartnerPaymentDialog
        establishmentId={establishmentId}
        partner={p}
        today={today}
        open={recording}
        onOpenChange={setRecording}
      />
    </Page>
  );
}

function StatementSection({
  establishmentId,
  partner,
}: {
  establishmentId: string;
  partner: Partner;
}) {
  const { t } = useTranslation('partners');
  const { t: tb } = useTranslation('bookings');
  const fmt = useInstantFormat();
  const perms = usePartnerPermissions();
  const { period, picker } = usePeriodPicker(establishmentId, 'statement');
  const statement = usePartnerStatement(establishmentId, partner.id, period);
  const [voiding, setVoiding] = useState<PartnerPayment | null>(null);
  const s = statement.data;
  const totals = s?.totals;

  const bookingColumns: ColumnDef<PartnerStatementBooking>[] = [
    {
      id: 'code',
      header: t('statement.booking'),
      meta: { label: t('statement.booking') },
      cell: ({ row: { original: b } }) => (
        <span className="flex flex-wrap items-center gap-2">
          <Link
            to="/e/$establishmentId/prenotazioni/$bookingId"
            params={{ establishmentId, bookingId: b.booking_id }}
            className="font-mono font-medium text-primary-on-surface underline-offset-4 hover:underline"
          >
            {b.code}
          </Link>
          {b.status === 'cancelled' && (
            <StatusBadge tone={STATUS_TONE[b.status]}>{tb(`status.${b.status}`)}</StatusBadge>
          )}
        </span>
      ),
    },
    {
      id: 'guest',
      header: t('statement.guest'),
      meta: { label: t('statement.guest') },
      cell: ({ row: { original: b } }) => (
        <span>
          {b.customer_display_name || '—'}
          {b.group_name && (
            <span className="block text-xs text-muted-foreground">{b.group_name}</span>
          )}
        </span>
      ),
    },
    {
      id: 'stay',
      header: t('statement.stay'),
      meta: { label: t('statement.stay') },
      cell: ({ row: { original: b } }) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatIsoDate(b.starts_on)}
          {b.ends_on !== b.starts_on && ` → ${formatIsoDate(b.ends_on)}`}
        </span>
      ),
    },
    {
      accessorKey: 'spot_days',
      header: t('statement.spotDays'),
      meta: { label: t('statement.spotDays'), align: 'right' },
      footer: () => totals?.spot_days ?? '',
    },
    {
      accessorKey: 'people',
      header: t('statement.people'),
      meta: { label: t('statement.people'), align: 'right' },
      footer: () => totals?.people ?? '',
    },
    {
      id: 'total',
      header: t('statement.total'),
      meta: { label: t('statement.total'), align: 'right' },
      cell: ({ row }) => formatCents(row.original.total_amount),
      footer: () => (totals ? formatCents(totals.total_amount) : ''),
    },
    {
      id: 'discount',
      header: t('statement.discount'),
      meta: { label: t('statement.discount'), align: 'right' },
      cell: ({ row }) => formatCents(row.original.discount_amount),
      footer: () => (totals ? formatCents(totals.discount_amount) : ''),
    },
    {
      id: 'partner',
      header: t('statement.partnerAmount'),
      meta: { label: t('statement.partnerAmount'), align: 'right' },
      cell: ({ row }) => (
        <span className="font-medium">{formatCents(row.original.partner_amount)}</span>
      ),
      footer: () => (totals ? formatCents(totals.charged) : ''),
    },
  ];

  const paymentColumns: ColumnDef<PartnerPayment>[] = [
    {
      id: 'date',
      header: t('payments.receivedOn'),
      meta: { label: t('payments.receivedOn') },
      cell: ({ row }) => (
        <span className="tabular-nums">{formatIsoDate(row.original.received_on)}</span>
      ),
    },
    {
      id: 'method',
      header: t('payments.method'),
      meta: { label: t('payments.method') },
      cell: ({ row: { original: x } }) => (
        <span className="flex flex-wrap items-center gap-2">
          {t(`methods.${x.method}`)}
          {x.voided_at && <StatusBadge tone="neutral">{t('payments.voidedBadge')}</StatusBadge>}
        </span>
      ),
    },
    {
      id: 'reference',
      header: t('payments.reference'),
      meta: { label: t('payments.reference') },
      cell: ({ row: { original: x } }) => (
        <span className="text-muted-foreground">
          {x.voided_at
            ? t('payments.voidedOn', { at: fmt.date(x.voided_at), reason: x.void_reason ?? '' })
            : x.reference || x.note || '—'}
        </span>
      ),
    },
    {
      id: 'amount',
      header: t('payments.amount'),
      meta: { label: t('payments.amount'), align: 'right' },
      cell: ({ row: { original: x } }) => (
        <span className={x.voided_at ? 'text-muted-foreground line-through' : undefined}>
          {formatCents(x.amount)}
        </span>
      ),
      footer: () => (totals ? formatCents(totals.paid) : ''),
    },
    {
      id: 'actions',
      header: () => <span className="sr-only">{t('actionsColumn')}</span>,
      meta: { label: t('actionsColumn'), align: 'right' },
      cell: ({ row: { original: x } }) =>
        perms.canTakePayments && !x.voided_at ? (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setVoiding(x)}
            aria-label={t('payments.voidAria', { amount: formatCents(x.amount) })}
          >
            <BanIcon aria-hidden="true" />
            {t('payments.void')}
          </Button>
        ) : null,
    },
  ];

  const exportMenu = period && (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <DownloadIcon aria-hidden="true" />
          {t('statement.export')}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem asChild>
          <a
            href={statementUrl(establishmentId, partner.id, period, 'xlsx')}
            download={`arenile_partner_${period.from}_${period.to}.xlsx`}
          >
            <FileSpreadsheetIcon aria-hidden="true" />
            {t('statement.xlsx')}
          </a>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <a
            href={statementUrl(establishmentId, partner.id, period, 'html')}
            target="_blank"
            rel="noopener"
          >
            <PrinterIcon aria-hidden="true" />
            {t('statement.pdf')}
          </a>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-2">
        {picker}
        {exportMenu}
      </div>
      {statement.error ? (
        <QueryState
          isLoading={false}
          error={statement.error}
          onRetry={() => void statement.refetch()}
        />
      ) : (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            label={t('statement.opening')}
            value={totals ? formatCents(totals.opening_balance) : '—'}
          />
          <StatCard
            label={t('statement.charged')}
            value={totals ? formatCents(totals.charged) : '—'}
          />
          <StatCard label={t('statement.paid')} value={totals ? formatCents(totals.paid) : '—'} />
          <StatCard
            label={t('statement.closing')}
            value={totals ? formatCents(totals.closing_balance) : '—'}
            tone={totals && totals.closing_balance > 0 ? 'danger' : 'default'}
            hint={t('statement.closingHint')}
          />
        </div>
      )}
      <SectionCard
        flush
        icon={CalendarIcon}
        title={t('statement.bookings')}
        description={totals ? t('statement.bookingsCount', { count: totals.bookings }) : undefined}
      >
        <DataTable
          embedded
          caption={t('statement.bookings')}
          loading={statement.isPending && !!period}
          columns={bookingColumns}
          data={s?.bookings ?? []}
          getRowId={(b) => b.booking_id}
          columnToggle={false}
          empty={
            <EmptyState icon={CalendarIcon} bordered={false} title={t('statement.noBookings')} />
          }
        />
      </SectionCard>
      <SectionCard flush icon={WalletIcon} title={t('payments.title')}>
        <DataTable
          embedded
          caption={t('payments.title')}
          loading={statement.isPending && !!period}
          columns={paymentColumns}
          data={s?.payments ?? []}
          getRowId={(x) => x.id}
          columnToggle={false}
          pageSize={0}
          empty={<EmptyState icon={WalletIcon} bordered={false} title={t('payments.empty')} />}
        />
      </SectionCard>
      <VoidPartnerPaymentDialog
        establishmentId={establishmentId}
        partnerId={partner.id}
        payment={voiding}
        onOpenChange={(o) => !o && setVoiding(null)}
      />
    </div>
  );
}

function ProfileCard({ partner: p }: { partner: Partner }) {
  const { t } = useTranslation('partners');
  return (
    <SectionCard title={t('detail.profile')}>
      <dl className="space-y-4 text-sm">
        {p.accommodation_type && (
          <Field label={t('fields.accommodationType')}>
            {t(`accommodation.${p.accommodation_type}`)}
          </Field>
        )}
        <Field label={t('fields.address')}>
          {[p.address, p.municipality].filter(Boolean).join(', ') || '—'}
        </Field>
        <Field label={t('fields.contactName')}>{p.contact_name || '—'}</Field>
        <Field label={t('fields.phone')}>
          {p.phone ? (
            <a className="text-primary-on-surface tabular-nums" href={`tel:${p.phone}`}>
              {p.phone}
            </a>
          ) : (
            '—'
          )}
        </Field>
        <Field label={t('fields.email')}>
          {p.email ? (
            <a className="break-all text-primary-on-surface" href={`mailto:${p.email}`}>
              {p.email}
            </a>
          ) : (
            '—'
          )}
        </Field>
        <Field label={t('fields.vatNumber')}>{p.vat_number || '—'}</Field>
        <Field label={t('fields.taxCode')}>{p.tax_code || '—'}</Field>
        <Field label={t('fields.notes')}>
          <span className="whitespace-pre-line">{p.notes || '—'}</span>
        </Field>
      </dl>
    </SectionCard>
  );
}

function PartnerAgreementsCard({
  establishmentId,
  partnerId,
}: {
  establishmentId: string;
  partnerId: string;
}) {
  const { t } = useTranslation('partners');
  const list = agreementsResource.useList(establishmentId);
  const items = (list.data ?? []).filter((a: Agreement) => a.partner_id === partnerId);
  return (
    <SectionCard icon={HandshakeIcon} title={t('detail.agreements')}>
      {list.isPending || list.error ? (
        <QueryState
          isLoading={list.isPending}
          error={list.error}
          onRetry={() => void list.refetch()}
        />
      ) : items.length === 0 ? (
        <EmptyState
          icon={HandshakeIcon}
          bordered={false}
          title={t('detail.noAgreements')}
          action={
            <Button variant="outline" asChild>
              <Link to="/e/$establishmentId/pricing" params={{ establishmentId }}>
                {t('agreementsLink')}
              </Link>
            </Button>
          }
        />
      ) : (
        <ul className="space-y-3 text-sm">
          {items.map((a) => (
            <li key={a.id} className="space-y-1">
              <p className="flex flex-wrap items-center gap-1.5 font-medium">
                {a.name}
                {a.code && (
                  <Badge variant="outline" className="font-mono">
                    {a.code}
                  </Badge>
                )}
                {!a.active && <StatusBadge tone="neutral">{t('inactive')}</StatusBadge>}
              </p>
              {a.public_link && (
                <p className="break-all text-xs text-muted-foreground">{a.public_link}</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </SectionCard>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="font-medium">{label}</dt>
      <dd className="text-muted-foreground">{children}</dd>
    </div>
  );
}
