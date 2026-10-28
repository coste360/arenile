import { formatCents } from '@arenile/shared/lib/money';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import {
  DateRangePicker,
  EmptyState,
  MultiSelect,
  OptionSelect,
  QuantityStepper,
  QueryState,
  SegmentedControl,
} from '@arenile/shared/ui/patterns';
import { ToggleGroup, ToggleGroupItem } from '@arenile/shared/ui/toggle-group';
import {
  CalendarCheckIcon,
  MapPinnedIcon,
  RefreshCwIcon,
  SearchIcon,
  TriangleAlertIcon,
} from 'lucide-react';
import { useNavigate } from '@tanstack/react-router';
import { useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useRequiredEstablishmentId } from '../../establishment';
import { packs as packsResource } from '../configuration/api/resources';
import { localize } from '../configuration/lib/i18nText';
import { SLOTS, todayIso } from '../bookings/model';
import { useEstablishmentSpots } from '../bookings/spots';
import { useOperationsPermissions } from '../operations/lib/permissions';
import { agreements as agreementsResource } from '../pricing/api';
import { useFindSpot, type FindSpotProposal, type FindSpotRequest } from './api';
import { FindSpotBookDialog } from './FindSpotBookDialog';
import { segmentsText } from './lib';
import type { FindSpotDefaults } from './types';

export interface FindSpotPanelProps {
  /** Default: the establishment of the route. */
  establishmentId?: string;
  defaults?: FindSpotDefaults;
  /** "Mostra sulla mappa": highlight the proposal on the operational map. */
  onShowOnMap?: (proposal: FindSpotProposal, request: FindSpotRequest) => void;
  /** Called with the new booking id once a proposal is booked (default: open it). */
  onBooked?: (bookingId: string) => void;
}

const MAX_CHANGES = ['0', '1', '2', '3'];

/**
 * Search form and ranked proposals of "Trova posto" (spec 30.6): dates,
 * slot, number of spots, zones, pack, attributes, maximum spot changes
 * (0-3) and agreement. Proposals come ranked by the server (no changes
 * first, then fewer changes, continuity and closeness); each one shows its
 * segments ("A5 dal 10/07 al 14/07, A7 dal 15/07 al 16/07") and the
 * estimated price with the agreement. "Prenota" books it in one
 * transaction; if a day was taken meanwhile the server answers 409 and the
 * search runs again. Rendered by the page and, from the map and the new
 * booking, in a sheet.
 */
