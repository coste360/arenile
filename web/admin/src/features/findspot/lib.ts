import { formatIsoDate } from '@arenile/shared/lib/dates';
import { defaultFormula } from '../bookings/model';
import type { BookingCreateBody } from './types';
import type { FindSpotProposal, FindSpotRequest } from './api';

type Translate = (key: string, options?: Record<string, unknown>) => string;

/** "A5 dal 10/07 al 14/07, A7 dal 15/07 al 16/07" (one line; spec 30.6). */
export function segmentsText(
  segments: FindSpotProposal['lines'][number]['segments'],
  t: Translate,
): string {
  return segments
    .map((s) =>
      s.starts_on === s.ends_on
        ? t('segment.day', { spot: s.spot_label, date: formatIsoDate(s.starts_on) })
        : t('segment.range', {
            spot: s.spot_label,
            from: formatIsoDate(s.starts_on),
            to: formatIsoDate(s.ends_on),
          }),
    )
    .join(', ');
}

/**
 * The booking of a proposal: the request dates, slot and formula, one line
 * per proposal line with its segments (a line on one spot for the whole
 * stay is sent as `spot_id`), the pack and the agreement of the search.
 */
export function proposalBooking(
  request: FindSpotRequest,
  proposal: FindSpotProposal,
  customerId: string,
): BookingCreateBody {
  const packId = request.pack_id ?? '';
  return {
    customer_id: customerId,
    starts_on: request.starts_on,
    ends_on: request.ends_on,
    slots: request.slots,
    formula: request.formula ?? defaultFormula(request.starts_on, request.ends_on, request.slots),
    lines: proposal.lines.map((l) =>
      l.segments.length === 1
        ? { pack_id: packId, spot_id: l.segments[0]!.spot_id }
        : {
            pack_id: packId,
            segments: l.segments.map((s) => ({
              spot_id: s.spot_id,
              starts_on: s.starts_on,
              ends_on: s.ends_on,
            })),
          },
    ),
    ...(request.agreement_id ? { agreement_id: request.agreement_id } : {}),
    status: 'confirmed',
  };
}
