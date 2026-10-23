import type { Schemas } from '@arenile/shared/api';
import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Button } from '@arenile/shared/ui/button';
import {
  DataTable,
  EmptyState,
  QueryState,
  SectionCard,
  StatCard,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { CalendarClockIcon, CoinsIcon, HandCoinsIcon, PlusIcon, WalletIcon } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInstantFormat } from '../../establishment';
import { useOperationalPermissions } from '../bookings/model';
import { useCustomerCredits, type CustomerCredit } from './api';
import { ApplyCreditDialog, GrantCreditDialog } from './CreditDialogs';
import { payableBookings } from './lib';

export interface CustomerCreditsSectionProps {
  establishmentId: string;
  customerId: string;
  /** Recent bookings of the customer, for "Usa su una prenotazione". */
  bookings?: Schemas['BookingSummary'][];
  /** Anonymised customers keep the ledger read-only. */
  readOnly?: boolean;
}

/**
 * Credit ledger of a customer (spec 13.4): available balance and the next
 * expiry as KPIs, movements (grants, uses, expiries, adjustments),
 * "Riconosci credito" (refunds.issue) and "Usa su una prenotazione" (a
 * payment with method `credit`, cash.operate).
 */
export function CustomerCreditsSection({
  establishmentId,
  customerId,
  bookings = [],
  readOnly,
}: CustomerCreditsSectionProps) {
  const { t } = useTranslation('credits');
  const fmt = useInstantFormat();
  const perms = useOperationalPermissions();
  const ledger = useCustomerCredits(establishmentId, customerId);
  const [granting, setGranting] = useState(false);
  const [applying, setApplying] = useState(false);
  const first = ledger.data?.pages[0];
  const items = useMemo(() => ledger.data?.pages.flatMap((p) => p.items) ?? [], [ledger.data]);
  const available = first?.available ?? 0;
  const nextExpiry = [...(first?.expiring ?? [])].sort((a, b) =>
    a.expires_on.localeCompare(b.expires_on),
  )[0];
  const payable = payableBookings(bookings);

  const columns: ColumnDef<CustomerCredit>[] = [
    {
      id: 'date',
      header: t('columns.date'),
      meta: { label: t('columns.date') },
      cell: ({ row }) => <span className="tabular-nums">{fmt.date(row.original.created_at)}</span>,
    },
    {
      id: 'source',
      header: t('columns.movement'),
      meta: { label: t('columns.movement') },
      cell: ({ row: { original: c } }) => (
        <span className="flex flex-col">
          <span>{t(`source.${c.source}`)}</span>
          {c.note && <span className="text-xs text-muted-foreground">{c.note}</span>}
        </span>
      ),
    },
    {
      id: 'booking',
      header: t('columns.booking'),
      meta: { label: t('columns.booking') },
      cell: ({ row: { original: c } }) =>
        c.booking_id ? (
          <Link
            to="/e/$establishmentId/prenotazioni/$bookingId"
            params={{ establishmentId, bookingId: c.booking_id }}
            className="underline-offset-4 hover:underline"
          >
            {t('columns.openBooking')}
          </Link>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      id: 'expires',
      header: t('columns.expires'),
      meta: { label: t('columns.expires') },
      cell: ({ row: { original: c } }) =>
        c.expires_on && c.amount > 0 ? (
          <span className="tabular-nums">{formatIsoDate(c.expires_on)}</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      id: 'amount',
      header: t('columns.amount'),
      meta: { label: t('columns.amount'), align: 'right' },
      cell: ({ row: { original: c } }) =>
        c.amount < 0 ? (
          <span className="text-muted-foreground">{formatCents(c.amount)}</span>
        ) : (
          <span className="font-medium">+{formatCents(c.amount)}</span>
        ),
    },
  ];

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        <StatCard
          label={t('available')}
          value={ledger.isPending ? '…' : formatCents(available)}
          icon={WalletIcon}
          hint={
            first && first.balance !== first.available
              ? t('balanceHint', { amount: formatCents(first.balance) })
              : undefined
          }
        />
        <StatCard
          label={t('expiring')}
          value={nextExpiry ? formatCents(nextExpiry.amount) : '—'}
          icon={CalendarClockIcon}
          hint={
            nextExpiry
              ? t('expiringHint', { date: formatIsoDate(nextExpiry.expires_on) })
              : t('noExpiry')
          }
        />
      </div>
      <SectionCard
        flush
        title={t('title')}
        description={t('description')}
        actions={
          !readOnly && (
            <>
              {perms.canTakePayments && available > 0 && payable.length > 0 && (
                <Button variant="outline" onClick={() => setApplying(true)}>
                  <HandCoinsIcon aria-hidden="true" />
                  {t('apply.open')}
                </Button>
              )}
              {perms.canRefund && (
                <Button variant="outline" onClick={() => setGranting(true)}>
                  <PlusIcon aria-hidden="true" />
                  {t('grant.open')}
                </Button>
              )}
            </>
          )
        }
      >
        <DataTable
          embedded
          caption={t('title')}
          columns={columns}
          data={items}
          loading={ledger.isPending}
          error={
            ledger.error ? (
              <QueryState
                isLoading={false}
                error={ledger.error}
                onRetry={() => void ledger.refetch()}
              />
            ) : undefined
          }
          getRowId={(c) => c.id}
          columnToggle={false}
          pageSize={0}
          loadMore={{
            hasMore: !!ledger.hasNextPage,
            loading: ledger.isFetchingNextPage,
            onLoadMore: () => void ledger.fetchNextPage(),
            label: t('loadMore'),
          }}
          empty={<EmptyState icon={CoinsIcon} bordered={false} title={t('empty')} />}
        />
      </SectionCard>
      <GrantCreditDialog
        establishmentId={establishmentId}
        customerId={customerId}
        available={available}
        open={granting}
        onOpenChange={setGranting}
      />
      <ApplyCreditDialog
        establishmentId={establishmentId}
        available={available}
        bookings={payable}
        open={applying}
        onOpenChange={setApplying}
      />
    </div>
  );
}
