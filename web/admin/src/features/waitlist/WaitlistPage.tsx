import { formatIsoDate } from '@arenile/shared/lib/dates';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import {
  ConfirmDialog,
  DataTable,
  EmptyState,
  FormSheet,
  OptionSelect,
  Page,
  QueryState,
  SectionCard,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import {
  ArrowDownIcon,
  ArrowUpIcon,
  BanIcon,
  GiftIcon,
  HourglassIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  SettingsIcon,
  Trash2Icon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat, useRequiredEstablishmentId } from '../../establishment';
import { useOperationalPermissions } from '../bookings/model';
import { useEstablishmentSpots, type EstablishmentSpots } from '../bookings/spots';
import { packs as packsResource } from '../configuration/api/resources';
import { useConfigContext } from '../configuration/api/useConfigContext';
import { localize } from '../configuration/lib/i18nText';
import { useDebounced } from '../customers/lib';
import { Countdown } from '../renewals/Countdown';
import { useNow } from '../renewals/lib';
import { describeM5bProblem } from '../renewals/problems';
import {
  useRemoveWaitlistEntry,
  useReorderWaitlist,
  useRevokeWaitlistOffer,
  useWaitlist,
  useWaitlistEntry,
  useWaitlistSettings,
  type WaitlistEntry,
  type WaitlistEntryStatus,
  type WaitlistOffer,
} from './api';
import { ENTRY_TONE, OFFER_TONE } from './lib';
import { EntryDialog, OfferDialog, SettingsDialog } from './WaitlistDialogs';

const STATUSES: readonly WaitlistEntryStatus[] = [
  'waiting',
  'offered',
  'converted',
  'expired',
  'removed',
];

/** "Ombrellone Base · Prima fila · ×2": what the entry asks for. */
function useRequestLabel(establishmentId: string, inventory: EstablishmentSpots) {
  const { i18n } = useTranslation();
  const packs = packsResource.useList(establishmentId);
  return (e: Pick<WaitlistEntry, 'pack_id' | 'zone_id' | 'spot_id' | 'quantity'>) => {
    const pack = packs.data?.find((p) => p.id === e.pack_id);
    const parts = [
      pack ? localize(pack.name, i18n.language, 'it') || pack.code : '',
      e.spot_id
        ? (inventory.spotById.get(e.spot_id)?.label ?? '')
        : e.zone_id
          ? inventory.zoneName(e.zone_id)
          : '',
      e.quantity > 1 ? `×${e.quantity}` : '',
    ].filter(Boolean);
    return parts.join(' · ') || '—';
  };
}

function Period({ entry }: { entry: Pick<WaitlistEntry, 'starts_on' | 'ends_on' | 'slots'> }) {
  const { t } = useTranslation('waitlist');
  return (
    <span className="flex flex-wrap items-center gap-2 tabular-nums">
      {formatIsoDate(entry.starts_on)}
      {entry.ends_on !== entry.starts_on && ` → ${formatIsoDate(entry.ends_on)}`}
      {entry.slots !== 'FULL' && (
        <Badge variant="outline">{t(`bookings:slots.${entry.slots}`)}</Badge>
      )}
    </span>
  );
}

/**
 * Lista d'attesa (spec 10.5): requests with no availability in queue order,
 * reorder, manual offers (a held spot with a deadline), revoke, remove, and
 * the settings (automatic offers when inventory frees, public join).
 */
export function WaitlistPage() {
  const { t, i18n } = useTranslation('waitlist');
  const establishmentId = useRequiredEstablishmentId();
  const perms = useOperationalPermissions();
  const ctx = useConfigContext();
  const now = useNow(30_000);
  const [status, setStatus] = useState<'' | WaitlistEntryStatus>('');
  const [zoneId, setZoneId] = useState('');
  const [search, setSearch] = useState('');
  const q = useDebounced(search.trim());
  const filters = { status: status || undefined, zone_id: zoneId || undefined, q: q || undefined };
  const list = useWaitlist(establishmentId, filters);
  const entries = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data]);
  const settings = useWaitlistSettings(establishmentId);
  const inventory = useEstablishmentSpots(establishmentId, i18n.language);
  const requestLabel = useRequestLabel(establishmentId, inventory);
  const reorder = useReorderWaitlist(establishmentId);
  const remove = useRemoveWaitlistEntry(establishmentId);
  const revoke = useRevokeWaitlistOffer(establishmentId);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<WaitlistEntry | undefined>();
  const [offering, setOffering] = useState<WaitlistEntry | null>(null);
  const [removing, setRemoving] = useState<WaitlistEntry | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [openId, setOpenId] = useState<string | undefined>();

  // Reordering needs the whole queue on screen: no filters other than "waiting".
  const queue = entries
    .filter((e) => e.status === 'waiting')
    .sort((a, b) => a.position - b.position);
  const canReorder =
    perms.canWrite &&
    !zoneId &&
    !q &&
    (status === '' || status === 'waiting') &&
    !list.hasNextPage &&
    queue.length > 1;

  const move = (entry: WaitlistEntry, delta: -1 | 1) => {
    const ids = queue.map((e) => e.id);
    const i = ids.indexOf(entry.id);
    const j = i + delta;
    if (i < 0 || j < 0 || j >= ids.length) return;
    [ids[i], ids[j]] = [ids[j] as string, ids[i] as string];
    reorder.mutate(ids, {
      onSuccess: () => toast.success(t('reorder.done', { customer: entry.customer_display_name })),
      onError: (err) => toast.error(describeM5bProblem(err, t).title),
    });
  };

  const runRevoke = (entry: WaitlistEntry) => {
    if (!entry.active_offer) return;
    revoke.mutate(
      { entryId: entry.id, offerId: entry.active_offer.id },
      {
        onSuccess: () => toast.success(t('offer.revoked')),
        onError: (err) => toast.error(describeM5bProblem(err, t).title),
      },
    );
  };

  const columns: ColumnDef<WaitlistEntry>[] = [
    {
      id: 'position',
      header: '#',
      meta: { label: t('columns.position'), align: 'right' },
      cell: ({ row }) =>
        row.original.status === 'waiting' ? (
          <span className="font-medium tabular-nums">{row.original.position}</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      id: 'customer',
      header: t('columns.customer'),
      meta: { label: t('columns.customer') },
      cell: ({ row }) => <span className="font-medium">{row.original.customer_display_name}</span>,
    },
    {
      id: 'period',
      header: t('columns.period'),
      meta: { label: t('columns.period') },
      cell: ({ row }) => <Period entry={row.original} />,
    },
    {
      id: 'request',
      header: t('columns.request'),
      meta: { label: t('columns.request') },
      cell: ({ row }) => requestLabel(row.original),
    },
    {
      id: 'status',
      header: t('columns.status'),
      meta: { label: t('columns.status') },
      cell: ({ row }) => (
        <StatusBadge tone={ENTRY_TONE[row.original.status]}>
          {t(`status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      id: 'offer',
      header: t('columns.offer'),
      meta: { label: t('columns.offer') },
      cell: ({ row }) => {
        const o = row.original.active_offer;
        if (!o) return <span className="text-muted-foreground">—</span>;
        return (
          <span className="flex flex-col">
            <span>{o.spot_label ?? t('offer.anySpot')}</span>
            {o.status === 'pending' ? (
              <span className="text-xs">
                <Countdown deadline={o.expires_at} now={now} />
              </span>
            ) : (
              <span className="text-xs text-muted-foreground">{t(`offerStatus.${o.status}`)}</span>
            )}
          </span>
        );
      },
    },
    {
      id: 'channel',
      header: t('columns.channel'),
      meta: { label: t('columns.channel') },
      cell: ({ row }) =>
        t(`bookings:channel.${row.original.channel}`, { defaultValue: row.original.channel }),
    },
    {
      id: 'actions',
      enableHiding: false,
      header: () => <span className="sr-only">{t('columns.actions')}</span>,
      cell: ({ row: { original: e } }) => {
        const i = queue.findIndex((x) => x.id === e.id);
        const active = e.status === 'waiting' || e.status === 'offered';
        if (!perms.canWrite) return null;
        return (
          <div
            className="flex items-center justify-end gap-0.5"
            onClick={(ev) => ev.stopPropagation()}
            onKeyDown={(ev) => ev.stopPropagation()}
            role="presentation"
          >
            {canReorder && e.status === 'waiting' && (
              <>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('reorder.up', { customer: e.customer_display_name })}
                  title={t('reorder.upShort')}
                  disabled={i <= 0 || reorder.isPending}
                  onClick={() => move(e, -1)}
                >
                  <ArrowUpIcon aria-hidden="true" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={t('reorder.down', { customer: e.customer_display_name })}
                  title={t('reorder.downShort')}
                  disabled={i < 0 || i >= queue.length - 1 || reorder.isPending}
                  onClick={() => move(e, 1)}
                >
                  <ArrowDownIcon aria-hidden="true" />
                </Button>
              </>
            )}
            {active && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('row.more', { customer: e.customer_display_name })}
                  >
                    <MoreHorizontalIcon aria-hidden="true" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {e.status === 'waiting' && (
                    <DropdownMenuItem onSelect={() => setOffering(e)}>
                      <GiftIcon aria-hidden="true" />
                      {t('row.offer')}
                    </DropdownMenuItem>
                  )}
                  {e.active_offer?.status === 'pending' && (
                    <DropdownMenuItem onSelect={() => runRevoke(e)}>
                      <BanIcon aria-hidden="true" />
                      {t('row.revoke')}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem onSelect={() => setEditing(e)}>
                    <PencilIcon aria-hidden="true" />
                    {t('row.edit')}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive" onSelect={() => setRemoving(e)}>
                    <Trash2Icon aria-hidden="true" />
                    {t('row.remove')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        );
      },
    },
  ];

  const disabled = settings.data && !settings.data.enabled;
  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={
        <>
          {ctx.canConfigure && settings.data && (
            <Button variant="outline" onClick={() => setSettingsOpen(true)}>
              <SettingsIcon aria-hidden="true" />
              {t('settings.open')}
            </Button>
          )}
          {perms.canWrite && (
            <Button onClick={() => setCreating(true)} disabled={!!disabled}>
              <PlusIcon aria-hidden="true" />
              {t('entry.newTitle')}
            </Button>
          )}
        </>
      }
      notice={
        disabled ? (
          <Alert variant="warning" role="status">
            <TriangleAlertIcon aria-hidden="true" />
            <AlertTitle>{t('disabled.title')}</AlertTitle>
            <AlertDescription>{t('disabled.description')}</AlertDescription>
          </Alert>
        ) : settings.data ? (
          <p className="text-sm text-muted-foreground">
            {settings.data.auto_offer
              ? t('settings.summaryAuto', { minutes: settings.data.offer_hold_minutes })
              : t('settings.summaryManual')}
          </p>
        ) : undefined
      }
    >
      <DataTable
        caption={t('title')}
        columns={columns}
        data={entries}
        loading={list.isPending}
        error={
          list.error ? (
            <QueryState isLoading={false} error={list.error} onRetry={() => void list.refetch()} />
          ) : undefined
        }
        getRowId={(e) => e.id}
        pageSize={0}
        initialColumnVisibility={{ channel: false }}
        searchable
        searchValue={search}
        onSearchChange={setSearch}
        searchPlaceholder={t('filters.search')}
        filters={
          <>
            <OptionSelect
              aria-label={t('filters.status')}
              value={status}
              onChange={(v) => setStatus(v as '' | WaitlistEntryStatus)}
              options={[
                { value: '', label: t('filters.allStatuses') },
                ...STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) })),
              ]}
              className="w-full sm:w-44"
            />
            {inventory.zones.length > 1 && (
              <OptionSelect
                aria-label={t('filters.zone')}
                value={zoneId}
                onChange={setZoneId}
                options={[
                  { value: '', label: t('filters.allZones') },
                  ...inventory.zones.map((z) => ({
                    value: z.id,
                    label: inventory.zoneName(z.id) || z.code,
                  })),
                ]}
                className="w-full sm:w-44"
              />
            )}
          </>
        }
        onRowClick={(e) => setOpenId(e.id)}
        getRowLabel={(e) => t('row.open', { customer: e.customer_display_name })}
        loadMore={{
          hasMore: !!list.hasNextPage,
          loading: list.isFetchingNextPage,
          onLoadMore: () => void list.fetchNextPage(),
          label: t('loadMore'),
        }}
        empty={
          <EmptyState
            icon={HourglassIcon}
            bordered={false}
            title={t('empty.title')}
            description={t('empty.description')}
          />
        }
      />

      <EntryDialog
        establishmentId={establishmentId}
        open={creating || !!editing}
        onOpenChange={(o) => {
          if (!o) {
            setCreating(false);
            setEditing(undefined);
          }
        }}
        entry={editing}
        inventory={inventory}
      />
      <OfferDialog
        establishmentId={establishmentId}
        entry={offering}
        settings={settings.data}
        inventory={inventory}
        onClose={() => setOffering(null)}
      />
      {settings.data && (
        <SettingsDialog
          establishmentId={establishmentId}
          settings={settings.data}
          open={settingsOpen}
          onOpenChange={setSettingsOpen}
        />
      )}
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t('remove.title')}
        description={
          removing ? t('remove.description', { customer: removing.customer_display_name }) : ''
        }
        confirmLabel={t('row.remove')}
        destructive
        errorText={(err) => describeM5bProblem(err, t).title}
        onConfirm={async () => {
          if (!removing) return;
          await remove.mutateAsync(removing);
          toast.success(t('remove.done'));
        }}
      />
      <EntrySheet
        establishmentId={establishmentId}
        entryId={openId}
        onClose={() => setOpenId(undefined)}
        requestLabel={requestLabel}
        now={now}
      />
    </Page>
  );
}

function EntrySheet({
  establishmentId,
  entryId,
  onClose,
  requestLabel,
  now,
}: {
  establishmentId: string;
  entryId: string | undefined;
  onClose: () => void;
  requestLabel: (e: WaitlistEntry) => string;
  now: number;
}) {
  const { t } = useTranslation('waitlist');
  const fmt = useInstantFormat();
  const detail = useWaitlistEntry(establishmentId, entryId);
  const entry = detail.data?.entry;
  const columns: ColumnDef<WaitlistOffer>[] = [
    {
      id: 'created',
      header: t('offers.created'),
      meta: { label: t('offers.created') },
      cell: ({ row }) => (
        <span className="tabular-nums">{fmt.dateTime(row.original.created_at)}</span>
      ),
    },
    {
      id: 'spot',
      header: t('offers.spot'),
      meta: { label: t('offers.spot') },
      cell: ({ row }) => row.original.spot_label ?? '—',
    },
    {
      id: 'cause',
      header: t('offers.cause'),
      meta: { label: t('offers.cause') },
      cell: ({ row }) => (row.original.cause ? t(`cause.${row.original.cause}`) : '—'),
    },
    {
      id: 'status',
      header: t('offers.status'),
      meta: { label: t('offers.status') },
      cell: ({ row: { original: o } }) =>
        o.status === 'pending' ? (
          <Countdown deadline={o.expires_at} now={now} />
        ) : (
          <StatusBadge tone={OFFER_TONE[o.status]}>{t(`offerStatus.${o.status}`)}</StatusBadge>
        ),
    },
  ];
  return (
    <FormSheet
      open={!!entryId}
      onOpenChange={(o) => !o && onClose()}
      title={entry ? entry.customer_display_name : t('sheet.loading')}
      description={entry ? requestLabel(entry) : undefined}
      size="lg"
    >
      <QueryState
        isLoading={detail.isPending}
        error={detail.error}
        onRetry={() => void detail.refetch()}
      />
      {entry && detail.data && (
        <div className="space-y-6">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('columns.status')}</dt>
              <dd>
                <StatusBadge tone={ENTRY_TONE[entry.status]}>
                  {t(`status.${entry.status}`)}
                </StatusBadge>
              </dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('columns.period')}</dt>
              <dd>
                <Period entry={entry} />
              </dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('columns.position')}</dt>
              <dd className="tabular-nums">{entry.status === 'waiting' ? entry.position : '—'}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-muted-foreground">{t('sheet.joined')}</dt>
              <dd className="tabular-nums">{fmt.date(entry.created_at)}</dd>
            </div>
          </dl>
          {entry.note && <p className="text-sm whitespace-pre-line">{entry.note}</p>}
          <SectionCard flush title={t('offers.title')} as="h3">
            <DataTable
              embedded
              caption={t('offers.title')}
              columns={columns}
              data={detail.data.offers}
              getRowId={(o) => o.id}
              columnToggle={false}
              pageSize={0}
              empty={<EmptyState icon={GiftIcon} bordered={false} title={t('offers.empty')} />}
            />
          </SectionCard>
        </div>
      )}
    </FormSheet>
  );
}
