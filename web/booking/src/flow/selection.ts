import type { Schemas } from '@arenile/shared/api';
import { daysInclusive } from '@arenile/shared/lib/dates';

/**
 * Functional core of the public booking flow (spec 12.3): the state of the
 * four steps, its reducer and the pure derivations (compatible formulas,
 * PublicSelection for `quote` / `createPublicBooking`, step completeness).
 * No React, no I/O: the Stepper is the imperative shell around it.
 */

export type Slots = Schemas['Slots'];
export type Formula = Schemas['Formula'];
export type AssignmentMode = Schemas['AssignmentMode'];

export interface StayDates {
  from: string;
  /** Missing while the second day of the range has not been picked yet. */
  to?: string;
}

/** One resource chosen in "Dove" and its pack/extras chosen in "Cosa". */
export interface FlowLine {
  /** Stable key: the spot id (seat), else the zone id. */
  key: string;
  zoneId: string;
  mode: AssignmentMode;
  /** Seat zones only. */
  spotId?: string;
  /** Seat zones only, for the summary. */
  spotLabel?: string;
  /** Units (quantity zones), 1 otherwise. */
  quantity: number;
  packId: string | null;
  /** extra id → quantity (attach_to = line). */
  extras: Record<string, number>;
}

export interface FlowState {
  dates?: StayDates;
  slots: Slots;
  /** Null until dates are complete; then the first compatible formula unless the user chose. */
  formula: Formula | null;
  lines: FlowLine[];
  /** extra id → quantity (attach_to = booking). */
  bookingExtras: Record<string, number>;
  promoCode: string;
  /** Agreement code (link `?c=` or typed, spec 30.2.2); '' = none. Outcome in `quote.agreement`. */
  agreementCode: string;
  /** Set when chosen resources were dropped because they became unavailable. */
  dropped: boolean;
}

export type FlowAction =
  | { type: 'dates'; dates: StayDates | undefined; formula: Formula | null }
  | { type: 'slots'; slots: Slots; formula: Formula | null }
  | { type: 'formula'; formula: Formula; dates?: StayDates }
  | { type: 'toggleSpot'; zoneId: string; spotId: string; spotLabel: string }
  | { type: 'toggleZone'; zoneId: string }
  | { type: 'quantity'; zoneId: string; quantity: number }
  | { type: 'pack'; key: string; packId: string | null }
  | { type: 'lineExtra'; key: string; extraId: string; quantity: number }
  | { type: 'bookingExtra'; extraId: string; quantity: number }
  | { type: 'promo'; code: string }
  | { type: 'agreement'; code: string }
  | { type: 'dropLines'; keys: readonly string[] }
  | { type: 'resetResources' };

export function initialFlowState(dates?: StayDates, agreementCode = ''): FlowState {
  return {
    dates,
    slots: 'FULL',
    formula: null,
    lines: [],
    bookingExtras: {},
    promoCode: '',
    agreementCode,
    dropped: false,
  };
}

function setQty(map: Record<string, number>, id: string, qty: number): Record<string, number> {
  const next = { ...map };
  if (qty > 0) next[id] = Math.floor(qty);
  else delete next[id];
  return next;
}

/**
 * The resources depend on dates, slot and formula: changing them clears the
 * chosen lines (availability and prices change) but keeps the promo and agreement codes.
 */
export function flowReducer(state: FlowState, action: FlowAction): FlowState {
  switch (action.type) {
    case 'dates':
      return {
        ...state,
        dropped: false,
        dates: action.dates,
        formula: action.formula,
        lines: [],
        bookingExtras: {},
      };
    case 'slots':
      return { ...state, slots: action.slots, formula: action.formula, lines: [] };
    case 'formula':
      return { ...state, formula: action.formula, dates: action.dates ?? state.dates, lines: [] };
    case 'toggleSpot': {
      if (state.lines.some((l) => l.spotId === action.spotId)) {
        return { ...state, lines: state.lines.filter((l) => l.spotId !== action.spotId) };
      }
      const line: FlowLine = {
        key: action.spotId,
        zoneId: action.zoneId,
        mode: 'seat',
        spotId: action.spotId,
        spotLabel: action.spotLabel,
        quantity: 1,
        packId: null,
        extras: {},
      };
      return { ...state, lines: [...state.lines, line] };
    }
    case 'toggleZone': {
      if (state.lines.some((l) => l.key === action.zoneId)) {
        return { ...state, lines: state.lines.filter((l) => l.key !== action.zoneId) };
      }
      const line: FlowLine = {
        key: action.zoneId,
        zoneId: action.zoneId,
        mode: 'zone',
        quantity: 1,
        packId: null,
        extras: {},
      };
      return { ...state, lines: [...state.lines, line] };
    }
    case 'quantity': {
      const qty = Math.max(0, Math.floor(action.quantity));
      const rest = state.lines.filter((l) => l.key !== action.zoneId);
      if (qty === 0) return { ...state, lines: rest };
      const current = state.lines.find((l) => l.key === action.zoneId);
      const line: FlowLine = current
        ? { ...current, quantity: qty }
        : {
            key: action.zoneId,
            zoneId: action.zoneId,
            mode: 'quantity',
            quantity: qty,
            packId: null,
            extras: {},
          };
      return current
        ? { ...state, lines: state.lines.map((l) => (l.key === line.key ? line : l)) }
        : { ...state, lines: [...rest, line] };
    }
    case 'pack':
      return {
        ...state,
        lines: state.lines.map((l) =>
          l.key === action.key && l.packId !== action.packId
            ? { ...l, packId: action.packId, extras: {} }
            : l,
        ),
      };
    case 'lineExtra':
      return {
        ...state,
        lines: state.lines.map((l) =>
          l.key === action.key
            ? { ...l, extras: setQty(l.extras, action.extraId, action.quantity) }
            : l,
        ),
      };
    case 'bookingExtra':
      return {
        ...state,
        bookingExtras: setQty(state.bookingExtras, action.extraId, action.quantity),
      };
    case 'promo':
      return { ...state, promoCode: action.code.trim().toUpperCase() };
    case 'agreement':
      return { ...state, agreementCode: action.code.trim().toUpperCase() };
    case 'dropLines':
      return {
        ...state,
        dropped: true,
        lines: state.lines.filter((l) => !action.keys.includes(l.key)),
      };
    case 'resetResources':
      return { ...state, lines: [], bookingExtras: {} };
  }
}

