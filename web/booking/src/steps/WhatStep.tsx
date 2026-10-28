import { lineExtras } from './what/extras';
import type { Schemas } from '@arenile/shared/api';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { useEffect, useId, useMemo, type Dispatch } from 'react';
import { useTranslation } from 'react-i18next';
import { usePublicCatalog, type CatalogQuery } from '../api/public';
import { useBookingEnv } from '../flow/context';
import { localize } from '../flow/format';
import type { FlowAction, FlowLine, FlowState } from '../flow/selection';
import { PUBLIC_NS } from '../i18n';
import { ExtrasList } from './what/ExtrasList';
import { PackChoice } from './what/PackChoice';
import { AgreementCode } from './what/AgreementCode';
import { PromoCode } from './what/PromoCode';

export interface WhatStepProps {
  state: FlowState;
  dispatch: Dispatch<FlowAction>;
  /** Promo status from the latest quote. */
  promoStatus?: Schemas['PromoStatus'];
  agreementStatus?: Schemas['AgreementStatus'];
}

function baseQuery(state: FlowState): CatalogQuery {
  return {
    from: state.dates?.from,
    to: state.dates?.to,
    slots: state.slots,
    formula: state.formula ?? undefined,
  };
}

function LinePacks({
  line,
  state,
  dispatch,
}: {
  line: FlowLine;
  state: FlowState;
  dispatch: Dispatch<FlowAction>;
}) {
  const { t, i18n } = useTranslation(PUBLIC_NS);
  const { t: tc } = useTranslation();
  const { slug, online, establishment, location } = useBookingEnv();
  const id = useId();
  const catalog = usePublicCatalog(slug, {
    ...baseQuery(state),
    zone_id: line.zoneId,
    ...(line.spotId ? { spot_id: line.spotId } : {}),
  });
  const packs = useMemo(() => catalog.data?.packs ?? [], [catalog.data]);
  const zone = online.zones.find((z) => z.id === line.zoneId);
  const zoneName = localize(zone?.name, i18n.language, establishment.default_locale);

  // One compatible pack, or the one requested by ?pack=code: choose it for the customer.
  useEffect(() => {
    if (line.packId || packs.length === 0) return;
    const wanted = location.pack ? packs.find((p) => p.code === location.pack) : undefined;
    const only = packs.length === 1 ? packs[0] : undefined;
    const pick = wanted ?? only;
    if (pick) dispatch({ type: 'pack', key: line.key, packId: pick.id });
  }, [line.packId, line.key, packs, location.pack, dispatch]);

  const title =
    line.mode === 'seat'
      ? t('what.lineSeat', { label: line.spotLabel ?? '' })
      : line.mode === 'quantity'
        ? t('what.lineQuantity', { zone: zoneName, count: line.quantity })
        : t('what.lineZone', { zone: zoneName });

  return (
    <section className="space-y-3" aria-labelledby={`${id}-title`}>
      <h3
        id={`${id}-title`}
        className="text-base font-semibold"
        aria-label={line.mode === 'seat' && zoneName ? `${title}, ${zoneName}` : undefined}
      >
        {title}
        {line.mode === 'seat' && zoneName && (
          <span className="ml-2 text-sm font-normal text-muted-foreground">{zoneName}</span>
        )}
      </h3>
      {catalog.isPending ? (
        <div role="status">
          <span className="sr-only">{tc('common.loading')}</span>
          <Skeleton aria-hidden="true" className="h-28 w-full" />
        </div>
      ) : packs.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('what.noPacks')}</p>
      ) : (
        <PackChoice
          labelledBy={`${id}-title`}
          packs={packs}
          value={line.packId}
          onChange={(packId) => dispatch({ type: 'pack', key: line.key, packId })}
        />
      )}
      <ExtrasList
        title={t('what.extras')}
        extras={lineExtras(catalog.data?.extras ?? [], line.packId)}
        value={line.extras}
        onChange={(extraId, quantity) =>
          dispatch({ type: 'lineExtra', key: line.key, extraId, quantity })
        }
      />
    </section>
  );
}

/**
 * "Cosa" (spec 12.3 passo 3): for each chosen resource the compatible packs
 * (composition with icons) and their extras with quantities, then the
 * booking extras and the promo code. Prices here are indicative; the price
 * summary shows the binding `quote` (debounced) on every change.
 */
export default function WhatStep({ state, dispatch, promoStatus, agreementStatus }: WhatStepProps) {
  const { t } = useTranslation(PUBLIC_NS);
  const { slug, online } = useBookingEnv();
  const general = usePublicCatalog(slug, baseQuery(state));
  const bookingExtras = (general.data?.extras ?? []).filter((e) => e.attach_to === 'booking');

  return (
    <div className="space-y-8">
      {state.lines.map((line) => (
        <LinePacks key={line.key} line={line} state={state} dispatch={dispatch} />
      ))}
      <ExtrasList
        title={t('what.bookingExtras')}
        extras={bookingExtras}
        value={state.bookingExtras}
        onChange={(extraId, quantity) => dispatch({ type: 'bookingExtra', extraId, quantity })}
      />
      {online.promo_codes_enabled && (
        <PromoCode
          value={state.promoCode}
          status={promoStatus}
          onApply={(code) => dispatch({ type: 'promo', code })}
        />
      )}
      <AgreementCode
        value={state.agreementCode}
        status={agreementStatus}
        onApply={(code) => dispatch({ type: 'agreement', code })}
      />
    </div>
  );
}
