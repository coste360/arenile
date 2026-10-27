import type { Schemas } from '@arenile/shared/api';
import { parseNames } from '../../partners/lib';

type BookingCreate = Schemas['BookingCreate'];
type BookingPatch = Schemas['BookingPatch'];
type BookingDetail = Schemas['BookingDetail'];
type Customer = Schemas['Customer'];
type Agreement = Schemas['Agreement'];
export type ShareType = Schemas['BookingPartnerShare']['type'];

/**
 * Agreement, partner and group of a booking (spec 30.2.3, 30.2.4, 30.3) as
 * edited in the new booking sheet and in the booking detail. Pure: no React.
 */
export interface BookingAgreementValue {
  /** Agreement id; undefined = the customer's default agreement; '' = none. */
  agreementId?: string;
  /** "Verificato" at the desk: date and operator recorded, never a document. */
  requirementVerified: boolean;
  /** Partner the guest comes from (hotel...); '' = none. */
  partnerId: string;
  shareType: ShareType;
  /** Partner share in basis points (5000 = 50%). */
  sharePercentBp: number | null;
  /** Partner share as a fixed amount in cents. */
  shareAmount: number | null;
  /** Stay at the partner's (optional). */
  guestStay?: { from: string; to?: string };
  groupName: string;
  groupOrigin: string;
  adults: number;
  children: number;
  /** One name per line or comma; no other guest data (spec 23). */
  guestNames: string;
}

export function emptyAgreementValue(): BookingAgreementValue {
  return {
    requirementVerified: false,
    partnerId: '',
    shareType: 'none',
    sharePercentBp: null,
    shareAmount: null,
    groupName: '',
    groupOrigin: '',
    adults: 0,
    children: 0,
    guestNames: '',
  };
}

/** Agreement actually applied: the chosen one, else the customer's default. */
export function effectiveAgreementId(
  v: BookingAgreementValue,
  customer: Pick<Customer, 'agreement_id'> | null | undefined,
): string {
  return v.agreementId ?? customer?.agreement_id ?? '';
}

/** Agreements offered at the desk: active, with the desk channel (absent = desk only). */
export function deskAgreements(list: readonly Agreement[], keepId?: string): Agreement[] {
  return list
    .filter(
      (a) => a.id === keepId || (a.active && (!a.channels?.length || a.channels.includes('desk'))),
    )
    .sort((a, b) => a.name.localeCompare(b.name, 'it'));
}

/** "50" ⇄ 5000 basis points (exact, no floats). */
export function bpToDecimal(bp: number): string {
  const neg = bp < 0;
  const s = String(Math.abs(Math.trunc(bp))).padStart(3, '0');
  const dec = s.slice(-2).replace(/0+$/, '');
  return `${neg ? '-' : ''}${s.slice(0, -2)}${dec ? `.${dec}` : ''}`;
}

export function decimalToBp(value: string | undefined): number | null {
  if (!value) return null;
  const m = /^(\d+)(?:\.(\d{1,2}))?/.exec(value.trim());
  if (!m) return null;
  return Number(m[1]) * 100 + Number((m[2] ?? '').padEnd(2, '0'));
}

function share(v: BookingAgreementValue): Schemas['BookingPartnerShare'] | undefined {
  switch (v.shareType) {
    case 'full':
      return { type: 'full' };
    case 'percent':
      return v.sharePercentBp
        ? { type: 'percent', percent: bpToDecimal(v.sharePercentBp) }
        : undefined;
    case 'amount':
      return v.shareAmount ? { type: 'amount', amount: v.shareAmount } : undefined;
    default:
      return undefined;
  }
}

/** The part of the selection that changes the price: the agreement (live quote). */
export function quoteFields(
  v: BookingAgreementValue,
  customer: Pick<Customer, 'agreement_id'> | null | undefined,
): Pick<BookingCreate, 'agreement_id'> {
  const id = effectiveAgreementId(v, customer);
  return id ? { agreement_id: id } : {};
}