/** Most specific first: the default is the first compatible one. */
const FORMULA_ORDER: readonly Formula[] = [
  'season',
  'month',
  'fortnight',
  'week',
  'multi_day',
  'day',
  'half_day',
];

/**
 * Formulas that fit a stay (spec 8.2): half day and day for a single day
 * (by slot), multi-day for more days, week / fortnight / month for exactly
 * 7 / 14 / 28-31 days, season for the whole active season. Restricted to
 * `allowed` (the online configuration; empty = every formula).
 */
export function compatibleFormulas(
  dates: StayDates | undefined,
  slots: Slots,
  allowed: readonly Formula[],
  season?: { opens_on: string; closes_on: string } | null,
): Formula[] {
  if (!dates?.to) return [];
  const n = daysInclusive(dates.from, dates.to);
  if (n <= 0) return [];
  const fits: Record<Formula, boolean> = {
    half_day: n === 1 && slots !== 'FULL',
    day: n === 1 && slots === 'FULL',
    multi_day: n > 1,
    week: n === 7,
    fortnight: n === 14,
    month: n >= 28 && n <= 31,
    season: !!season && dates.from === season.opens_on && dates.to === season.closes_on,
  };
  return FORMULA_ORDER.filter((f) => fits[f] && (allowed.length === 0 || allowed.includes(f)));
}

/** Selectable formulas including season (which sets the dates to the whole season). */
export function formulaChoices(
  dates: StayDates | undefined,
  slots: Slots,
  allowed: readonly Formula[],
  season?: { opens_on: string; closes_on: string } | null,
): Formula[] {
  const list = compatibleFormulas(dates, slots, allowed, season);
  const seasonAllowed = !!season && (allowed.length === 0 || allowed.includes('season'));
  if (seasonAllowed && !list.includes('season')) list.unshift('season');
  return list;
}

function extrasInput(map: Record<string, number>): Schemas['PublicExtraInput'][] {
  return Object.entries(map)
    .filter(([, q]) => q > 0)
    .map(([extra_id, quantity]) => ({ extra_id, quantity }));
}

/** Lines of a selection (with or without packs, the server validates). */
export function lineInputs(lines: readonly FlowLine[]): Schemas['PublicLineInput'][] {
  return lines
    .filter((l): l is FlowLine & { packId: string } => !!l.packId)
    .map((l) => {
      const extras = extrasInput(l.extras);
      return {
        pack_id: l.packId,
        ...(l.mode === 'seat' ? { spot_id: l.spotId } : { zone_id: l.zoneId }),
        ...(l.mode === 'quantity' ? { quantity: l.quantity } : {}),
        ...(extras.length ? { extras } : {}),
      };
    });
}

export const isWhenComplete = (s: FlowState): boolean => !!s.dates?.to && !!s.formula;
export const isWhereComplete = (s: FlowState): boolean => isWhenComplete(s) && s.lines.length > 0;
export const isWhatComplete = (s: FlowState): boolean =>
  isWhereComplete(s) && s.lines.every((l) => !!l.packId);

/**
 * The selection sent to `quote` and `createPublicBooking`, or null while
 * the steps up to "Cosa" are incomplete.
 */
export function toSelection(s: FlowState): Schemas['PublicSelection'] | null {
  if (!isWhatComplete(s) || !s.dates?.to || !s.formula) return null;
  const extras = extrasInput(s.bookingExtras);
  return {
    starts_on: s.dates.from,
    ends_on: s.dates.to,
    slots: s.slots,
    formula: s.formula,
    lines: lineInputs(s.lines),
    ...(extras.length ? { extras } : {}),
    ...(s.promoCode ? { promo_code: s.promoCode } : {}),
    ...(s.agreementCode ? { agreement_code: s.agreementCode } : {}),
  };
}

/** Bookable window: online min/max dates intersected with the season, never before today. */
export function bookableWindow(
  today: string,
  online?: { min_date?: string; max_date?: string } | null,
  season?: { opens_on: string; closes_on: string } | null,
): { min: string; max?: string } {
  const mins = [today, online?.min_date, season?.opens_on].filter(Boolean) as string[];
  const maxs = [online?.max_date, season?.closes_on].filter(Boolean) as string[];
  const min = mins.sort().at(-1) as string;
  const max = maxs.sort()[0];
  return max ? { min, max } : { min };
}
