import type { FieldTarget } from '@arenile/shared/api';
import type { TFunction } from 'i18next';
import type { BookingCreate } from './api';

export interface BookingFieldContext {
  /** The request that failed: server paths index its lines and extras. */
  body: BookingCreate;
  /** Customer entered inline (customer.* fields exist in the form). */
  inlineCustomer: boolean;
  extraName: (extraId: string) => string | undefined;
  extraMax: (extraId: string) => number | undefined;
  /** Resolves the `bookings` namespace. */
  t: TFunction;
}

const LINE_EXTRA = /^lines\.(\d+)\.extras\.(\d+)\.(quantity|extra_id)$/;
const BOOKING_EXTRA = /^extras\.(\d+)\.(quantity|extra_id)$/;
const LINE_FIELD = /^lines(?:\.(\d+)(?:\.(\w+))?)?$/;

const CUSTOMER_LABELS: Record<string, string> = {
  first_name: 'fields.firstName',
  last_name: 'fields.lastName',
  email: 'fields.email',
  phone: 'fields.phone',
};

/**
 * Maps a server field path of a booking create/quote request (spec 17:
 * `lines[0].extras[0].quantity`, `customer.email`, …) to the field of the
 * new booking sheet and a human label. Extras are matched through the
 * request body, since the form keys them by extra id, not by position.
 */
export function resolveBookingField(
  path: string,
  message: string,
  ctx: BookingFieldContext,
): FieldTarget | undefined {
  const { t, body } = ctx;
  const extra = (extraId: string | undefined, prop: string): FieldTarget | undefined => {
    if (!extraId) return undefined;
    const max = ctx.extraMax(extraId);
    return {
      name: `extras.${extraId}`,
      label: ctx.extraName(extraId) ?? t('fields.extras'),
      // Translate the server's "Massimo N." (booking checkExtra); other texts stay as sent.
      ...(prop === 'quantity' && max !== undefined && /^Massimo \d+/.test(message)
        ? { message: t('validation.extraMax', { max }) }
        : {}),
    };
  };

  let m = LINE_EXTRA.exec(path);
  if (m) return extra(body.lines[Number(m[1])]?.extras?.[Number(m[2])]?.extra_id, m[3] ?? '');
  m = BOOKING_EXTRA.exec(path);
  if (m) return extra(body.extras?.[Number(m[1])]?.extra_id, m[2] ?? '');

  m = LINE_FIELD.exec(path);
  if (m) {
    const line = m[1] !== undefined ? body.lines[Number(m[1])] : undefined;
    switch (m[2]) {
      case 'pack_id':
        return { name: 'pack_id', label: t('fields.pack') };
      case 'quantity':
        return { name: 'quantity', label: t('fields.quantity') };
      case 'zone_id':
        return line?.spot_id
          ? { name: 'spot_ids', label: t('fields.spots') }
          : { name: 'zone_id', label: t('fields.zone') };
      default:
        return {
          name: line?.zone_id && !line.spot_id ? 'zone_id' : 'spot_ids',
          label: t('fields.spots'),
        };
    }
  }

  if (path.startsWith('customer.')) {
    const key = CUSTOMER_LABELS[path.slice('customer.'.length)];
    return ctx.inlineCustomer
      ? { name: path, ...(key ? { label: t(key) } : {}) }
      : { name: 'customer_id', label: t('fields.customer') };
  }
  switch (path) {
    case 'starts_on':
    case 'ends_on':
      return { name: 'ends_on', label: t('fields.period') };
    case 'slots':
      return { label: t('fields.slots') };
    case 'formula':
      return { name: 'formula', label: t('fields.formula') };
    case 'customer_id':
      return { name: 'customer_id', label: t('fields.customer') };
    case 'notes_internal':
      return { name: 'notes_internal', label: t('fields.notesInternal') };
    default:
      return undefined;
  }
}