export function FindSpotPanel({
  establishmentId: estProp,
  defaults,
  onShowOnMap,
  onBooked,
}: FindSpotPanelProps) {
  const { t, i18n } = useTranslation('findspot');
  const routeEst = useRequiredEstablishmentId();
  const establishmentId = estProp ?? routeEst;
  const locale = i18n.language || 'it';
  const permissions = useOperationsPermissions();
  const inventory = useEstablishmentSpots(establishmentId, locale);
  const packList = packsResource.useList(establishmentId);
  const agreementList = agreementsResource.useList(establishmentId);
  const today = todayIso();
  const resultsId = useId();
  const countId = useId();

  const [range, setRange] = useState<{ from: string; to?: string }>({
    from: defaults?.startsOn ?? today,
    to: defaults?.endsOn ?? defaults?.startsOn ?? today,
  });
  const [slots, setSlots] = useState<'FULL' | 'AM' | 'PM'>(defaults?.slots ?? 'FULL');
  const [count, setCount] = useState(defaults?.count ?? 1);
  const [zoneIds, setZoneIds] = useState<string[]>([]);
  const [packId, setPackId] = useState('');
  const [attributes, setAttributes] = useState<string[]>([]);
  const [maxChanges, setMaxChanges] = useState('0');
  const [agreementId, setAgreementId] = useState('');
  const [request, setRequest] = useState<FindSpotRequest | null>(null);
  const [booking, setBooking] = useState<FindSpotProposal | null>(null);

  const search = useFindSpot(establishmentId, request);
  const navigate = useNavigate();

  const attributeOptions = useMemo(() => {
    const set = new Set<string>();
    for (const d of inventory.documents) {
      for (const s of d.spots) for (const a of s.attributes ?? []) set.add(a);
    }
    return [...set].sort().map((a) => ({ value: a, label: a }));
  }, [inventory.documents]);
  const zoneOptions = inventory.zones.map((z) => ({
    value: z.id,
    label: inventory.zoneName(z.id),
  }));
  const packOptions = [
    { value: '', label: t('form.noPack') },
    ...(packList.data ?? [])
      .filter((p) => p.active)
      .map((p) => ({ value: p.id, label: localize(p.name, locale, 'it') })),
  ];
  const agreementOptions = [
    { value: '', label: t('form.noAgreement') },
    ...(agreementList.data ?? [])
      .filter((a) => a.active)
      .map((a) => ({ value: a.id, label: a.name })),
  ];

  const submit = () => {
    const to = range.to ?? range.from;
    setRequest({
      starts_on: range.from,
      ends_on: to < range.from ? range.from : to,
      slots,
      count,
      max_spot_changes: Number(maxChanges),
      ...(zoneIds.length ? { zone_ids: zoneIds } : {}),
      ...(packId ? { pack_id: packId } : {}),
      ...(attributes.length ? { attributes } : {}),
      ...(agreementId ? { agreement_id: agreementId } : {}),
    });
  };

  const proposals = search.data?.proposals ?? [];
  const canBook = permissions.canWriteBookings && !!request?.pack_id;

  return (
    <div className="space-y-6">
      <form
        className="space-y-4"
        aria-label={t('form.label')}
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
        noValidate
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="min-w-0 space-y-1.5">
            <Label>{t('form.dates')}</Label>
            <DateRangePicker
              aria-label={t('form.dates')}
              value={range}
              onChange={(v) => v && setRange(v)}
            />
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label>{t('form.slots')}</Label>
            <ToggleGroup
              type="single"
              variant="outline"
              aria-label={t('form.slots')}
              value={slots}
              onValueChange={(v) => v && setSlots(v as typeof slots)}
            >
              {SLOTS.map((s) => (
                <ToggleGroupItem key={s} value={s}>
                  {t(`bookings:slots.${s}`)}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label htmlFor={countId}>{t('form.count')}</Label>
            <QuantityStepper
              id={countId}
              label={t('form.count')}
              value={count}
              onChange={setCount}
              min={1}
              max={20}
            />
            <p className="text-xs text-muted-foreground">{t('form.countHint')}</p>
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label>{t('form.maxChanges')}</Label>
            <SegmentedControl
              label={t('form.maxChanges')}
              value={maxChanges}
              onValueChange={setMaxChanges}
              options={MAX_CHANGES.map((v) => ({ value: v, label: t(`form.changes.${v}`) }))}
              collapseOnNarrow
            />
            <p className="text-xs text-muted-foreground">{t('form.maxChangesHint')}</p>
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label>{t('form.zones')}</Label>
            <MultiSelect
              aria-label={t('form.zones')}
              value={zoneIds}
              onChange={setZoneIds}
              options={zoneOptions}
              placeholder={t('form.anyZone')}
            />
          </div>
          <div className="min-w-0 space-y-1.5">
            <Label>{t('form.pack')}</Label>
            <OptionSelect
              aria-label={t('form.pack')}
              value={packId}
              onChange={setPackId}
              options={packOptions}
            />
            <p className="text-xs text-muted-foreground">{t('form.packHint')}</p>
          </div>
          {attributeOptions.length > 0 && (
            <div className="min-w-0 space-y-1.5">
              <Label>{t('form.attributes')}</Label>
              <MultiSelect
                aria-label={t('form.attributes')}
                value={attributes}
                onChange={setAttributes}
                options={attributeOptions}
                placeholder={t('form.anyAttribute')}
              />
            </div>
          )}
          <div className="min-w-0 space-y-1.5">
            <Label>{t('form.agreement')}</Label>
            <OptionSelect
              aria-label={t('form.agreement')}
              value={agreementId}
              onChange={setAgreementId}
              options={agreementOptions}
            />
          </div>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          {request && (
            <Button
              variant="outline"
              onClick={() => void search.refetch()}
              disabled={search.isFetching}
            >
              <RefreshCwIcon aria-hidden="true" />
              {t('form.again')}
            </Button>
          )}
          <Button type="submit" disabled={search.isFetching}>
            <SearchIcon aria-hidden="true" />
            {t('form.search')}
          </Button>
        </div>
      </form>

      {request && (
        <section aria-labelledby={resultsId} className="space-y-3">
          <h2 id={resultsId} className="font-semibold">
            {t('results.title')}
          </h2>
          <QueryState
            isLoading={search.isLoading}
            error={search.error}
            onRetry={() => void search.refetch()}
          />
          {search.data && proposals.length === 0 && (
            <EmptyState
              icon={SearchIcon}
              title={t('results.empty')}
              description={t('results.emptyHint')}
            />
          )}
          {proposals.length > 0 && (
            <p role="status" className="text-sm text-muted-foreground">
              {t('results.count', { count: proposals.length })}
              {search.data?.truncated ? ` ${t('results.truncated')}` : ''}
            </p>
          )}
          <ol className="space-y-3">
            {proposals.map((p) => (
              <ProposalItem
                key={p.id}
                proposal={p}
                withPrice={!!request.pack_id}
                onShowOnMap={onShowOnMap ? () => onShowOnMap(p, request) : undefined}
                onBook={canBook ? () => setBooking(p) : undefined}
              />
            ))}
          </ol>
          {proposals.length > 0 && !request.pack_id && permissions.canWriteBookings && (
            <p className="text-sm text-muted-foreground">{t('results.packToBook')}</p>
          )}
        </section>
      )}

      {booking && request && (
        <FindSpotBookDialog
          establishmentId={establishmentId}
          request={request}
          proposal={booking}
          open
          onOpenChange={(v) => !v && setBooking(null)}
          onConflict={() => {
            setBooking(null);
            void search.refetch();
          }}
          onBooked={(id) => {
            setBooking(null);
            if (onBooked) onBooked(id);
            else
              void navigate({
                to: '/e/$establishmentId/prenotazioni/$bookingId',
                params: { establishmentId, bookingId: id },
              });
          }}
        />
      )}
    </div>
  );
}

function ProposalItem({
  proposal,
  withPrice,
  onShowOnMap,
  onBook,
}: {
  proposal: FindSpotProposal;
  withPrice: boolean;
  onShowOnMap?: () => void;
  onBook?: () => void;
}) {
  const { t } = useTranslation('findspot');
  const titleId = useId();
  const violations = proposal.violations ?? [];
  return (
    <li
      aria-labelledby={titleId}
      className="space-y-2 rounded-lg border bg-card p-4 text-sm"
      data-testid="findspot-proposal"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 id={titleId} className="font-medium">
          {t('results.proposal', { rank: proposal.rank })}
        </h3>
        <Badge variant={proposal.spot_changes === 0 ? 'secondary' : 'outline'}>
          {proposal.spot_changes === 0
            ? t('results.noChanges')
            : t('results.changes', { count: proposal.spot_changes })}
        </Badge>
        {proposal.adjacent && <Badge variant="outline">{t('results.adjacent')}</Badge>}
        {proposal.agreement_applied && (
          <Badge variant="outline">{t('results.agreementApplied')}</Badge>
        )}
        <span className="ml-auto font-semibold tabular-nums">
          {withPrice && proposal.total !== undefined ? formatCents(proposal.total) : '—'}
        </span>
      </div>
      <ul className="space-y-1">
        {proposal.lines.map((l, i) => (
          <li key={i}>{segmentsText(l.segments, t)}</li>
        ))}
      </ul>
      {violations.length > 0 && (
        <Alert variant="warning">
          <TriangleAlertIcon aria-hidden="true" />
          <AlertDescription>
            {violations
              .map((v) =>
                t(`pricing:violations.${v.code}`, { defaultValue: v.code, ...(v.params ?? {}) }),
              )
              .join(' · ')}
          </AlertDescription>
        </Alert>
      )}
      {(onShowOnMap || onBook) && (
        <div className="flex flex-wrap justify-end gap-2">
          {onShowOnMap && (
            <Button variant="outline" onClick={onShowOnMap}>
              <MapPinnedIcon aria-hidden="true" />
              {t('results.showOnMap')}
            </Button>
          )}
          {onBook && (
            <Button onClick={onBook}>
              <CalendarCheckIcon aria-hidden="true" />
              {t('results.book')}
            </Button>
          )}
        </div>
      )}
    </li>
  );
}