/** Agreement, partner and group fields of the create request (only what is set). */
export function toCreateFields(
  v: BookingAgreementValue,
  customer: Pick<Customer, 'agreement_id'> | null | undefined,
  requirement: string | undefined,
): Partial<BookingCreate> {
  const out: Partial<BookingCreate> = { ...quoteFields(v, customer) };
  if (out.agreement_id && requirement && v.requirementVerified) out.requirement_verified = true;
  if (v.partnerId) {
    out.partner_id = v.partnerId;
    const s = share(v);
    if (s) out.partner_share = s;
    if (v.guestStay?.from && v.guestStay.to) {
      out.partner_guest_stay = { from: v.guestStay.from, to: v.guestStay.to };
    }
  }
  if (v.groupName.trim()) out.group_name = v.groupName.trim();
  if (!v.partnerId && v.groupOrigin.trim()) out.group_origin = v.groupOrigin.trim();
  if (v.adults > 0) out.party_adults = v.adults;
  if (v.children > 0) out.party_children = v.children;
  const names = parseNames(v.guestNames);
  if (names.length) out.guest_names = names;
  return out;
}

/** Current values of a booking, for the edit dialog. */
export function fromDetail(b: BookingDetail): BookingAgreementValue {
  const s = b.partner_share;
  return {
    agreementId: b.agreement_id ?? '',
    requirementVerified: !!b.requirement_verification,
    partnerId: b.partner?.id ?? '',
    shareType: s?.type ?? 'none',
    sharePercentBp: decimalToBp(s?.percent),
    shareAmount: s?.amount ?? null,
    guestStay: b.partner_guest_stay ? { ...b.partner_guest_stay } : undefined,
    groupName: b.group_name ?? '',
    groupOrigin: b.group_origin ?? '',
    adults: b.party_adults ?? 0,
    children: b.party_children ?? 0,
    guestNames: (b.guest_names ?? []).join('\n'),
  };
}

const sameJson = (a: unknown, b: unknown) =>
  JSON.stringify(a ?? null) === JSON.stringify(b ?? null);

/**
 * Patch with only the changed fields (spec 10.3: changing the agreement
 * re-prices the booking with a difference to collect or refund). An empty
 * string removes the agreement or the partner.
 */
export function toPatch(v: BookingAgreementValue, b: BookingDetail): BookingPatch {
  const before = fromDetail(b);
  const patch: BookingPatch = {};
  const agreementId = v.agreementId ?? '';
  if (agreementId !== (b.agreement_id ?? '')) patch.agreement_id = agreementId;
  if (v.partnerId !== before.partnerId) patch.partner_id = v.partnerId;
  const nextShare = v.partnerId
    ? (share(v) ?? { type: 'none' as const })
    : { type: 'none' as const };
  const prevShare = b.partner_share ?? { type: 'none' as const };
  if (!sameJson(nextShare, prevShare) && (v.partnerId || b.partner_share)) {
    patch.partner_share = nextShare;
  }
  const stay =
    v.partnerId && v.guestStay?.from && v.guestStay.to
      ? { from: v.guestStay.from, to: v.guestStay.to }
      : undefined;
  if (stay && !sameJson(stay, b.partner_guest_stay)) patch.partner_guest_stay = stay;
  if (v.groupName.trim() !== before.groupName) patch.group_name = v.groupName.trim();
  const origin = v.partnerId ? '' : v.groupOrigin.trim();
  if (origin !== before.groupOrigin) patch.group_origin = origin;
  if (v.adults !== before.adults) patch.party_adults = v.adults;
  if (v.children !== before.children) patch.party_children = v.children;
  const names = parseNames(v.guestNames);
  if (!sameJson(names, b.guest_names ?? [])) patch.guest_names = names;
  return patch;
}

/** True when the patch changes the price (agreement), so a dry-run preview is shown. */
export function changesPrice(patch: BookingPatch): boolean {
  return patch.agreement_id !== undefined;
}
