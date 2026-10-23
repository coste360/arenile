import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import { Label } from '@arenile/shared/ui/label';
import { Switch } from '@arenile/shared/ui/switch';
import {
  DataTable,
  DateRangePicker,
  EmptyState,
  MultiSelect,
  OptionSelect,
  PageHeader,
  QueryState,
  StatusBadge,
  type ColumnDef,
  type IsoDateRange,
} from '@arenile/shared/ui/patterns';
import { useNavigate } from '@tanstack/react-router';
import { CalendarCheckIcon, FilterXIcon, PlusIcon, SearchIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { useDebounced } from '../customers/lib';
import { useBookings, type BookingFilters, type BookingSummary } from './api';
import {
  BOOKING_STATUSES,
  CHANNELS,
  STATUS_TONE,
  useOperationalPermissions,
  type BookingStatus,
  type Channel,
} from './model';
import { NewBookingSheet } from './NewBookingSheet';
import { compareSpots, useEstablishmentSpots } from './spots';

const ALL = '__all';

/** Prenotazioni (spec 15.1): filterable list with cursor paging, new booking sheet. */
export function BookingsPage() {
  const { t, i18n } = useTranslation('bookings');
  const establishmentId = useRequiredEstablishmentId();
  const navigate = useNavigate();
  const perms = useOperationalPermissions();
  const inventory = useEstablishmentSpots(establishmentId, i18n.language);
  const [range, setRange] = useState<IsoDateRange | undefined>();
  const [statuses, setStatuses] = useState<string[]>([]);
  const [channel, setChannel] = useState<string>(ALL);
  const [spotId, setSpotId] = useState<string>(ALL);
  const [kind, setKind] = useState<string>(ALL);
  const [balanceDue, setBalanceDue] = useState(false);
  const [search, setSearch] = useState('');
  const q = useDebounced(search.trim(), 300);
  const [sheetOpen, setSheetOpen] = useState(false);

  const filters: BookingFilters = {
    from: range?.from,
    to: range?.to ?? range?.from,
    status: statuses as BookingStatus[],
    channel: channel === ALL ? undefined : (channel as Channel),
    spot_id: spotId === ALL ? undefined : spotId,
    season_pass: kind === ALL ? undefined : kind === 'pass',
    balance_due: balanceDue || undefined,
    q: q || undefined,
  };
  const list = useBookings(establishmentId, filters);
  const rows = list.data?.pages.flatMap((p) => p.items) ?? [];
  const filtered =
    !!range ||
    statuses.length > 0 ||
    channel !== ALL ||
    spotId !== ALL ||
    kind !== ALL ||
    balanceDue ||
    !!q;
  const reset = () => {
    setRange(undefined);
    setStatuses([]);
    setChannel(ALL);
    setSpotId(ALL);
    setKind(ALL);
    setBalanceDue(false);
    setSearch('');
  };

  const columns: ColumnDef<BookingSummary>[] = [
    {
      accessorKey: 'code',
      header: t('columns.code'),
      meta: { label: t('columns.code') },
      cell: ({ row }) => (
        <span className="flex items-center gap-2 font-medium">
          {row.original.code}
          {row.original.is_season_pass && <Badge variant="outline">{t('list.pass')}</Badge>}
        </span>
      ),
    },
    {
      accessorKey: 'customer_display_name',
      header: t('columns.customer'),
      meta: { label: t('columns.customer') },
    },
    {
      id: 'period',
      header: t('columns.period'),
      meta: { label: t('columns.period') },
      cell: ({ row }) => (
        <span className="tabular-nums">
          {formatIsoDate(row.original.starts_on)}
          {row.original.ends_on !== row.original.starts_on &&
            ` → ${formatIsoDate(row.original.ends_on)}`}
          {row.original.slots !== 'FULL' && (
            <span className="text-muted-foreground"> · {t(`slots.${row.original.slots}`)}</span>
          )}
        </span>
      ),
    },
    {
      id: 'spots',
      header: t('columns.spots'),
      meta: { label: t('columns.spots') },
      cell: ({ row }) =>
        row.original.spot_labels.join(', ') || <span className="text-muted-foreground">—</span>,
    },
    {
      accessorKey: 'status',
      header: t('columns.status'),
      meta: { label: t('columns.status') },
      cell: ({ row }) => (
        <StatusBadge tone={STATUS_TONE[row.original.status]}>
          {t(`status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'channel',
      header: t('columns.channel'),
      meta: { label: t('columns.channel') },
      cell: ({ row }) => t(`channel.${row.original.channel}`),
    },
    {
      accessorKey: 'total_amount',
      header: () => <span className="block text-right">{t('columns.total')}</span>,
      meta: { label: t('columns.total') },
      cell: ({ row }) => (
        <span className="block text-right tabular-nums">
          {formatCents(row.original.total_amount)}
        </span>
      ),
    },
    {
      accessorKey: 'balance_due',
      header: () => <span className="block text-right">{t('columns.balance')}</span>,
      meta: { label: t('columns.balance') },
      cell: ({ row }) =>
        row.original.balance_due > 0 ? (
          <span className="block text-right font-medium text-destructive tabular-nums">
            {formatCents(row.original.balance_due)}
          </span>
        ) : (
          <span className="block text-right text-muted-foreground tabular-nums">
            {formatCents(0)}
          </span>
        ),
    },
  ];

  const spots = [...inventory.spots].sort(compareSpots);

  return (
    <section aria-labelledby="page-title" className="space-y-6">
      <PageHeader
        title={t('list.title')}
        description={t('list.description')}
        actions={
          perms.canWrite && (
            <Button onClick={() => setSheetOpen(true)}>
              <PlusIcon aria-hidden="true" />
              {t('new.title')}
            </Button>
          )
        }
      />
      <div
        className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
        role="search"
        aria-label={t('filters.title')}
      >
        <div className="space-y-1.5">
          <Label htmlFor="bookings-q">{t('filters.search')}</Label>
          <InputGroup>
            <InputGroupAddon>
              <SearchIcon aria-hidden="true" />
            </InputGroupAddon>
            <InputGroupInput
              id="bookings-q"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t('filters.searchPlaceholder')}
            />
          </InputGroup>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="bookings-range">{t('filters.period')}</Label>
          <DateRangePicker id="bookings-range" value={range} onChange={setRange} clearable />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="bookings-status">{t('filters.status')}</Label>
          <MultiSelect
            id="bookings-status"
            value={statuses}
            onChange={setStatuses}
            options={BOOKING_STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) }))}
            placeholder={t('filters.allStatuses')}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="bookings-channel">{t('filters.channel')}</Label>
          <OptionSelect
            id="bookings-channel"
            value={channel}
            onChange={setChannel}
            options={[
              { value: ALL, label: t('filters.allChannels') },
              ...CHANNELS.map((c) => ({ value: c, label: t(`channel.${c}`) })),
            ]}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="bookings-spot">{t('filters.spot')}</Label>
          <OptionSelect
            id="bookings-spot"
            value={spotId}
            onChange={setSpotId}
            options={[
              { value: ALL, label: t('filters.allSpots') },
              ...spots.map((s) => ({ value: s.id, label: s.label })),
            ]}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="bookings-kind">{t('filters.kind')}</Label>
          <OptionSelect
            id="bookings-kind"
            value={kind}
            onChange={setKind}
            options={[
              { value: ALL, label: t('filters.allKinds') },
              { value: 'booking', label: t('filters.onlyBookings') },
              { value: 'pass', label: t('filters.onlyPasses') },
            ]}
          />
        </div>
        <div className="flex items-end gap-3 pb-2">
          <Switch id="bookings-balance" checked={balanceDue} onCheckedChange={setBalanceDue} />
          <Label htmlFor="bookings-balance">{t('filters.balanceDue')}</Label>
        </div>
        <div className="flex items-end">
          {filtered && (
            <Button variant="ghost" onClick={reset}>
              <FilterXIcon aria-hidden="true" />
              {t('filters.reset')}
            </Button>
          )}
        </div>
      </div>

      {list.error ? (
        <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
      ) : (
        <DataTable
          caption={t('list.title')}
          columns={columns}
          data={rows}
          loading={list.isPending}
          pageSize={0}
          getRowId={(r) => r.id}
          onRowClick={(r) =>
            void navigate({
              to: '/e/$establishmentId/prenotazioni/$bookingId',
              params: { establishmentId, bookingId: r.id },
            })
          }
          empty={
            <EmptyState
              icon={CalendarCheckIcon}
              bordered={false}
              title={filtered ? t('list.emptyFiltered') : t('list.empty')}
              action={
                filtered ? (
                  <Button variant="outline" onClick={reset}>
                    {t('filters.reset')}
                  </Button>
                ) : undefined
              }
            />
          }
        />
      )}
      {list.hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            onClick={() => void list.fetchNextPage()}
            disabled={list.isFetchingNextPage}
          >
            {list.isFetchingNextPage ? t('list.loadingMore') : t('list.loadMore')}
          </Button>
        </div>
      )}
      <NewBookingSheet
        establishmentId={establishmentId}
        open={sheetOpen}
        onOpenChange={setSheetOpen}
      />
    </section>
  );
}
