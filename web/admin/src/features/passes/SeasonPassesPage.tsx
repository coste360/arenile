import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import {
  DataTable,
  EmptyState,
  FormSheet,
  OptionSelect,
  Page,
  QueryState,
  StatusBadge,
  type ColumnDef,
} from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { ExternalLinkIcon, PlusIcon, TicketIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { STATUS_TONE, todayIso, useOperationalPermissions } from '../bookings/model';
import { compareSpots, useEstablishmentSpots, type EstablishmentSpots } from '../bookings/spots';
import { seasons as seasonsResource, type Season } from '../configuration/api/resources';
import { useSeasonPass, useSeasonPasses, type SeasonPass } from './api';
import { CreatePassDialog } from './CreatePassDialog';
import { ReleaseCalendar } from './ReleaseCalendar';

/** Season of today, else the latest one. */
function currentSeason(seasons: Season[], today: string): Season | undefined {
  return (
    seasons.find((s) => s.opens_on <= today && s.closes_on >= today) ??
    [...seasons].sort((a, b) => b.year - a.year)[0]
  );
}

interface ZoneGroup {
  zoneId: string;
  name: string;
  rows: { row: string; passes: SeasonPass[] }[];
  count: number;
}

/** Passes grouped by zone, then by row, in map order. */
function groupPasses(
  passes: SeasonPass[],
  inventory: EstablishmentSpots,
  otherLabel: string,
): ZoneGroup[] {
  const byZone = new Map<string, SeasonPass[]>();
  for (const p of passes) {
    const zoneId = inventory.spotById.get(p.spot_id)?.zone_id ?? '';
    byZone.set(zoneId, [...(byZone.get(zoneId) ?? []), p]);
  }
  const order = new Map(inventory.zones.map((z, i) => [z.id, i]));
  return [...byZone.entries()]
    .sort(([a], [b]) => (order.get(a) ?? 999) - (order.get(b) ?? 999))
    .map(([zoneId, items]) => {
      const sorted = [...items].sort((a, b) => {
        const sa = inventory.spotById.get(a.spot_id);
        const sb = inventory.spotById.get(b.spot_id);
        return sa && sb
          ? compareSpots(sa, sb)
          : a.spot_label.localeCompare(b.spot_label, undefined, { numeric: true });
      });
      const rows = new Map<string, SeasonPass[]>();
      for (const p of sorted) {
        const row = inventory.spotById.get(p.spot_id)?.row_label ?? '';
        rows.set(row, [...(rows.get(row) ?? []), p]);
      }
      return {
        zoneId,
        name: inventory.zoneName(zoneId) || otherLabel,
        rows: [...rows.entries()].map(([row, ps]) => ({ row, passes: ps })),
        count: items.length,
      };
    });
}

/** Abbonamenti (spec 7.7, 15.1): season passes by zone and row, creation, releases calendar. */
export function SeasonPassesPage() {
  const { t, i18n } = useTranslation('passes');
  const establishmentId = useRequiredEstablishmentId();
  const perms = useOperationalPermissions();
  const today = todayIso();
  const seasonList = seasonsResource.useList(establishmentId);
  const seasons = [...(seasonList.data ?? [])].sort((a, b) => b.year - a.year);
  const [seasonChoice, setSeasonChoice] = useState('');
  const seasonId = seasonChoice || currentSeason(seasons, today)?.id || '';
  const inventory = useEstablishmentSpots(establishmentId, i18n.language);
  const list = useSeasonPasses(establishmentId, { season_id: seasonId }, !!seasonId);
  const passes = list.data?.pages.flatMap((p) => p.items) ?? [];
  const groups = groupPasses(passes, inventory, t('list.otherZone'));
  const [creating, setCreating] = useState(false);
  const [openId, setOpenId] = useState<string | undefined>();

  // One table grouped by zone (and by row when a zone has several), in map order.
  const groupKey = new Map<string, string>();
  const groupLabel = new Map<string, string>();
  const rows: SeasonPass[] = [];
  for (const g of groups) {
    for (const { row, passes: rowPasses } of g.rows) {
      const key = `${g.zoneId}|${row}`;
      groupLabel.set(
        key,
        row && g.rows.length > 1 ? `${g.name} · ${t('list.row', { row })}` : g.name,
      );
      for (const p of rowPasses) {
        groupKey.set(p.id, key);
        rows.push(p);
      }
    }
  }

  const columns: ColumnDef<SeasonPass>[] = [
    {
      accessorKey: 'spot_label',
      sortingFn: 'alphanumeric',
      header: t('columns.spot'),
      meta: { label: t('columns.spot') },
      cell: ({ row }) => (
        <span className="flex items-center gap-2 font-medium">
          {row.original.spot_label}
          {row.original.slots !== 'FULL' && (
            <Badge variant="outline">{t(`bookings:slots.${row.original.slots}`)}</Badge>
          )}
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
          {formatIsoDate(row.original.starts_on)} → {formatIsoDate(row.original.ends_on)}
        </span>
      ),
    },
    {
      accessorKey: 'status',
      header: t('columns.status'),
      meta: { label: t('columns.status') },
      cell: ({ row }) => (
        <StatusBadge tone={STATUS_TONE[row.original.status]}>
          {t(`bookings:status.${row.original.status}`)}
        </StatusBadge>
      ),
    },
    {
      accessorKey: 'released_days',
      header: t('columns.released'),
      meta: { label: t('columns.released'), align: 'right' },
      cell: ({ row }) => row.original.released_days || '—',
    },
    {
      accessorKey: 'credit_amount',
      header: t('columns.credit'),
      meta: { label: t('columns.credit'), align: 'right' },
      cell: ({ row }) =>
        row.original.credit_amount ? formatCents(row.original.credit_amount) : '—',
    },
    {
      accessorKey: 'balance_due',
      header: t('columns.balance'),
      meta: { label: t('columns.balance'), align: 'right' },
      cell: ({ row }) =>
        row.original.balance_due > 0 ? (
          <span className="font-medium text-destructive">
            {formatCents(row.original.balance_due)}
          </span>
        ) : (
          <span className="text-muted-foreground">{formatCents(row.original.balance_due)}</span>
        ),
    },
  ];

  return (
    <Page
      title={t('title')}
      description={t('description')}
      actions={
        <>
          {seasons.length > 0 && (
            <div className="flex items-center gap-2">
              <Label htmlFor="passes-season" className="text-muted-foreground">
                {t('fields.season')}
              </Label>
              <OptionSelect
                id="passes-season"
                value={seasonId}
                onChange={setSeasonChoice}
                options={seasons.map((s) => ({ value: s.id, label: String(s.year) }))}
                className="w-32"
              />
            </div>
          )}
          {perms.canWrite && (
            <Button onClick={() => setCreating(true)} disabled={seasons.length === 0}>
              <PlusIcon aria-hidden="true" />
              {t('create.title')}
            </Button>
          )}
        </>
      }
    >
      <QueryState
        isLoading={seasonList.isPending || (!!seasonId && list.isPending)}
        error={seasonList.error ?? list.error}
        onRetry={() => void list.refetch()}
      />
      {!seasonList.isPending && seasons.length === 0 && (
        <EmptyState icon={TicketIcon} title={t('list.noSeasons')} />
      )}
      {list.isSuccess && passes.length === 0 && (
        <EmptyState icon={TicketIcon} title={t('list.empty')} description={t('list.emptyHint')} />
      )}

      {rows.length > 0 && (
        <DataTable
          caption={t('title')}
          columns={columns}
          data={rows}
          pageSize={0}
          getRowId={(p) => p.id}
          toolbar={
            <span className="text-sm text-muted-foreground">
              {t('list.count', { count: passes.length })}
            </span>
          }
          groupBy={(p) => groupKey.get(p.id) ?? ''}
          renderGroup={(key, items) => (
            <span className="flex flex-wrap items-baseline gap-x-2">
              {groupLabel.get(key)}
              <span className="font-normal text-muted-foreground">
                {t('list.count', { count: items.length })}
              </span>
            </span>
          )}
          onRowClick={(p) => setOpenId(p.id)}
          getRowLabel={(p) =>
            t('list.openAria', { spot: p.spot_label, customer: p.customer_display_name })
          }
          loadMore={{
            hasMore: !!list.hasNextPage,
            loading: list.isFetchingNextPage,
            onLoadMore: () => void list.fetchNextPage(),
            label: t('list.loadMore'),
          }}
        />
      )}

      <CreatePassDialog
        establishmentId={establishmentId}
        open={creating}
        onOpenChange={setCreating}
        seasons={seasons}
        defaultSeasonId={seasonId}
        inventory={inventory}
        onCreated={setOpenId}
      />
      <PassSheet
        establishmentId={establishmentId}
        passId={openId}
        onClose={() => setOpenId(undefined)}
        canWrite={perms.canWrite}
      />
    </Page>
  );
}

function PassSheet({
  establishmentId,
  passId,
  onClose,
  canWrite,
}: {
  establishmentId: string;
  passId: string | undefined;
  onClose: () => void;
  canWrite: boolean;
}) {
  const { t } = useTranslation('passes');
  const detail = useSeasonPass(establishmentId, passId);
  const pass = detail.data?.pass;
  return (
    <FormSheet
      open={!!passId}
      onOpenChange={(o) => !o && onClose()}
      title={
        pass
          ? t('sheet.title', { spot: pass.spot_label, customer: pass.customer_display_name })
          : t('sheet.loading')
      }
      description={
        pass
          ? `${pass.code} · ${formatIsoDate(pass.starts_on)} → ${formatIsoDate(pass.ends_on)}`
          : undefined
      }
      size="lg"
    >
      <QueryState
        isLoading={detail.isPending}
        error={detail.error}
        onRetry={() => void detail.refetch()}
      />
      {pass && detail.data && (
        <div className="space-y-6">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4">
            <div>
              <dt className="text-muted-foreground">{t('columns.status')}</dt>
              <dd>
                <StatusBadge tone={STATUS_TONE[pass.status]}>
                  {t(`bookings:status.${pass.status}`)}
                </StatusBadge>
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('sheet.total')}</dt>
              <dd className="tabular-nums">{formatCents(pass.total_amount)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('sheet.paid')}</dt>
              <dd className="tabular-nums">{formatCents(pass.paid_amount)}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('columns.balance')}</dt>
              <dd
                className={
                  pass.balance_due > 0
                    ? 'font-medium text-destructive tabular-nums'
                    : 'tabular-nums'
                }
              >
                {formatCents(pass.balance_due)}
              </dd>
            </div>
          </dl>
          <Button variant="outline" size="sm" asChild>
            <Link
              to="/e/$establishmentId/prenotazioni/$bookingId"
              params={{ establishmentId, bookingId: pass.id }}
            >
              <ExternalLinkIcon aria-hidden="true" />
              {t('sheet.openBooking')}
            </Link>
          </Button>
          <ReleaseCalendar
            establishmentId={establishmentId}
            pass={pass}
            releases={detail.data.releases}
            canWrite={canWrite && (pass.status === 'confirmed' || pass.status === 'checked_in')}
          />
        </div>
      )}
    </FormSheet>
  );
}
