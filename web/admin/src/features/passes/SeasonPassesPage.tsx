import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Label } from '@arenile/shared/ui/label';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import {
  EmptyState,
  FormSheet,
  OptionSelect,
  PageHeader,
  QueryState,
  StatusBadge,
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

  return (
    <section aria-labelledby="page-title" className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          perms.canWrite && (
            <Button onClick={() => setCreating(true)} disabled={seasons.length === 0}>
              <PlusIcon aria-hidden="true" />
              {t('create.title')}
            </Button>
          )
        }
      />
      <div className="flex flex-wrap items-end gap-4">
        <div className="space-y-1.5">
          <Label htmlFor="passes-season">{t('fields.season')}</Label>
          <OptionSelect
            id="passes-season"
            value={seasonId}
            onChange={setSeasonChoice}
            options={seasons.map((s) => ({ value: s.id, label: String(s.year) }))}
            className="w-40"
          />
        </div>
        {passes.length > 0 && (
          <p className="pb-2 text-sm text-muted-foreground">
            {t('list.count', { count: passes.length })}
          </p>
        )}
      </div>

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

      {groups.map((g) => (
        <Card key={g.zoneId || 'other'} className="gap-0 py-0">
          <CardHeader className="border-b py-4">
            <CardTitle className="text-base">{g.name}</CardTitle>
            <CardDescription>{t('list.count', { count: g.count })}</CardDescription>
          </CardHeader>
          <Table>
            <caption className="sr-only">{t('list.zoneCaption', { zone: g.name })}</caption>
            <TableHeader>
              <TableRow>
                <TableHead className="pl-6">{t('columns.spot')}</TableHead>
                <TableHead>{t('columns.customer')}</TableHead>
                <TableHead>{t('columns.period')}</TableHead>
                <TableHead>{t('columns.status')}</TableHead>
                <TableHead>{t('columns.released')}</TableHead>
                <TableHead className="text-right">{t('columns.credit')}</TableHead>
                <TableHead className="pr-6 text-right">{t('columns.balance')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {g.rows.flatMap(({ row, passes: rowPasses }) => [
                ...(row && g.rows.length > 1
                  ? [
                      <TableRow key={`row-${row}`} className="bg-muted/40 hover:bg-muted/40">
                        <TableCell
                          colSpan={7}
                          className="pl-6 text-xs font-medium text-muted-foreground"
                        >
                          {t('list.row', { row })}
                        </TableCell>
                      </TableRow>,
                    ]
                  : []),
                ...rowPasses.map((p) => (
                  <TableRow
                    key={p.id}
                    className="cursor-pointer"
                    onClick={() => setOpenId(p.id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        setOpenId(p.id);
                      }
                    }}
                    tabIndex={0}
                    aria-label={t('list.openAria', {
                      spot: p.spot_label,
                      customer: p.customer_display_name,
                    })}
                  >
                    <TableCell className="pl-6 font-medium">
                      {p.spot_label}
                      {p.slots !== 'FULL' && (
                        <Badge variant="outline" className="ml-2">
                          {t(`bookings:slots.${p.slots}`)}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>{p.customer_display_name}</TableCell>
                    <TableCell className="tabular-nums">
                      {formatIsoDate(p.starts_on)} → {formatIsoDate(p.ends_on)}
                    </TableCell>
                    <TableCell>
                      <StatusBadge tone={STATUS_TONE[p.status]}>
                        {t(`bookings:status.${p.status}`)}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="tabular-nums">{p.released_days || '—'}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {p.credit_amount ? formatCents(p.credit_amount) : '—'}
                    </TableCell>
                    <TableCell
                      className={
                        p.balance_due > 0
                          ? 'pr-6 text-right font-medium text-destructive tabular-nums'
                          : 'pr-6 text-right text-muted-foreground tabular-nums'
                      }
                    >
                      {formatCents(p.balance_due)}
                    </TableCell>
                  </TableRow>
                )),
              ])}
            </TableBody>
          </Table>
        </Card>
      ))}
      {list.hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="outline"
            onClick={() => void list.fetchNextPage()}
            disabled={list.isFetchingNextPage}
          >
            {t('list.loadMore')}
          </Button>
        </div>
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
    </section>
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
